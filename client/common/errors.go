package common

// 业务错误码，统一在此定义，避免各 handler 散落魔法数字。
// 与前端文档 api.md 中的错误码保持一致；新增错误码在这里追加。
const (
	ErrNoADB         = 10001 // 未检测到 ADB 环境
	ErrADBFailed     = 10002 // ADB 执行失败
	ErrNoDevice      = 10003 // 未检测到设备，或获取设备应用列表失败
	ErrBadParam      = 10004 // 参数错误
	ErrDeviceOffline = 10006 // 指定设备不在线
	ErrStartCollect  = 10007 // 启动性能采集失败
	ErrStopCollect   = 10008 // 停止性能采集失败
	ErrQueryCollect  = 10009 // 查询性能采集任务失败
	ErrSaveHistory   = 10010 // 保存历史采集失败
	ErrQueryHistory  = 10011 // 查询历史采集失败
	ErrDeleteHistory = 10012 // 删除历史采集失败
)
