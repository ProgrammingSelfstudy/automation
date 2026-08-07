package stop

// StopCollectRequest 是停止性能采集的请求参数。
type StopCollectRequest struct {
	TaskID string `json:"task_id" binding:"required"` // 开始采集接口返回的任务ID
}

// StopCollectResponse 是停止性能采集后的返回数据。
type StopCollectResponse struct {
	TaskID      string `json:"task_id"`      // 任务ID
	DeviceID    string `json:"device_id"`    // Android 序列号 / iOS UDID
	PackageName string `json:"package_name"` // Android 包名 / iOS Bundle ID
	ProcessName string `json:"process_name"` // Android 等同包名；iOS 是进程名
	Platform    string `json:"platform"`     // android / ios
	DeviceModel string `json:"device_model"` // iOS 机型
	Status      string `json:"status"`       // stopped：已停止
	StopTime    string `json:"stop_time"`    // 停止时间
}
