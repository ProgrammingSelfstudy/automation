package server

import (
	"bufio"
	"errors"
	"log"
	"net"
	"net/http"
	"time"
)

// AccessLogMiddleware 记录一行请求日志：方法、路径、状态码、耗时。
//
// Gin 默认自带请求日志中间件；改成标准库之后如果不补一个，这部分可观测性
// 会静默消失——所以照 interface-load-test 那边 internal/httpapi/accesslog.go
// 的方式补一个一致的。
func AccessLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		log.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.status, time.Since(start))
	})
}

// statusRecorder 包一层 http.ResponseWriter 来拿到状态码。
//
// 必须转发 Hijack 和 Unwrap——Unwrap 是 net/http 的 ResponseController 用来
// 找到底层 ResponseWriter 的机制，coder/websocket 升级连接时依赖它。这层
// 包装如果两个都不转发，会让 /ws/collect/perf/:taskId 的每一次连接升级都
// 报错"不支持 hijack"，静默把新加的 WebSocket 推送整个弄挂。
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("server: underlying ResponseWriter does not support hijacking")
	}
	return hijacker.Hijack()
}

func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}
