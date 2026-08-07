package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"interface-load-test/internal/perfstore"
)

// 性能采集的 samples 是一段完整时间序列（一次采集可能几十上百 KB），
// 比其他接口的请求体大得多，单独给一个更宽松的上限。
const maxPerfTaskBodyBytes int64 = 8 << 20

type perfTaskRequest struct {
	DeviceID         string          `json:"device_id"`
	PackageName      string          `json:"package_name"`
	ProcessName      string          `json:"process_name"`
	Platform         string          `json:"platform"`
	DeviceModel      string          `json:"device_model,omitempty"`
	Status           string          `json:"status"`
	StartTime        time.Time       `json:"start_time"`
	StopTime         time.Time       `json:"stop_time"`
	SampleIntervalMS int64           `json:"sample_interval_ms"`
	SampleCount      int             `json:"sample_count"`
	LastError        string          `json:"last_error,omitempty"`
	Samples          json.RawMessage `json:"samples"`
}

type perfTaskSummaryResponse struct {
	ID               string    `json:"id"`
	UserID           string    `json:"user_id"`
	DeviceID         string    `json:"device_id"`
	PackageName      string    `json:"package_name"`
	ProcessName      string    `json:"process_name"`
	Platform         string    `json:"platform"`
	DeviceModel      string    `json:"device_model,omitempty"`
	Status           string    `json:"status"`
	StartTime        time.Time `json:"start_time"`
	StopTime         time.Time `json:"stop_time"`
	SampleIntervalMS int64     `json:"sample_interval_ms"`
	SampleCount      int       `json:"sample_count"`
	LastError        string    `json:"last_error,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

type perfTaskResponse struct {
	perfTaskSummaryResponse
	Samples json.RawMessage `json:"samples"`
}

// createPerfTask 接收一次已经结束的性能采集完整记录。上报方式见架构文档：
// 本地 Agent 只负责浏览器直连实时采集，最终记录由浏览器带着登录态转发到这里
// 写库，不是 Agent 直接调用（这样不用单独给 Agent 设计一套鉴权）。
func (h *handler) createPerfTask(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxPerfTaskBodyBytes)

	var req perfTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "invalid json")
		return
	}

	user := currentUser(r.Context())
	task := &perfstore.PerfTask{
		PerfTaskSummary: perfstore.PerfTaskSummary{
			UserID:           user.ID,
			DeviceID:         req.DeviceID,
			PackageName:      req.PackageName,
			ProcessName:      req.ProcessName,
			Platform:         req.Platform,
			DeviceModel:      req.DeviceModel,
			Status:           req.Status,
			StartTime:        req.StartTime,
			StopTime:         req.StopTime,
			SampleIntervalMS: req.SampleIntervalMS,
			SampleCount:      req.SampleCount,
			LastError:        req.LastError,
		},
		Samples: req.Samples,
	}
	if err := h.deps.PerfStore.Create(r.Context(), task); err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, newPerfTaskSummaryResponse(task.PerfTaskSummary))
}

// listPerfTasks 返回历史摘要，不含 samples——列表页不需要每条几十 KB 的时间序列。
func (h *handler) listPerfTasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := h.deps.PerfStore.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}

	responses := make([]perfTaskSummaryResponse, 0, len(tasks))
	for _, t := range tasks {
		responses = append(responses, newPerfTaskSummaryResponse(t))
	}
	writeJSON(w, http.StatusOK, responses)
}

// getPerfTask 返回单条完整记录，包含 samples，用于图表回放。
func (h *handler) getPerfTask(w http.ResponseWriter, r *http.Request) {
	task, err := h.deps.PerfStore.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, perfTaskResponse{
		perfTaskSummaryResponse: newPerfTaskSummaryResponse(task.PerfTaskSummary),
		Samples:                 task.Samples,
	})
}

func (h *handler) deletePerfTask(w http.ResponseWriter, r *http.Request) {
	if err := h.deps.PerfStore.Delete(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func newPerfTaskSummaryResponse(t perfstore.PerfTaskSummary) perfTaskSummaryResponse {
	return perfTaskSummaryResponse{
		ID:               t.ID,
		UserID:           t.UserID,
		DeviceID:         t.DeviceID,
		PackageName:      t.PackageName,
		ProcessName:      t.ProcessName,
		Platform:         t.Platform,
		DeviceModel:      t.DeviceModel,
		Status:           t.Status,
		StartTime:        t.StartTime,
		StopTime:         t.StopTime,
		SampleIntervalMS: t.SampleIntervalMS,
		SampleCount:      t.SampleCount,
		LastError:        t.LastError,
		CreatedAt:        t.CreatedAt,
	}
}
