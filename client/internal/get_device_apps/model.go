package get_device_apps

type AppResponse struct {
	AppName     string `json:"app_name,omitempty"`   // 应用展示名，iOS 可拿到中文名；Android 当前只返回包名
	PackageName string `json:"package_name"`         // Android 包名 / iOS Bundle ID，开始采集时使用这个字段
	Executable  string `json:"executable,omitempty"` // iOS 可执行文件名，排查进程名时有用
}

type GetAppsResponse struct {
	Total int           `json:"total"`
	Items []AppResponse `json:"items"`
}

//GET /api/devices/15064060/apps
