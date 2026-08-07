package stop

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"interface-load-test/client/common"
)

func TestStopCollectPerfRejectsEmptyTaskID(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/collect/perf//stop", nil)
	req.SetPathValue("taskId", "")
	resp := httptest.NewRecorder()

	StopCollectPerf(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, want 200 (业务失败也用 200，见 common.Fail)", resp.Code)
	}

	var body common.Resp
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if body.Code != 10004 {
		t.Fatalf("Resp.Code = %d, want 10004 (msg=%q)", body.Code, body.Msg)
	}
}

func TestStopCollectPerfReturnsFailureForUnknownTask(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/collect/perf/does-not-exist/stop", nil)
	req.SetPathValue("taskId", "does-not-exist")
	resp := httptest.NewRecorder()

	StopCollectPerf(resp, req)

	var body common.Resp
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if body.Code != 10008 {
		t.Fatalf("Resp.Code = %d, want 10008 (msg=%q)", body.Code, body.Msg)
	}
}
