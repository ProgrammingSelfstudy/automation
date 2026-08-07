package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSMiddlewareReflectsOrigin(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := CORSMiddleware(next)

	req := httptest.NewRequest(http.MethodGet, "/api/device/list", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	if got := resp.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want reflected origin", got)
	}
	if got := resp.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Fatal("Access-Control-Allow-Methods missing")
	}
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (request should reach next handler)", resp.Code)
	}
}

func TestCORSMiddlewareHandlesPreflight(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	handler := CORSMiddleware(next)

	req := httptest.NewRequest(http.MethodOptions, "/api/collect/perf/start", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.Code)
	}
	if called {
		t.Fatal("OPTIONS preflight should not reach the wrapped handler")
	}
}
