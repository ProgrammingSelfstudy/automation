package get

import (
	"bufio"
	"context"
	"fmt"
	"interface-load-test/perf-rabbit/client/common"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type IOSFPSInfo struct {
	CoreAnimationFramesPerSecond float64 `json:"core_animation_frames_per_second"` // 对应 CoreAnimationFramesPerSecond
}

type IOSGPUInfo struct {
	DeviceUtilization float64 `json:"device_utilization"` // 对应 Device Utilization %
}

type iosGraphicsSample struct {
	FPS               float64
	DeviceUtilization float64
}

type iosGraphicsMonitor struct {
	cancel context.CancelFunc
	ready  chan struct{}
	once   sync.Once

	mu     sync.RWMutex
	latest *iosGraphicsSample
	err    error
}

var (
	iosGraphicsMonitors             sync.Map
	iosGraphicsFPSRegexp            = regexp.MustCompile(`'CoreAnimationFramesPerSecond':\s*([0-9.]+)`)
	iosGraphicsDeviceUtilRegexp     = regexp.MustCompile(`'Device Utilization %':\s*([0-9.]+)`)
	iosGraphicsDoubleQuoteFPSRegexp = regexp.MustCompile(`"CoreAnimationFramesPerSecond":\s*([0-9.]+)`)
	iosGraphicsDoubleQuoteGPURegexp = regexp.MustCompile(`"Device Utilization %":\s*([0-9.]+)`)
)

// CollectIOSGraphics 从 iOS graphics 后台进程取 FPS 和 GPU Device 利用率。
// 只保留 CoreAnimationFramesPerSecond 和 Device Utilization % 两个值。
func CollectIOSGraphics(ctx context.Context, deviceID, processName string) (IOSFPSInfo, IOSGPUInfo, error) {
	monitor := loadIOSGraphicsMonitor(deviceID, processName)

	select {
	case <-monitor.ready:
	case <-time.After(8 * time.Second):
		return IOSFPSInfo{}, IOSGPUInfo{}, fmt.Errorf("等待 iOS graphics 数据超时")
	case <-ctx.Done():
		return IOSFPSInfo{}, IOSGPUInfo{}, ctx.Err()
	}

	monitor.mu.RLock()
	sample := monitor.latest
	lastErr := monitor.err
	monitor.mu.RUnlock()

	if sample == nil {
		if lastErr != nil {
			return IOSFPSInfo{}, IOSGPUInfo{}, lastErr
		}
		return IOSFPSInfo{}, IOSGPUInfo{}, fmt.Errorf("未获取到 iOS graphics 数据")
	}

	return IOSFPSInfo{
			CoreAnimationFramesPerSecond: sample.FPS,
		}, IOSGPUInfo{
			DeviceUtilization: sample.DeviceUtilization,
		}, nil
}

func StopIOSGraphics(deviceID, processName string) {
	key := buildIOSGraphicsKey(deviceID, processName)
	value, exists := iosGraphicsMonitors.LoadAndDelete(key)
	if !exists {
		return
	}

	monitor, ok := value.(*iosGraphicsMonitor)
	if ok && monitor.cancel != nil {
		monitor.cancel()
	}
}

func loadIOSGraphicsMonitor(deviceID, processName string) *iosGraphicsMonitor {
	key := buildIOSGraphicsKey(deviceID, processName)
	if value, exists := iosGraphicsMonitors.Load(key); exists {
		return value.(*iosGraphicsMonitor)
	}

	// 与 ios_sysmon.go 保持一致：在 goroutine 启动前赋值 cancel，消除竞态。
	ctx, cancel := context.WithCancel(context.Background())
	monitor := &iosGraphicsMonitor{ready: make(chan struct{}), cancel: cancel}

	actual, loaded := iosGraphicsMonitors.LoadOrStore(key, monitor)
	if loaded {
		// 另一个协程抢先注册了相同 key，撤销我们创建的 context，返回已有的 monitor。
		cancel()
		return actual.(*iosGraphicsMonitor)
	}

	go monitor.run(deviceID, key, ctx)
	return monitor
}

func (m *iosGraphicsMonitor) run(deviceID, key string, ctx context.Context) {
	// ctx 和 cancel 已由 loadIOSGraphicsMonitor 在 goroutine 启动前创建并赋值到结构体，
	// 这里直接使用，不再内部构造，消除与 StopIOSGraphics 之间的竞态。

	pythonCommand, pythonErr := common.PythonCommand()
	if pythonErr != nil {
		m.setError(fmt.Errorf("未检测到 Python 或 pymobiledevice3"))
		return
	}

	args := []string{
		"-m",
		"pymobiledevice3",
		"developer",
		"dvt",
		"graphics",
	}
	args = append(args, iosDVTConnectionArgs(deviceID)...)

	cmd := exec.CommandContext(ctx, pythonCommand, args...)
	// graphics 走日志输出，关闭缓冲后才能稳定按秒读取。
	cmd.Env = append(os.Environ(), "PYTHONUNBUFFERED=1")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		m.setError(fmt.Errorf("启动 iOS graphics 失败: %w", err))
		return
	}
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		m.setError(fmt.Errorf("启动 iOS graphics 失败: %w", err))
		return
	}

	if stderr != nil {
		go m.drainStderr(stderr)
	}
	m.readSamples(stdout)

	if err := cmd.Wait(); err != nil && ctx.Err() == nil {
		m.setError(fmt.Errorf("iOS graphics 已退出: %w", err))
	}
	iosGraphicsMonitors.Delete(key)
}

