package start

import "interface-load-test/client/internal/perf"

// StartCollectRequest 是开始性能采集的请求参数。
type StartCollectRequest struct {
	DeviceID    string `json:"device_id"`    // Android 序列号 / iOS UDID
	PackageName string `json:"package_name"` // Android 包名 / iOS Bundle ID
	ProcessName string `json:"process_name"` // iOS 进程名，例如 VoiceRoom；Android 可不传
	DeviceModel string `json:"device_model"` // iOS 机型，例如 iPhone12,1 或 iPhone 11；用于 CPU 核心数换算
}

// StartCollectResponse 是开始性能采集接口的返回数据。
type StartCollectResponse struct {
	TaskID           string             `json:"task_id"`                  // 服务端自动生成的任务ID
	DeviceID         string             `json:"device_id"`                // Android 序列号 / iOS UDID
	PackageName      string             `json:"package_name"`             // Android 包名 / iOS Bundle ID
	ProcessName      string             `json:"process_name"`             // Android 等同包名；iOS 是进程名
	Platform         string             `json:"platform"`                 // android / ios
	DeviceModel      string             `json:"device_model"`             // iOS 机型
	Status           string             `json:"status"`                   // collecting：采集中
	StartTime        string             `json:"start_time"`               // 开始采集时间
	SampleIntervalMS int64              `json:"sample_interval_ms"`       // 采集间隔，单位毫秒
	InitialSample    *perf.MetricSample `json:"initial_sample,omitempty"` // 启动后立即采集的首条数据
}
