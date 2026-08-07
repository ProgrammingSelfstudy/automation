package main

import (
	"log"
	"net/http"

	"interface-load-test/client/internal/perf"
	"interface-load-test/client/internal/server"
)

func main() {
	// 本地采集 Agent 入口：只启动后端接口，不内嵌前端页面，不自动开浏览器——
	// 中心平台的浏览器页面直接跟这个进程对话（127.0.0.1:9527），UI 全部在
	// 中心平台的 web/ 前端里（PerfTestPage/PerfHistoryPage），这里不需要有。
	perf.DefaultManager.SetCollector(perf.CollectPerformanceMetrics)

	mux := http.NewServeMux()
	server.RegisterAPI(mux)

	port := server.DevPort()
	log.Printf("后端调试服务已启动: http://127.0.0.1:%s", port)
	if err := http.ListenAndServe(":"+port, server.AccessLogMiddleware(server.CORSMiddleware(mux))); err != nil {
		log.Fatalf("后端调试服务启动失败，端口 %s 可能被占用: %v", port, err)
	}
}
