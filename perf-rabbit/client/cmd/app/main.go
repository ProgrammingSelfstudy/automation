package main

import (
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"client/internal/perf"
	"client/internal/server"
	"client/internal/webapp"
)

func main() {
	// 打包入口：启动后端接口 + 内置前端页面，并自动打开浏览器。
	perf.DefaultManager.SetCollector(perf.CollectPerformanceMetrics)

	mux := http.NewServeMux()
	server.RegisterAPI(mux)
	webapp.Register(mux)

	port := server.AppPort()
	url := "http://127.0.0.1:" + port
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("一体化程序启动失败，端口 %s 可能被占用: %v", port, err)
	}

	log.Printf("一体化程序已启动: %s", url)
	if shouldOpenBrowser() {
		go func() {
			time.Sleep(700 * time.Millisecond)
			openBrowser(url)
		}()
	}

	_ = http.Serve(listener, server.AccessLogMiddleware(server.CORSMiddleware(mux)))
}

func shouldOpenBrowser() bool {
	// 默认自动打开浏览器；如果不想弹页面，可以设置 PERF_RABBIT_OPEN_BROWSER=false。
	value := strings.ToLower(strings.TrimSpace(os.Getenv("PERF_RABBIT_OPEN_BROWSER")))
	return value != "false" && value != "0" && value != "no"
}

func openBrowser(url string) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}

	_ = cmd.Start()
}
