package get

import (
	"fmt"
	"regexp"
	"strconv"

	"interface-load-test/perf-rabbit/client/common"
)

// MemoryInfo 是应用核心内存指标，单位：KB。
type MemoryInfo struct {
	JavaHeap   int64 `json:"java_heap"`   // Java 堆 PSS：Java/Kotlin 对象等实际占用的内存
	NativeHeap int64 `json:"native_heap"` // Native 堆 PSS：C/C++、NDK、底层 SDK 等占用的内存
	Stack      int64 `json:"stack"`       // 线程栈 PSS：应用所有线程栈占用的内存
	Graphics   int64 `json:"graphics"`    // 图形 PSS：Bitmap、纹理、Surface、渲染等占用的内存
	TotalPSS   int64 `json:"total_pss"`   // 总 PSS：应用真实内存占用，前端内存主趋势图建议使用该值
}

var (
	javaHeapRegexp   = regexp.MustCompile(`(?m)^\s*Java Heap:\s*(\d+)`)
	nativeHeapRegexp = regexp.MustCompile(`(?m)^\s*Native Heap:\s*(\d+)`)
	stackRegexp      = regexp.MustCompile(`(?m)^\s*Stack:\s*(\d+)`)
	graphicsRegexp   = regexp.MustCompile(`(?m)^\s*Graphics:\s*(\d+)`)
	totalPSSRegexp   = regexp.MustCompile(`(?m)^\s*TOTAL PSS:\s*(\d+)`)
)

// CollectMemory 从指定设备采集一次应用内存数据。
func CollectMemory(deviceID, packageName string) (interface{}, error) {
	output, err := common.AdbShell(
		deviceID,
		"dumpsys meminfo --local -s "+packageName,
	)
	if err != nil {
		return nil, fmt.Errorf("执行 dumpsys meminfo 失败: %w", err)
	}

	// 从 dumpsys 输出中匹配数值；不存在或解析失败时返回 0。
	getValue := func(pattern *regexp.Regexp) int64 {
		match := pattern.FindStringSubmatch(output)
		if len(match) < 2 {
			return 0
		}

		value, _ := strconv.ParseInt(match[1], 10, 64)
		return value
	}

	metrics := MemoryInfo{
		JavaHeap:   getValue(javaHeapRegexp) / 1024,
		NativeHeap: getValue(nativeHeapRegexp) / 1024,
		Stack:      getValue(stackRegexp) / 1024,
		Graphics:   getValue(graphicsRegexp) / 1024,
		TotalPSS:   getValue(totalPSSRegexp) / 1024,
	}

	// TOTAL PSS 没取到，说明包名不存在、应用未运行，或者 dumpsys 输出异常。
	if metrics.TotalPSS == 0 {
		return nil, fmt.Errorf("未获取到应用 TOTAL PSS，package_name=%s", packageName)
	}

	return metrics, nil
}
