package perf

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"client/common"
)

func TestGetCollectPerfRejectsEmptyTaskID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/collect/perf/", nil)
	req.SetPathValue("taskId", "")
	resp := httptest.NewRecorder()

	GetCollectPerf(resp, req)

	assertFailCode(t, resp, 10004)
}

func TestGetCollectPerfRejectsInvalidFromParam(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/collect/perf/task-1?from=not-a-number", nil)
	req.SetPathValue("taskId", "task-1")
	resp := httptest.NewRecorder()

	GetCollectPerf(resp, req)

	assertFailCode(t, resp, 10004)
}

func TestGetCollectPerfRejectsNegativeFromParam(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/collect/perf/task-1?from=-1", nil)
	req.SetPathValue("taskId", "task-1")
	resp := httptest.NewRecorder()

	GetCollectPerf(resp, req)

	assertFailCode(t, resp, 10004)
}

func TestGetCollectPerfReturnsNotFoundForUnknownTask(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/collect/perf/does-not-exist", nil)
	req.SetPathValue("taskId", "does-not-exist")
	resp := httptest.NewRecorder()

	GetCollectPerf(resp, req)

	assertFailCode(t, resp, 10009)
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
