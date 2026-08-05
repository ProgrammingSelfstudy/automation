package httpapi

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"interface-load-test/internal/accountstore"
	"interface-load-test/internal/export"
	"interface-load-test/internal/resultstore"
	"interface-load-test/internal/task"
	"interface-load-test/internal/taskmanager"
)

const maxCreateTaskBodyBytes int64 = 1 << 20

type createTaskRequest struct {
	ModuleType     string          `json:"module_type"`
	Name           string          `json:"name"`
	Config         json.RawMessage `json:"config"`
	AccountGroupID string          `json:"account_group_id"`
	Concurrency    int             `json:"concurrency"`
	CreatedBy      string          `json:"created_by"`
}

type taskResponse struct {
	ID           string          `json:"id"`
	ModuleType   string          `json:"module_type"`
	Name         string          `json:"name"`
	Status       task.Status     `json:"status"`
	Config       json.RawMessage `json:"config"`
	Concurrency  int             `json:"concurrency"`
	TotalCount   int             `json:"total_count"`
	SuccessCount int             `json:"success_count"`
	FailCount    int             `json:"fail_count"`
	ErrMsg       string          `json:"err_msg,omitempty"`
	CreatedBy    string          `json:"created_by"`
	CreatedAt    time.Time       `json:"created_at"`
	StartedAt    *time.Time      `json:"started_at,omitempty"`
	FinishedAt   *time.Time      `json:"finished_at,omitempty"`
}

type resultRowResponse struct {
	ID            int64           `json:"id"`
	TaskID        string          `json:"task_id"`
	AccountID     string          `json:"account_id"`
	AccountName   string          `json:"account_name"`
	SeqNo         int             `json:"seq_no"`
	Steps         json.RawMessage `json:"steps"`
	FormulaResult float64         `json:"formula_result"`
	Success       bool            `json:"success"`
	ErrMsg        string          `json:"err_msg,omitempty"`
	CostMs        int64           `json:"cost_ms"`
	CreatedAt     time.Time       `json:"created_at"`
}

type accountResultsResponse struct {
	AccountID   string              `json:"account_id"`
	AccountName string              `json:"account_name"`
	Rows        []resultRowResponse `json:"rows"`
}

func (h *handler) createTask(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCreateTaskBodyBytes)

	var req createTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "invalid json")
		return
	}
	if req.AccountGroupID == "" {
		writeBadRequest(w, "account_group_id is required")
		return
	}

	accounts, err := h.deps.AccountStore.ListByGroup(r.Context(), req.AccountGroupID)
	if err != nil {
		writeError(w, err)
		return
	}

	poolAccounts := accountstore.ToPoolAccounts(accountstore.FilterEnabled(accounts))
	t, err := h.deps.TaskManager.CreateTask(r.Context(), taskmanager.CreateTaskRequest{
		ModuleType:  req.ModuleType,
		Name:        req.Name,
		Config:      req.Config,
		Accounts:    poolAccounts,
		Concurrency: req.Concurrency,
		CreatedBy:   req.CreatedBy,
	})
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, newTaskResponse(t))
}

func (h *handler) listTasks(w http.ResponseWriter, r *http.Request) {
	filter := taskmanager.ListFilter{
		Status: task.Status(r.URL.Query().Get("status")),
	}
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil {
			writeBadRequest(w, "limit must be an integer")
			return
		}
		filter.Limit = limit
	}

	tasks, err := h.deps.TaskManager.ListTasks(r.Context(), filter)
	if err != nil {
		writeError(w, err)
		return
	}

	responses := make([]taskResponse, 0, len(tasks))
	for _, t := range tasks {
		responses = append(responses, newTaskResponse(t))
	}
	writeJSON(w, http.StatusOK, responses)
}

func (h *handler) getTask(w http.ResponseWriter, r *http.Request) {
	t, err := h.deps.TaskManager.GetTask(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, newTaskResponse(t))
}

func (h *handler) cancelTask(w http.ResponseWriter, r *http.Request) {
	if err := h.deps.TaskManager.CancelTask(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"canceled": true})
}

func (h *handler) getTaskResults(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	accountID := r.URL.Query().Get("account_id")

	if _, err := h.deps.TaskManager.GetTask(r.Context(), taskID); err != nil {
		writeError(w, err)
		return
	}

	groups, err := h.deps.ResultStore.ListByTaskGroupedByAccount(r.Context(), taskID)
	if err != nil {
		writeError(w, err)
		return
	}

	responses := newAccountResultsResponses(groups)
	if accountID == "" {
		writeJSON(w, http.StatusOK, responses)
		return
	}

	for _, group := range groups {
		if group.AccountID == accountID {
			writeJSON(w, http.StatusOK, []accountResultsResponse{newAccountResultsResponse(group)})
			return
		}
	}
	writeJSON(w, http.StatusOK, []accountResultsResponse{})
}

func (h *handler) exportTask(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	file, err := export.ExportTask(r.Context(), h.deps.ResultStore, taskID)
	if err != nil {
		writeError(w, err)
		return
	}
	defer file.Close()

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="task-%s.xlsx"`, taskID))
	if err := file.Write(w); err != nil {
		log.Printf("httpapi write export: %v", err)
	}
}

func newTaskResponse(t *task.Task) taskResponse {
	if t == nil {
		return taskResponse{}
	}
	return taskResponse{
		ID:           t.ID,
		ModuleType:   t.ModuleType,
		Name:         t.Name,
		Status:       t.Status,
		Config:       t.Config,
		Concurrency:  t.Concurrency,
		TotalCount:   t.TotalCount,
		SuccessCount: t.SuccessCount,
		FailCount:    t.FailCount,
		ErrMsg:       t.ErrMsg,
		CreatedBy:    t.CreatedBy,
		CreatedAt:    t.CreatedAt,
		StartedAt:    t.StartedAt,
		FinishedAt:   t.FinishedAt,
	}
}

func newResultRowResponses(rows []resultstore.ResultRow) []resultRowResponse {
	responses := make([]resultRowResponse, 0, len(rows))
	for _, row := range rows {
		responses = append(responses, resultRowResponse{
			ID:            row.ID,
			TaskID:        row.TaskID,
			AccountID:     row.AccountID,
			AccountName:   row.AccountName,
			SeqNo:         row.SeqNo,
			Steps:         row.Steps,
			FormulaResult: row.FormulaResult,
			Success:       row.Success,
			ErrMsg:        row.ErrMsg,
			CostMs:        row.CostMs,
			CreatedAt:     row.CreatedAt,
		})
	}
	return responses
}

func newAccountResultsResponses(groups []resultstore.AccountResults) []accountResultsResponse {
	responses := make([]accountResultsResponse, 0, len(groups))
	for _, group := range groups {
		responses = append(responses, newAccountResultsResponse(group))
	}
	return responses
}

func newAccountResultsResponse(group resultstore.AccountResults) accountResultsResponse {
	return accountResultsResponse{
		AccountID:   group.AccountID,
		AccountName: group.AccountName,
		Rows:        newResultRowResponses(group.Rows),
	}
}
