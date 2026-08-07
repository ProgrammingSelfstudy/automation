package get

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"interface-load-test/perf-rabbit/client/common"
)

// NetworkMetrics 是设备网络流量指标。
type NetworkMetrics struct {
	UploadSpeed   float64 `json:"upload_speed"`   // 上传速度，单位：KB/s
	DownloadSpeed float64 `json:"download_speed"` // 下载速度，单位：KB/s
}

// networkSnapshot 保存某台设备上一次采集的累计流量。
type networkSnapshot struct {
	UploadBytes   int64     // 上一次上传累计字节数
	DownloadBytes int64     // 上一次下载累计字节数
	CollectTime   time.Time // 上一次采集时间
}

var (
	networkMu          sync.Mutex
	networkSnapshotMap = make(map[string]networkSnapshot) // key：设备序列号
)

// CollectNetwork 采集一次设备总上传、下载速度。
// 第一次调用只记录流量基线，因此上传、下载速度均为 0。
func CollectNetwork(deviceID string) (NetworkMetrics, error) {
	if strings.TrimSpace(deviceID) == "" {
		return NetworkMetrics{}, fmt.Errorf("device_id 不能为空")
	}

	// 获取设备所有网卡累计流量。
	output, err := common.AdbShell(deviceID, "cat /proc/net/dev")
	if err != nil {
		return NetworkMetrics{}, fmt.Errorf("获取设备流量失败: %w", err)
	}

	var currentDownloadBytes int64
	var currentUploadBytes int64
	var foundNetworkInterface bool

	for _, line := range strings.Split(output, "\n") {
		// 网卡行格式示例：
		// wlan0: 123456 0 0 0 0 0 0 0 654321 0 0 0 0 0 0 0
		if !strings.Contains(line, ":") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		interfaceName := strings.TrimSpace(parts[0])

		// 只统计常见真实网络网卡：
		// wlan：Wi-Fi
		// eth：以太网
		// rmnet、ccmni：移动网络
		if !strings.HasPrefix(interfaceName, "wlan") &&
			!strings.HasPrefix(interfaceName, "eth") &&
			!strings.HasPrefix(interfaceName, "rmnet") &&
			!strings.HasPrefix(interfaceName, "ccmni") {
			continue
		}

		fields := strings.Fields(parts[1])
		if len(fields) < 9 {
			continue
		}

		// fields[0]：RX bytes，累计下载字节数。
		downloadBytes, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil {
			continue
		}

		// fields[8]：TX bytes，累计上传字节数。
		uploadBytes, err := strconv.ParseInt(fields[8], 10, 64)
		if err != nil {
			continue
		}

		currentDownloadBytes += downloadBytes
		currentUploadBytes += uploadBytes
		foundNetworkInterface = true
	}

	if !foundNetworkInterface {
		return NetworkMetrics{}, fmt.Errorf("未找到可用网络网卡")
	}

	now := time.Now()
	metrics := NetworkMetrics{}

	networkMu.Lock()
	defer networkMu.Unlock()

	// 查询该设备上一次的流量基线。
	last, exists := networkSnapshotMap[deviceID]

	// 第二次开始，才能用两次累计流量的差值计算速度。
	if exists {
		seconds := now.Sub(last.CollectTime).Seconds()

		// 网络切换、重启设备后，累计流量可能变小。
		// 此时本次速度保持 0，下次重新正常计算。
		if seconds > 0 &&
			currentDownloadBytes >= last.DownloadBytes &&
			currentUploadBytes >= last.UploadBytes {

			metrics.DownloadSpeed = float64(
				currentDownloadBytes-last.DownloadBytes,
			) / 1024 / seconds

			metrics.UploadSpeed = float64(
				currentUploadBytes-last.UploadBytes,
			) / 1024 / seconds
		}
	}

	// 保存本次累计流量，作为下一次计算基线。
	networkSnapshotMap[deviceID] = networkSnapshot{
		UploadBytes:   currentUploadBytes,
		DownloadBytes: currentDownloadBytes,
		CollectTime:   now,
	}

	// 保留两位小数。
	metrics.UploadSpeed = math.Round(metrics.UploadSpeed*100) / 100
	metrics.DownloadSpeed = math.Round(metrics.DownloadSpeed*100) / 100

	return metrics, nil

}

//for i := 0; i < 500; i++ {
//networkData, err := get.CollectNetwork("10AF5J1JP1004SU")
//if err != nil {
//fmt.Println("采集流量失败：", err)
//return
//}
//
//fmt.Printf(
//"第 %d 次：上传 %.2f KB/s，下载 %.2f KB/s\n",
//i+1,
//networkData.UploadSpeed,
//networkData.DownloadSpeed,
//)
//
//time.Sleep(time.Second)
//}
