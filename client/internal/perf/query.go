package perf

import (
	"net/http"
	"strconv"
	"strings"

	"interface-load-test/client/common"
)

// GetCollectPerf 查询指定任务已采集的性能数据。
func GetCollectPerf(w http.ResponseWriter, r *http.Request) {
	taskID := strings.TrimSpace(r.PathValue("taskId"))

	if taskID == "" {
		common.Fail(w, 10004, "task_id 不能为空")
		return
	}

	fromSample := 0
	fromRaw := strings.TrimSpace(r.URL.Query().Get("from"))
	if fromRaw != "" {
		value, err := strconv.Atoi(fromRaw)
		if err != nil || value < 0 {
			common.Fail(w, 10004, "from 必须是大于等于 0 的整数")
			return
		}
		fromSample = value
	}

	// Manager 内部会返回任务副本；带 from 时只复制新增 Samples。
	task, totalSamples, err := DefaultManager.GetTask(taskID, fromSample)
	if err != nil {
		common.Fail(w, 10009, "查询性能数据失败: "+err.Error())
		return
	}

	common.Success(w, GetCollectPerfResponse{
		TaskID:         task.TaskID,
		DeviceID:       task.DeviceID,
		PackageName:    task.PackageName,
		ProcessName:    task.ProcessName,
		Platform:       task.Platform,
		DeviceModel:    task.DeviceModel,
		Status:         task.Status,
		StartTime:      task.StartTime,
		SampleInterval: task.SampleInterval,
		LastError:      task.LastError,
		TotalSamples:   totalSamples,
		NextFrom:       totalSamples,
		Samples:        task.Samples,
	})
}

// GetCollectPerfResponse 是查询性能数据接口返回结构。
type GetCollectPerfResponse struct {
	TaskID         string         `json:"task_id"`            // 任务ID
	DeviceID       string         `json:"device_id"`          // Android 序列号 / iOS UDID
	PackageName    string         `json:"package_name"`       // Android 包名 / iOS Bundle ID
	ProcessName    string         `json:"process_name"`       // Android 等同包名；iOS 是进程名
	Platform       string         `json:"platform"`           // android / ios
	DeviceModel    string         `json:"device_model"`       // iOS 机型
	Status         string         `json:"status"`             // collecting / stopped
	StartTime      string         `json:"start_time"`         // 开始采集时间
	SampleInterval int64          `json:"sample_interval_ms"` // 采集间隔，单位毫秒
	LastError      string         `json:"last_error"`         // 最近一次采集错误
	TotalSamples   int            `json:"total_samples"`      // 当前任务累计样本数
	NextFrom       int            `json:"next_from"`          // 前端下次查询时可作为 from 参数
	Samples        []MetricSample `json:"samples"`            // 每秒性能数据
}
