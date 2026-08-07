package webapp

import (
	"client/web"
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"
)

func Register(mux *http.ServeMux) {
	distFS, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		panic(err)
	}

	fileServer := http.FileServer(http.FS(distFS))

	// Vite 打包后的静态资源路径，例如 /assets/index-xxx.js。
	mux.Handle("GET /assets/", http.StripPrefix("/", fileServer))

	// "/" 是 ServeMux 里优先级最低的兜底模式，只有更具体的模式（比如
	// RegisterAPI 注册的那些 /api/xxx、上面的 /assets/）都没匹配上才会落到
	// 这里——等价于原来 Gin 的首页路由 + NoRoute 兜底二合一。
	// API 写错时仍要返回 404，不能被 index.html 吃掉。
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeNotFound(w)
			return
		}

		serveIndex(w, distFS)
	})
}

func serveIndex(w http.ResponseWriter, distFS fs.FS) {
	data, err := fs.ReadFile(distFS, "index.html")
	if err != nil {
		http.Error(w, "前端页面不存在", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

func writeNotFound(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code": 404,
		"msg":  "接口不存在",
		"data": nil,
	})
}
