package perf

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"client/internal/perf/get"
)

// PerformanceData 是一秒内采集到的完整性能数据。
type PerformanceData struct {
	CPU    interface{} `json:"cpu"`    // CPU数据
	Memory interface{} `json:"memory"` // 内存数据
	//Network interface{} `json:"network"` // 流量数据
	FPS  interface{} `json:"fps"`  // FPS数据
	Jank interface{} `json:"jank"` // 卡顿数据
}

// IOSPerformanceData 是 iOS 当前支持的指标。
// iOS 现在只采 CPU、内存、FPS、GPU，不返回 Jank，避免前端误以为卡顿指标有效。
type IOSPerformanceData struct {
	CPU    interface{} `json:"cpu"`    // iOS 进程 CPU
	Memory interface{} `json:"memory"` // iOS 进程物理内存
	FPS    interface{} `json:"fps"`    // iOS CoreAnimation FPS
	GPU    interface{} `json:"gpu"`    // iOS Device GPU 利用率
}

// ResetPerformanceMetrics 清空带状态的性能指标。
// 每个新任务开始前必须重置，避免 FPS/Jank 复用上一个任务的游标和累计值。
func ResetPerformanceMetrics(deviceID, packageName, processName string) {
	get.ResetFPS(deviceID, packageName)
	get.ResetJankTotal(deviceID, packageName)
	get.StopIOSSysmon(deviceID, processName)
	get.StopIOSGraphics(deviceID, processName)
}

// CollectPerformanceMetrics 是 Manager 每次 Tick 实际调用的方法。
// 开始接口调用成功后会立即执行一次，之后每秒执行一次。
func CollectPerformanceMetrics(
	ctx context.Context,
	deviceID string,
	packageName string,
	processName string,
	platform string,
	deviceModel string,
) (MetricSample, error) {
	if processName == "" {
		processName = packageName
	}

	if strings.EqualFold(platform, "ios") {
		var cpuData get.IOSCPUInfo
		var memoryData get.IOSMemoryInfo
		var fpsData get.IOSFPSInfo
		var gpuData get.IOSGPUInfo
		var cpuMemoryErr error
		var graphicsErr error

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			cpuData, memoryData, cpuMemoryErr = get.CollectIOSCPUAndMemory(ctx, deviceID, processName, deviceModel)
		}()
		go func() {
			defer wg.Done()
			fpsData, gpuData, graphicsErr = get.CollectIOSGraphics(ctx, deviceID, processName)
		}()
		wg.Wait()

		if cpuMemoryErr != nil {
			return MetricSample{}, fmt.Errorf("采集 iOS CPU/内存失败: %w", cpuMemoryErr)
		}
		if graphicsErr != nil {
			return MetricSample{}, fmt.Errorf("采集 iOS FPS/GPU失败: %w", graphicsErr)
		}

		return MetricSample{
			CollectTime: time.Now().Format("2006-01-02 15:04:05"),
			Data: IOSPerformanceData{
				CPU:    cpuData,
				Memory: memoryData,
				FPS:    fpsData,
				GPU:    gpuData,
			},
		}, nil
	}

	data := PerformanceData{}

	// 采集 CPU。
	cpuData, err := get.CollectCPU(deviceID, packageName)
	if err != nil {
		return MetricSample{}, fmt.Errorf("采集CPU失败: %w", err)
	}
	data.CPU = cpuData

	// 采集内存。
	memoryData, err := get.CollectMemory(deviceID, packageName)
	if err != nil {
		return MetricSample{}, fmt.Errorf("采集内存失败: %w", err)
	}
	data.Memory = memoryData

	//// 采集设备流量。
	//networkData, err := get.CollectNetwork(deviceID)
	//if err != nil {
	//	return MetricSample{}, fmt.Errorf("采集流量失败: %w", err)
	//}
	//data.Network = networkData

	// 采集 FPS。
	fpsData, err := get.CollectFPS(deviceID, packageName)
	if err != nil {
		return MetricSample{}, fmt.Errorf("采集FPS失败: %w", err)
	}
	data.FPS = fpsData

	// 采集卡顿。
	jankData, err := get.CollectJank(deviceID, packageName)
	if err != nil {
		return MetricSample{}, fmt.Errorf("采集卡顿失败: %w", err)
	}
	data.Jank = jankData

	return MetricSample{
		CollectTime: time.Now().Format("2006-01-02 15:04:05"),
		Data:        data,
	}, nil
}
