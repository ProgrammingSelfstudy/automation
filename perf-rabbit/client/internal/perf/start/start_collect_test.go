package start

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"interface-load-test/perf-rabbit/client/common"
)

func TestStartCollectPerfRejectsMalformedJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/collect/perf/start", strings.NewReader("{not json"))
	resp := httptest.NewRecorder()

	StartCollectPerf(resp, req)

	assertFailCode(t, resp, 10004)
}

func TestStartCollectPerfRejectsEmptyDeviceID(t *testing.T) {
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/collect/perf/start",
		strings.NewReader(`{"device_id":"  ","package_name":"com.example.app"}`),
	)
	resp := httptest.NewRecorder()

	StartCollectPerf(resp, req)

	assertFailCode(t, resp, 10004)
}

func TestStartCollectPerfRejectsEmptyPackageName(t *testing.T) {
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/collect/perf/start",
		strings.NewReader(`{"device_id":"device-1","package_name":""}`),
	)
	resp := httptest.NewRecorder()

	StartCollectPerf(resp, req)

	assertFailCode(t, resp, 10004)
}

func assertFailCode(t *testing.T, resp *httptest.ResponseRecorder, wantCode int) {
	t.Helper()

	if resp.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, want 200 (业务失败也用 200，见 common.Fail)", resp.Code)
	}

	var body common.Resp
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if body.Code != wantCode {
		t.Fatalf("Resp.Code = %d, want %d (msg=%q)", body.Code, wantCode, body.Msg)
	}
}
