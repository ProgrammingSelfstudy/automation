package start

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"interface-load-test/client/common"
	"interface-load-test/client/internal/perf"
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

// TestStartCollectPerfReturnsExistingTaskIDOnConflict 验证同一个设备+App
// 重复开始采集时，响应里带着结构化的 task_id（不是只有一句中文错误文案），
// 前端靠这个字段接管显示已经在跑的任务，而不是卡死在报错里
// （见 web/src/pages/PerfTestPage.tsx 的 PerfAgentBusyError 分支）。
func TestStartCollectPerfReturnsExistingTaskIDOnConflict(t *testing.T) {
	defer perf.DefaultManager.SetCollector(nil)
	perf.DefaultManager.SetCollector(func(context.Context, string, string, string, string, string) (perf.MetricSample, error) {
		return perf.MetricSample{}, nil
	})

	body := `{"device_id":"conflict-device","package_name":"com.example.conflict"}`

	first := httptest.NewRecorder()
	StartCollectPerf(first, httptest.NewRequest(http.MethodPost, "/api/collect/perf/start", strings.NewReader(body)))
	var firstResp common.Resp
	if err := json.Unmarshal(first.Body.Bytes(), &firstResp); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if firstResp.Code != 0 {
		t.Fatalf("first start Resp.Code = %d, want 0 (msg=%q)", firstResp.Code, firstResp.Msg)
	}
	firstData, ok := firstResp.Data.(map[string]any)
	if !ok {
		t.Fatalf("first start Data = %#v, want object", firstResp.Data)
	}
	runningTaskID, _ := firstData["task_id"].(string)
	if runningTaskID == "" {
		t.Fatal("first start did not return a task_id")
	}
	defer perf.DefaultManager.Stop(context.Background(), runningTaskID)

	second := httptest.NewRecorder()
	StartCollectPerf(second, httptest.NewRequest(http.MethodPost, "/api/collect/perf/start", strings.NewReader(body)))
	assertFailCode(t, second, 10011)

	var secondResp common.Resp
	if err := json.Unmarshal(second.Body.Bytes(), &secondResp); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	secondData, ok := secondResp.Data.(map[string]any)
	if !ok {
		t.Fatalf("conflict response Data = %#v, want object with task_id", secondResp.Data)
	}
	if got, _ := secondData["task_id"].(string); got != runningTaskID {
		t.Fatalf("conflict response task_id = %q, want %q", got, runningTaskID)
	}
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
