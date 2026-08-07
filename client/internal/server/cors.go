package server

import "net/http"

// CORSMiddleware 允许任意来源跨域访问。
//
// 这个进程只监听 127.0.0.1，是用户自己下载运行在自己电脑上的本地 Agent，
// 不是公网服务；中心平台前端（无论跑在 localhost:5173 开发端口还是生产
// 域名）都要能直接从浏览器调用它，且不带 cookie / 不需要登录态，所以用
// 反射 Origin（等价于允许所有来源）没有额外安全代价——本来跨域 POST 在
// 没有 CORS 头的情况下也已经能执行，CORS 头缺失只是让浏览器读不到返回值。
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
