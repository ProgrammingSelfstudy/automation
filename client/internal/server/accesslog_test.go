package server

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAccessLogMiddlewareCapturesStatus(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	handler := AccessLogMiddleware(next)

	req := httptest.NewRequest(http.MethodGet, "/api/missing", nil)
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.Code)
	}
}

// fakeHijackableResponseWriter 实现 http.Hijacker，用来验证 statusRecorder
// 把 Hijack 转发到了真正的底层 ResponseWriter，而不是自己吞掉。
type fakeHijackableResponseWriter struct {
	http.ResponseWriter
	hijacked bool
}

func (f *fakeHijackableResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	f.hijacked = true
	return nil, nil, nil
}

func TestStatusRecorderForwardsHijack(t *testing.T) {
	underlying := &fakeHijackableResponseWriter{ResponseWriter: httptest.NewRecorder()}
	rec := &statusRecorder{ResponseWriter: underlying, status: http.StatusOK}

	hijacker, ok := any(rec).(http.Hijacker)
	if !ok {
		t.Fatal("statusRecorder does not implement http.Hijacker")
	}
	if _, _, err := hijacker.Hijack(); err != nil {
		t.Fatalf("Hijack() error = %v", err)
	}
	if !underlying.hijacked {
		t.Fatal("Hijack() was not forwarded to the underlying ResponseWriter")
	}
}

func TestStatusRecorderHijackErrorsWhenUnsupported(t *testing.T) {
	// httptest.NewRecorder() 本身不实现 http.Hijacker。
	rec := &statusRecorder{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}

	hijacker, ok := any(rec).(http.Hijacker)
	if !ok {
		t.Fatal("statusRecorder does not implement http.Hijacker")
	}
	if _, _, err := hijacker.Hijack(); err == nil {
		t.Fatal("Hijack() error = nil, want an error when the underlying writer can't hijack")
	}
}

// TestStatusRecorderUnwraps 这个测试保护的是曾经在 interface-load-test 平台
// 出现过、这里照抄同一套修复的那类 bug：包装 ResponseWriter 时如果不转发
// Unwrap()，net/http 的 ResponseController（coder/websocket 升级连接靠它
// 找到真正能 hijack 的底层 writer）就找不到底层对象，WebSocket 升级会在
// AccessLogMiddleware 包了一层之后失败。
func TestStatusRecorderUnwraps(t *testing.T) {
	underlying := httptest.NewRecorder()
	rec := &statusRecorder{ResponseWriter: underlying, status: http.StatusOK}

	unwrapper, ok := any(rec).(interface{ Unwrap() http.ResponseWriter })
	if !ok {
		t.Fatal("statusRecorder does not implement Unwrap() http.ResponseWriter")
	}
	if unwrapper.Unwrap() != http.ResponseWriter(underlying) {
		t.Fatal("Unwrap() did not return the underlying ResponseWriter")
	}
}
