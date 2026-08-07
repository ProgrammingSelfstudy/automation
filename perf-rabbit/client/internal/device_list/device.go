package device_list

import (
	"client/common"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// deviceStaticInfo 保存基本不会变化的 Android 设备信息，避免前端轮询设备列表时反复 adb shell。
type deviceStaticInfo struct {
	Version  string
	Brand    string
	Model    string
	CachedAt time.Time
}

type iosDeviceListCache struct {
	Devices  []DeviceInfo
	Err      error
	CachedAt time.Time
}

// sync.Map 保证多个 HTTP 请求同时读取缓存时安全。
var deviceStaticCache sync.Map
var iosDeviceCache = iosDeviceListCache{}
var iosDeviceCacheMu sync.RWMutex

// 静态信息十分钟刷新一次：列表接口会被前端频繁请求，缓存能明显减少 adb 开销。
const deviceStaticCacheTTL = 10 * time.Minute
const iosDeviceCacheTTL = 30 * time.Second

type androidDeviceLine struct {
	Serial string
	Status string
}

type deviceCollectResult struct {
	Devices []DeviceInfo
	Err     error
}

type pymobiledeviceItem struct {
	DeviceName     string `json:"DeviceName"`
	Identifier     string `json:"Identifier"`
	UniqueDeviceID string `json:"UniqueDeviceID"`
	ProductType    string `json:"ProductType"`
	ProductVersion string `json:"ProductVersion"`
	ConnectionType string `json:"ConnectionType"`
}

// GetAndroidUUID 保留给其它 Android 接口复用，只负责执行 adb devices -l。
func GetAndroidUUID() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "adb", "devices", "-l").CombinedOutput()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(out)), nil
}

// ParseDeviceList 保留原有方法名，返回状态为 device 的 Android 序列号。
func ParseDeviceList(raw string) []string {
	lines := parseAndroidDeviceLines(raw)
	serials := make([]string, 0, len(lines))
	for _, line := range lines {
		if line.Status == "device" {
			serials = append(serials, line.Serial)
		}
	}

	return serials
}

// GetDeviceInfo 一个接口同时返回多台 Android 和多台 iOS。
func GetDeviceInfo(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	resultCh := make(chan deviceCollectResult, 2)

	// Android 和 iOS 互不依赖，并发采集，避免其中一种平台慢导致整个接口慢。
	go func() {
		devices, err := collectAndroidDevices(ctx)
		resultCh <- deviceCollectResult{Devices: devices, Err: err}
	}()
	go func() {
		devices, err := collectIOSDevices(ctx)
		resultCh <- deviceCollectResult{Devices: devices, Err: err}
	}()

	var allDevices []DeviceInfo
	var errorsText []string
	for i := 0; i < 2; i++ {
		result := <-resultCh
		if result.Err != nil {
			errorsText = append(errorsText, result.Err.Error())
		}
		allDevices = append(allDevices, result.Devices...)
	}

	if len(allDevices) == 0 {
		// 两个平台都没有设备时才返回失败；单个平台工具缺失不影响另一个平台正常展示。
		msg := "未检测到连接设备，请检查 Android USB 调试或 iOS 信任连接"
		if len(errorsText) > 0 {
			msg += "：" + strings.Join(errorsText, "；")
		}
		common.Fail(w, 10003, msg)
		return
	}

	androidTotal := 0
	iosTotal := 0
	for _, device := range allDevices {
		if device.Platform == "ios" {
			iosTotal++
		} else {
			androidTotal++
		}
	}

	common.Success(w, map[string]any{
		"total":         len(allDevices),
		"android_total": androidTotal,
		"ios_total":     iosTotal,
		"devices":       allDevices,
	})
}

func collectAndroidDevices(ctx context.Context) ([]DeviceInfo, error) {
	out, err := exec.CommandContext(ctx, "adb", "devices", "-l").CombinedOutput()
	if err != nil {
		if strings.Contains(err.Error(), "executable file not found") {
			return nil, errors.New("未检测到 adb 环境")
		}
		return nil, errors.New("adb 执行失败")
	}

	lines := parseAndroidDeviceLines(string(out))
	if len(lines) == 0 {
		return nil, nil
	}

	devices := make([]DeviceInfo, len(lines))
	var wg sync.WaitGroup
	for index, line := range lines {
		index := index
		line := line
		devices[index] = DeviceInfo{
			Serial:   line.Serial,
			DeviceID: line.Serial,
			Platform: "android",
			Status:   androidStatusText(line.Status),
		}

		// 只有 Online 的 Android 才能 adb shell；offline/unauthorized 直接返回状态，避免无效等待。
		if line.Status != "device" {
			continue
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			devices[index] = fillAndroidStaticInfo(ctx, devices[index])
		}()
	}
	wg.Wait()

	return devices, nil
}

func parseAndroidDeviceLines(raw string) []androidDeviceLine {
	var devices []androidDeviceLine
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "List of devices attached") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= 2 {
			devices = append(devices, androidDeviceLine{Serial: parts[0], Status: parts[1]})
		}
	}

	return devices
}

