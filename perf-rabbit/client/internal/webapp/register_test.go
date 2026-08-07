package webapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegisterServesIndexAtRoot(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.Code)
	}
	if got := resp.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
}

func TestRegisterFallsBackToIndexForSPARoutes(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux)

	// 前端路由（比如 /devices/123），刷新时要落到 index.html 交给前端路由处理，
	// 不能返回 404——跟主平台 web/vite 打包出来的 nginx SPA 兜底是一回事。
	req := httptest.NewRequest(http.MethodGet, "/some/client/side/route", nil)
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (SPA fallback)", resp.Code)
	}
}

func TestRegisterReturnsJSONNotFoundForUnknownAPIRoutes(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux)

	// 没在 RegisterAPI 里注册过的 /api/xxx 路径写错了，要返回 JSON 404，
	// 不能被 "/" 那个兜底路由当成 SPA 页面吃掉——不然调试起来会很费解。
	req := httptest.NewRequest(http.MethodGet, "/api/does-not-exist", nil)
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if body["code"] != float64(404) {
		t.Fatalf("body = %#v, want code=404", body)
	}
}
