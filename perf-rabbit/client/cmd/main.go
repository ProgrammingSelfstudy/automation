package main

import (
	"log"
	"net/http"

	"client/internal/perf"
	"client/internal/server"
)

func main() {
	// 开发调试入口：只启动后端接口，不挂前端页面，也不会自动打开浏览器。
	// 这样打包出来的一体化程序占着 8080 时，你本地调试后端仍然可以独立跑。
	perf.DefaultManager.SetCollector(perf.CollectPerformanceMetrics)

	mux := http.NewServeMux()
	server.RegisterAPI(mux)

	port := server.DevPort()
	log.Printf("后端调试服务已启动: http://127.0.0.1:%s", port)
	if err := http.ListenAndServe(":"+port, server.AccessLogMiddleware(server.CORSMiddleware(mux))); err != nil {
		log.Fatalf("后端调试服务启动失败，端口 %s 可能被占用: %v", port, err)
	}
}
