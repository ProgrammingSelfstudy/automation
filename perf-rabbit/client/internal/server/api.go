package server

import (
	"net/http"

	"client/internal/device_list"
	"client/internal/get_device_apps"
	"client/internal/perf"
	"client/internal/perf/start"
	"client/internal/perf/stop"
)

func RegisterAPI(mux *http.ServeMux) {
	// 上报 Agent 版本号，供中心平台探测存活时顺带做版本兼容检查。
	mux.HandleFunc("GET /api/agent/info", AgentInfoHandler)

	// 获取设备列表。
	mux.HandleFunc("GET /api/device/list", device_list.GetDeviceInfo)

	// 获取指定设备已安装应用列表。
	mux.HandleFunc("GET /api/devices/{deviceId}/apps", get_device_apps.GetDeviceApps)

	// 开始性能采集。
	mux.HandleFunc("POST /api/collect/perf/start", start.StartCollectPerf)

	// 查询性能采集任务实时数据（轮询，兼容不支持 WebSocket 的场景）。
	mux.HandleFunc("GET /api/collect/perf/{taskId}", perf.GetCollectPerf)

	// 推送性能采集任务增量数据，替代前端轮询。
	mux.HandleFunc("GET /ws/collect/perf/{taskId}", perf.ServeCollectWS)

	// 停止指定性能采集任务。
	mux.HandleFunc("POST /api/collect/perf/{taskId}/stop", stop.StopCollectPerf)

	// 查询历史性能采集列表。
	mux.HandleFunc("GET /api/collect/perf-history", perf.GetPerfHistoryList)

	// 查询历史性能采集详情。
	mux.HandleFunc("GET /api/collect/perf-history/{taskId}", perf.GetPerfHistoryDetail)

	// 下载历史性能采集 CSV。
	mux.HandleFunc("GET /api/collect/perf-history/{taskId}/csv", perf.DownloadPerfHistoryCSV)

	// 删除历史性能采集记录。
	mux.HandleFunc("DELETE /api/collect/perf-history/{taskId}", perf.DeletePerfHistory)
}