func fillAndroidStaticInfo(ctx context.Context, device DeviceInfo) DeviceInfo {
	cachedValue, cacheExists := deviceStaticCache.Load(device.DeviceID)
	staticInfo, cacheValid := cachedValue.(deviceStaticInfo)
	if cacheExists && cacheValid && time.Since(staticInfo.CachedAt) < deviceStaticCacheTTL {
		device.Version = staticInfo.Version
		device.Brand = staticInfo.Brand
		device.Model = staticInfo.Model
		return device
	}

	// 三个 getprop 合成一次 adb shell，比连续执行三次 adb shell 快很多。
	out, err := common.AdbShellCommandContext(
		ctx,
		device.DeviceID,
		"getprop ro.build.version.release; getprop ro.product.brand; getprop ro.product.model",
	)
	if err != nil {
		device.Error = "获取 Android 设备信息失败"
		return device
	}

	lines := strings.Split(out, "\n")
	if len(lines) > 0 {
		device.Version = strings.TrimSpace(lines[0])
	}
	if len(lines) > 1 {
		device.Brand = strings.TrimSpace(lines[1])
	}
	if len(lines) > 2 {
		device.Model = strings.TrimSpace(lines[2])
	}

	if device.Version != "" && device.Brand != "" && device.Model != "" {
		deviceStaticCache.Store(device.DeviceID, deviceStaticInfo{
			Version:  device.Version,
			Brand:    device.Brand,
			Model:    device.Model,
			CachedAt: time.Now(),
		})
	}

	return device
}

func collectIOSDevices(ctx context.Context) ([]DeviceInfo, error) {
	iosDeviceCacheMu.RLock()
	if time.Since(iosDeviceCache.CachedAt) < iosDeviceCacheTTL {
		devices := append([]DeviceInfo(nil), iosDeviceCache.Devices...)
		err := iosDeviceCache.Err
		iosDeviceCacheMu.RUnlock()
		return devices, err
	}
	iosDeviceCacheMu.RUnlock()

	pythonCommand, pythonErr := common.PythonCommand()
	if pythonErr != nil {
		readErr := errors.New("未检测到 Python 或 pymobiledevice3")
		cacheIOSDevices(nil, readErr)
		return nil, readErr
	}

	// iOS 设备列表统一使用 pymobiledevice3。
	log.Printf("执行 iOS 设备列表命令: %s -m pymobiledevice3 usbmux list", pythonCommand)
	out, err := exec.CommandContext(ctx, pythonCommand, "-m", "pymobiledevice3", "usbmux", "list").CombinedOutput()
	if err != nil {
		log.Printf("iOS 设备列表命令失败: %s -m pymobiledevice3 usbmux list, error=%v, output=%s", pythonCommand, err, strings.TrimSpace(string(out)))
		readErr := errors.New("未检测到 pymobiledevice3，或 iOS 设备列表读取失败")
		cacheIOSDevices(nil, readErr)
		return nil, readErr
	}

	devices, parseErr := parsePymobiledeviceList(string(out))
	if parseErr != nil {
		readErr := errors.New("解析 iOS 设备列表失败")
		cacheIOSDevices(nil, readErr)
		return nil, readErr
	}

	cacheIOSDevices(devices, nil)
	return devices, nil
}

func cacheIOSDevices(devices []DeviceInfo, err error) {
	iosDeviceCacheMu.Lock()
	defer iosDeviceCacheMu.Unlock()

	// 缓存一小段时间，避免前端轮询时反复拉起 Python；设备接入后最多延迟 30 秒刷新出来。
	iosDeviceCache = iosDeviceListCache{
		Devices:  append([]DeviceInfo(nil), devices...),
		Err:      err,
		CachedAt: time.Now(),
	}
}

func parsePymobiledeviceList(raw string) ([]DeviceInfo, error) {
	var items []pymobiledeviceItem
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, err
	}

	devices := make([]DeviceInfo, 0, len(items))
	for _, item := range items {
		udid := strings.TrimSpace(item.UniqueDeviceID)
		if udid == "" {
			udid = strings.TrimSpace(item.Identifier)
		}
		if udid == "" {
			continue
		}

		model := strings.TrimSpace(item.ProductType)
		if model == "" {
			model = "iPhone"
		}

		devices = append(devices, DeviceInfo{
			Serial:         udid,
			DeviceID:       udid,
			DeviceName:     strings.TrimSpace(item.DeviceName),
			Platform:       "ios",
			Version:        strings.TrimSpace(item.ProductVersion),
			Brand:          "Apple",
			Model:          model,
			ProductType:    strings.TrimSpace(item.ProductType),
			ConnectionType: strings.TrimSpace(item.ConnectionType),
			Status:         "Online",
		})
	}

	return devices, nil
}

func androidStatusText(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "device":
		return "Online"
	case "unauthorized":
		return "Unauthorized"
	case "offline":
		return "Offline"
	default:
		return status
	}
}
