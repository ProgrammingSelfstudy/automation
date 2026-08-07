package get

import (
	"client/common"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type CpuInfo struct {
	AppCpu   float64 `json:"app_cpu"`
	TotalCpu float64 `json:"total_cpu"` // 设备 CPU 使用率：user + nice + sys，已除以 CPU 核数
}

var totalCPURegexp = regexp.MustCompile(
	`(?m)^\s*([\d.]+)%cpu\s+([\d.]+)%user\s+([\d.]+)%nice\s+([\d.]+)%sys`,
)

// CollectCPU 从指定设备采集一次应用 CPU 和设备整体 CPU。
func CollectCPU(deviceID, packageName string) (interface{}, error) {

	output, err := common.AdbShell(deviceID, "top -n 1 -d 1")
	if err != nil {
		return nil, fmt.Errorf("执行 top 命令失败: %w", err)
	}

	// 取设备 CPU 汇总行。
	// 示例：800%cpu  63%user  4%nice  163%sys ...
	match := totalCPURegexp.FindStringSubmatch(output)
	if len(match) < 5 {
		return nil, fmt.Errorf("未找到设备 CPU 汇总数据")
	}
	cpuCore, _ := strconv.ParseFloat(match[1], 64) // cpu核心
	userCPU, _ := strconv.ParseFloat(match[2], 64) // 63user
	niceCPU, _ := strconv.ParseFloat(match[3], 64) // 4nice
	sysCPU, _ := strconv.ParseFloat(match[4], 64)  // 163sys

	appCPU := 0.0
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)

		if len(fields) < 12 {
			continue
		}

		if fields[11] != packageName {
			continue
		}
		cpu, err := strconv.ParseFloat(fields[8], 64)
		if err != nil {
			continue
		}
		appCPU = cpu
		break

	}
	coreCount := cpuCore / 100
	totalCpu := (userCPU + niceCPU + sysCPU) / coreCount
	appCPU = appCPU / coreCount
	return CpuInfo{
		AppCpu:   appCPU,
		TotalCpu: totalCpu,
	}, nil
}