func (m *iosGraphicsMonitor) readSamples(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		sample, ok := parseIOSGraphicsLine(line)
		if !ok {
			continue
		}

		m.mu.Lock()
		m.latest = &sample
		m.err = nil
		m.mu.Unlock()
		m.once.Do(func() { close(m.ready) })
	}

	if err := scanner.Err(); err != nil {
		m.setError(fmt.Errorf("读取 iOS graphics 输出失败: %w", err))
	}
}

// drainStderr 消耗 stderr 避免管道满后阻塞整个采集进程。
// 跟 ios_sysmon.go 保持一致：stderr 只消费不解析，不能跟 stdout 共用
// readSamples——共用会导致 stderr 里凑巧匹配正则的内容被当成真实采集
// 数据写入 m.latest，甚至可能让 monitor.ready 提前被 stderr 内容关闭。
func (m *iosGraphicsMonitor) drainStderr(stderr io.Reader) {
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		// 只消耗，不处理。
	}
}

func parseIOSGraphicsLine(line string) (iosGraphicsSample, bool) {
	fps, fpsOK := matchFloat(line, iosGraphicsFPSRegexp)
	if !fpsOK {
		fps, fpsOK = matchFloat(line, iosGraphicsDoubleQuoteFPSRegexp)
	}

	deviceUtilization, gpuOK := matchFloat(line, iosGraphicsDeviceUtilRegexp)
	if !gpuOK {
		deviceUtilization, gpuOK = matchFloat(line, iosGraphicsDoubleQuoteGPURegexp)
	}

	if !fpsOK || !gpuOK {
		return iosGraphicsSample{}, false
	}

	return iosGraphicsSample{
		FPS:               fps,
		DeviceUtilization: deviceUtilization,
	}, true
}

func matchFloat(text string, pattern *regexp.Regexp) (float64, bool) {
	match := pattern.FindStringSubmatch(text)
	if len(match) < 2 {
		return 0, false
	}

	value, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return 0, false
	}

	return value, true
}

func (m *iosGraphicsMonitor) setError(err error) {
	m.mu.Lock()
	m.err = err
	m.mu.Unlock()
	m.once.Do(func() { close(m.ready) })
}

func buildIOSGraphicsKey(deviceID, processName string) string {
	return strings.TrimSpace(deviceID) + "::" + strings.TrimSpace(processName)
}
