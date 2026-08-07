package start

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"interface-load-test/client/common"
	"interface-load-test/client/internal/device_list"
	"interface-load-test/client/internal/perf"
)

// StartCollectPerf 开始性能采集。
func StartCollectPerf(w http.ResponseWriter, r *http.Request) {
	var req StartCollectRequest

	// 解析请求参数。
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.Fail(w, 10004, "device_id 和 package_name 不能为空")
		return
	}

	req.DeviceID = strings.TrimSpace(req.DeviceID)
	req.PackageName = strings.TrimSpace(req.PackageName)
	req.ProcessName = strings.TrimSpace(req.ProcessName)
	req.DeviceModel = strings.TrimSpace(req.DeviceModel)

	// 防止传入空格。
	if req.DeviceID == "" || req.PackageName == "" {
		common.Fail(w, 10004, "device_id 和 package_name 不能为空")
		return
	}
	if req.ProcessName == "" {
		req.ProcessName = req.PackageName
	}

	// Android 设备仍然走原来的在线校验；没有命中 Android 时按 iOS UDID 处理。
	// 这样不会因为 adb 环境异常影响 iOS 采集。
	platform := "ios"
	if output, err := device_list.GetAndroidUUID(); err == nil {
		serials := device_list.ParseDeviceList(output)
		if contains(serials, req.DeviceID) {
			platform = "android"
			req.ProcessName = req.PackageName
		}
	}

	// Manager 内部会自动创建 task_id、立即采集一次、然后每秒继续采集。
	// 不能传 r.Context()，接口返回后该 Context 可能被取消。
	task, err := perf.DefaultManager.Start(
		context.Background(),
		req.DeviceID,
		req.PackageName,
		req.ProcessName,
		platform,
		req.DeviceModel,
	)
	if err != nil {
		common.Fail(w, 10007, "启动性能采集失败: "+err.Error())
		return
	}

	response := StartCollectResponse{
		TaskID:           task.TaskID,
		DeviceID:         task.DeviceID,
		PackageName:      task.PackageName,
		ProcessName:      task.ProcessName,
		Platform:         task.Platform,
		DeviceModel:      task.DeviceModel,
		Status:           string(task.Status),
		StartTime:        task.StartTime,
		SampleIntervalMS: task.SampleInterval,
	}

	// Start 成功后通常已有首条样本；这里仍做长度判断，避免以后改成异步首采时越界。
	if len(task.Samples) > 0 {
		firstSample := task.Samples[0]
		response.InitialSample = &firstSample
	}

	common.Success(w, response)
}

// contains 判断设备是否在线。
func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}

	return false
}
