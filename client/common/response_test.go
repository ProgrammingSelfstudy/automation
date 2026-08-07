package common

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestSuccessWritesEnvelope(t *testing.T) {
	w := httptest.NewRecorder()

	Success(w, map[string]string{"foo": "bar"})

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}

	var resp Resp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if resp.Code != 0 || resp.Msg != "success" {
		t.Fatalf("resp = %#v, want code=0 msg=success", resp)
	}
}

func TestFailWritesEnvelopeWithHTTP200(t *testing.T) {
	w := httptest.NewRecorder()

	Fail(w, 10004, "device_id 不能为空")

	// 项目约定：业务失败也返回 HTTP 200，前端只解析 Resp.Code，不看 HTTP 状态码。
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp Resp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if resp.Code != 10004 || resp.Msg != "device_id 不能为空" || resp.Data != nil {
		t.Fatalf("resp = %#v", resp)
	}
}
