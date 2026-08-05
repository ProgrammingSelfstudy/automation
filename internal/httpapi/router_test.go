package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"interface-load-test/internal/accountstore"
	"interface-load-test/internal/export"
	"interface-load-test/internal/loadtest"
	"interface-load-test/internal/logevent"
	"interface-load-test/internal/resultstore"
	"interface-load-test/internal/task"
	"interface-load-test/internal/taskmanager"
)

func TestCreateAccountOmitsPassword(t *testing.T) {
	deps := newHTTPTestDeps()
	deps.accounts.createFunc = func(ctx context.Context, acc *accountstore.Account) error {
		acc.ID = "acc-1"
		acc.CreatedAt = testHTTPTime()
		return nil
	}
	router := NewRouter(deps.Dependencies())

	resp := serveJSON(t, router, http.MethodPost, "/api/accounts", `{"group_id":"group-1","username":"alice","password":"secret"}`)

	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusCreated, readBody(t, resp.Body))
	}
	body := decodeObject(t, resp.Body)
	assertNoPassword(t, body)
	if got, want := body["id"], "acc-1"; got != want {
		t.Fatalf("id = %v, want %v", got, want)
	}
}

func TestCreateAccountValidationError(t *testing.T) {
	deps := newHTTPTestDeps()
	deps.accounts.createFunc = func(ctx context.Context, acc *accountstore.Account) error {
		return accountstore.ErrUsernameRequired
	}
	router := NewRouter(deps.Dependencies())

	resp := serveJSON(t, router, http.MethodPost, "/api/accounts", `{"group_id":"group-1","password":"secret"}`)

	assertErrorResponse(t, resp, http.StatusBadRequest, accountstore.ErrUsernameRequired.Error())
}

func TestListAccounts(t *testing.T) {
	deps := newHTTPTestDeps()
	deps.accounts.listByGroupFunc = func(ctx context.Context, groupID string) ([]accountstore.Account, error) {
		if groupID != "group-1" {
			t.Fatalf("groupID = %q, want group-1", groupID)
		}
		return []accountstore.Account{
			{ID: "acc-1", GroupID: "group-1", Username: "alice", Password: "secret", Enabled: true, CreatedAt: testHTTPTime()},
		}, nil
	}
	router := NewRouter(deps.Dependencies())

	missing := httptest.NewRecorder()
	router.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/api/accounts", nil))
	assertErrorResponse(t, missing, http.StatusBadRequest, "group_id is required")

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/accounts?group_id=group-1", nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}
	var body []map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got, want := len(body), 1; got != want {
		t.Fatalf("len(body) = %d, want %d", got, want)
	}
	assertNoPassword(t, body[0])
}

func TestCreateTaskFiltersDisabledAccounts(t *testing.T) {
	deps := newHTTPTestDeps()
	deps.accounts.listByGroupFunc = func(ctx context.Context, groupID string) ([]accountstore.Account, error) {
		return []accountstore.Account{
			{ID: "acc-1", Username: "alice", Password: "secret", Enabled: true},
			{ID: "acc-2", Username: "bob", Password: "secret", Enabled: false},
			{ID: "acc-3", Username: "carol", Password: "secret", Enabled: true},
		}, nil
	}
	deps.tasks.createFunc = func(ctx context.Context, req taskmanager.CreateTaskRequest) (*task.Task, error) {
		if got, want := len(req.Accounts), 2; got != want {
			t.Fatalf("CreateTask Accounts length = %d, want %d", got, want)
		}
		if req.Accounts[0].ID != "acc-1" || req.Accounts[1].ID != "acc-3" {
			t.Fatalf("CreateTask Accounts = %#v, want enabled accounts in order", req.Accounts)
		}
		return &task.Task{ID: "task-1", ModuleType: req.ModuleType, Name: req.Name, Status: task.StatusRunning, Config: req.Config, Concurrency: req.Concurrency, TotalCount: 5, CreatedBy: req.CreatedBy, CreatedAt: testHTTPTime()}, nil
	}
	router := NewRouter(deps.Dependencies())

	body := `{"module_type":"load_test","name":"load","config":{"scenario":{"steps":[{"name":"run","method":"GET","url":"https://load.test"}]},"per_account_count":5},"account_group_id":"group-1","concurrency":2,"created_by":"tester"}`
	resp := serveJSON(t, router, http.MethodPost, "/api/tasks", body)

	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusCreated, readBody(t, resp.Body))
	}
	got := decodeObject(t, resp.Body)
	if got["total_count"] != float64(5) {
		t.Fatalf("total_count = %v, want 5", got["total_count"])
	}
}

func TestCreateTaskMapsKnownError(t *testing.T) {
	deps := newHTTPTestDeps()
	deps.accounts.listByGroupFunc = func(ctx context.Context, groupID string) ([]accountstore.Account, error) {
		return []accountstore.Account{{ID: "acc-1", Enabled: true}}, nil
	}
	deps.tasks.createFunc = func(ctx context.Context, req taskmanager.CreateTaskRequest) (*task.Task, error) {
		return nil, taskmanager.ErrConcurrencyExceedsAccounts
	}
	router := NewRouter(deps.Dependencies())

	body := `{"module_type":"load_test","name":"load","config":{},"account_group_id":"group-1","concurrency":3}`
	resp := serveJSON(t, router, http.MethodPost, "/api/tasks", body)

	assertErrorResponse(t, resp, http.StatusBadRequest, taskmanager.ErrConcurrencyExceedsAccounts.Error())
}

func TestListTasks(t *testing.T) {
	t.Run("default filter", func(t *testing.T) {
		deps := newHTTPTestDeps()
		deps.tasks.listFunc = func(ctx context.Context, filter taskmanager.ListFilter) ([]*task.Task, error) {
			if filter.Status != "" || filter.Limit != 0 {
				t.Fatalf("ListTasks filter = %#v, want zero filter", filter)
			}
			return []*task.Task{
				{ID: "task-1", ModuleType: "load_test", Name: "load", Status: task.StatusRunning, Concurrency: 2, TotalCount: 5, CreatedAt: testHTTPTime()},
			}, nil
		}
		router := NewRouter(deps.Dependencies())

		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/tasks", nil))
		if resp.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusOK, resp.Body.String())
		}
		var body []map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		if got, want := body[0]["id"], "task-1"; got != want {
			t.Fatalf("id = %v, want %v", got, want)
		}
	})

	t.Run("status and limit filter", func(t *testing.T) {
		deps := newHTTPTestDeps()
		deps.tasks.listFunc = func(ctx context.Context, filter taskmanager.ListFilter) ([]*task.Task, error) {
			if filter.Status != task.StatusRunning || filter.Limit != 20 {
				t.Fatalf("ListTasks filter = %#v, want running limit 20", filter)
			}
			return []*task.Task{}, nil
		}
		router := NewRouter(deps.Dependencies())

		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/tasks?status=running&limit=20", nil))
		if resp.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusOK, resp.Body.String())
		}
		if strings.TrimSpace(resp.Body.String()) != "[]" {
			t.Fatalf("body = %q, want []", resp.Body.String())
		}
	})

	t.Run("invalid limit", func(t *testing.T) {
		deps := newHTTPTestDeps()
		router := NewRouter(deps.Dependencies())

		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/tasks?limit=many", nil))
		assertErrorResponse(t, resp, http.StatusBadRequest, "limit must be an integer")
	})
}

func TestGetTaskAndCancelTask(t *testing.T) {
	deps := newHTTPTestDeps()
	deps.tasks.getFunc = func(ctx context.Context, taskID string) (*task.Task, error) {
		if taskID == "missing" {
			return nil, taskmanager.ErrNotFound
		}
		return &task.Task{ID: taskID, Status: task.StatusRunning, CreatedAt: testHTTPTime()}, nil
	}
	deps.tasks.cancelFunc = func(ctx context.Context, taskID string) error {
		if taskID == "missing" {
			return taskmanager.ErrNotFound
		}
		return nil
	}
	router := NewRouter(deps.Dependencies())

	getResp := httptest.NewRecorder()
	router.ServeHTTP(getResp, httptest.NewRequest(http.MethodGet, "/api/tasks/task-1", nil))
	if getResp.Code != http.StatusOK {
		t.Fatalf("GET task status = %d, want %d", getResp.Code, http.StatusOK)
	}

	getMissing := httptest.NewRecorder()
	router.ServeHTTP(getMissing, httptest.NewRequest(http.MethodGet, "/api/tasks/missing", nil))
	assertErrorResponse(t, getMissing, http.StatusNotFound, taskmanager.ErrNotFound.Error())

	cancelResp := httptest.NewRecorder()
	router.ServeHTTP(cancelResp, httptest.NewRequest(http.MethodPost, "/api/tasks/task-1/cancel", nil))
	if cancelResp.Code != http.StatusOK {
		t.Fatalf("cancel status = %d, want %d", cancelResp.Code, http.StatusOK)
	}

	cancelMissing := httptest.NewRecorder()
	router.ServeHTTP(cancelMissing, httptest.NewRequest(http.MethodPost, "/api/tasks/missing/cancel", nil))
	assertErrorResponse(t, cancelMissing, http.StatusNotFound, taskmanager.ErrNotFound.Error())
}

func TestGetTaskResults(t *testing.T) {
	deps := newHTTPTestDeps()
	deps.tasks.getFunc = func(ctx context.Context, taskID string) (*task.Task, error) {
		if taskID == "missing" {
			return nil, taskmanager.ErrNotFound
		}
		return &task.Task{ID: taskID, CreatedAt: testHTTPTime()}, nil
	}
	deps.results.groups = []resultstore.AccountResults{
		{
			AccountID:   "acc-1",
			AccountName: "alice",
			Rows: []resultstore.ResultRow{
				{ID: 1, TaskID: "task-1", AccountID: "acc-1", AccountName: "alice", SeqNo: 1, Success: true, CostMs: 10, CreatedAt: testHTTPTime()},
			},
		},
		{
			AccountID:   "acc-2",
			AccountName: "bob",
			Rows: []resultstore.ResultRow{
				{ID: 2, TaskID: "task-1", AccountID: "acc-2", AccountName: "bob", SeqNo: 2, Success: false, ErrMsg: "boom", CostMs: 20, CreatedAt: testHTTPTime()},
			},
		},
	}
	router := NewRouter(deps.Dependencies())

	missingTask := httptest.NewRecorder()
	router.ServeHTTP(missingTask, httptest.NewRequest(http.MethodGet, "/api/tasks/missing/results", nil))
	assertErrorResponse(t, missingTask, http.StatusNotFound, taskmanager.ErrNotFound.Error())

	empty := httptest.NewRecorder()
	router.ServeHTTP(empty, httptest.NewRequest(http.MethodGet, "/api/tasks/task-1/results?account_id=missing-acc", nil))
	if empty.Code != http.StatusOK || strings.TrimSpace(empty.Body.String()) != "[]" {
		t.Fatalf("empty results = status %d body %q, want 200 []", empty.Code, empty.Body.String())
	}

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/tasks/task-1/results?account_id=acc-1", nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusOK)
	}
	var filtered []struct {
		AccountID   string           `json:"account_id"`
		AccountName string           `json:"account_name"`
		Rows        []map[string]any `json:"rows"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &filtered); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got, want := len(filtered), 1; got != want {
		t.Fatalf("len(filtered) = %d, want %d", got, want)
	}
	if got, want := filtered[0].AccountID, "acc-1"; got != want {
		t.Fatalf("account_id = %q, want %q", got, want)
	}
	if got, want := filtered[0].Rows[0]["seq_no"], float64(1); got != want {
		t.Fatalf("seq_no = %v, want %v", got, want)
	}

	all := httptest.NewRecorder()
	router.ServeHTTP(all, httptest.NewRequest(http.MethodGet, "/api/tasks/task-1/results", nil))
	if all.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", all.Code, http.StatusOK)
	}
	var allGroups []struct {
		AccountID   string           `json:"account_id"`
		AccountName string           `json:"account_name"`
		Rows        []map[string]any `json:"rows"`
	}
	if err := json.Unmarshal(all.Body.Bytes(), &allGroups); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got, want := len(allGroups), 2; got != want {
		t.Fatalf("len(allGroups) = %d, want %d", got, want)
	}
	if got, want := allGroups[1].Rows[0]["err_msg"], "boom"; got != want {
		t.Fatalf("err_msg = %v, want %v", got, want)
	}
}

func TestCORS(t *testing.T) {
	deps := newHTTPTestDeps()
	deps.allowedOrigins = []string{"http://localhost:5173"}
	router := NewRouter(deps.Dependencies())

	allowed := httptest.NewRecorder()
	allowedReq := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	allowedReq.Header.Set("Origin", "http://localhost:5173")
	router.ServeHTTP(allowed, allowedReq)
	if got, want := allowed.Header().Get("Access-Control-Allow-Origin"), "http://localhost:5173"; got != want {
		t.Fatalf("allowed origin header = %q, want %q", got, want)
	}

	disallowed := httptest.NewRecorder()
	disallowedReq := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	disallowedReq.Header.Set("Origin", "http://evil.local")
	router.ServeHTTP(disallowed, disallowedReq)
	if got := disallowed.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("disallowed origin header = %q, want empty", got)
	}

	options := httptest.NewRecorder()
	optionsReq := httptest.NewRequest(http.MethodOptions, "/api/tasks", nil)
	optionsReq.Header.Set("Origin", "http://localhost:5173")
	optionsReq.Header.Set("Access-Control-Request-Method", http.MethodPost)
	router.ServeHTTP(options, optionsReq)
	if options.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS status = %d, want %d", options.Code, http.StatusNoContent)
	}
	if got, want := options.Header().Get("Access-Control-Allow-Origin"), "http://localhost:5173"; got != want {
		t.Fatalf("OPTIONS allowed origin = %q, want %q", got, want)
	}
}

func TestExportTask(t *testing.T) {
	deps := newHTTPTestDeps()
	deps.results.groups = []resultstore.AccountResults{
		{
			AccountID:   "acc-1",
			AccountName: "alice",
			Rows: []resultstore.ResultRow{
				{SeqNo: 1, CostMs: 10, Success: true, CreatedAt: testHTTPTime()},
			},
		},
	}
	router := NewRouter(deps.Dependencies())

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/tasks/task-1/export", nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}
	if got := resp.Header().Get("Content-Type"); got != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := resp.Header().Get("Content-Disposition"); got != `attachment; filename="task-task-1.xlsx"` {
		t.Fatalf("Content-Disposition = %q", got)
	}
	workbook, err := excelize.OpenReader(bytes.NewReader(resp.Body.Bytes()))
	if err != nil {
		t.Fatalf("OpenReader() error = %v", err)
	}
	defer workbook.Close()
	if got, err := workbook.GetCellValue("alice", "A2"); err != nil || got != "1" {
		t.Fatalf("alice!A2 = %q, %v; want 1", got, err)
	}

	deps.results.groups = nil
	empty := httptest.NewRecorder()
	router.ServeHTTP(empty, httptest.NewRequest(http.MethodGet, "/api/tasks/task-1/export", nil))
	assertErrorResponse(t, empty, http.StatusNotFound, export.ErrNoResults.Error())
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		err     error
		status  int
		message string
	}{
		{taskmanager.ErrNotFound, http.StatusNotFound, taskmanager.ErrNotFound.Error()},
		{accountstore.ErrNotFound, http.StatusNotFound, accountstore.ErrNotFound.Error()},
		{export.ErrNoResults, http.StatusNotFound, export.ErrNoResults.Error()},
		{taskmanager.ErrNoAccounts, http.StatusBadRequest, taskmanager.ErrNoAccounts.Error()},
		{taskmanager.ErrInvalidConcurrency, http.StatusBadRequest, taskmanager.ErrInvalidConcurrency.Error()},
		{taskmanager.ErrConcurrencyExceedsAccounts, http.StatusBadRequest, taskmanager.ErrConcurrencyExceedsAccounts.Error()},
		{taskmanager.ErrInvalidTotalCount, http.StatusBadRequest, taskmanager.ErrInvalidTotalCount.Error()},
		{taskmanager.ErrUnknownModuleType, http.StatusBadRequest, taskmanager.ErrUnknownModuleType.Error()},
		{accountstore.ErrGroupIDRequired, http.StatusBadRequest, accountstore.ErrGroupIDRequired.Error()},
		{accountstore.ErrUsernameRequired, http.StatusBadRequest, accountstore.ErrUsernameRequired.Error()},
		{accountstore.ErrPasswordRequired, http.StatusBadRequest, accountstore.ErrPasswordRequired.Error()},
		{loadtest.ErrInvalidConfig, http.StatusBadRequest, loadtest.ErrInvalidConfig.Error()},
		{loadtest.ErrScenarioStepsRequired, http.StatusBadRequest, loadtest.ErrScenarioStepsRequired.Error()},
		{loadtest.ErrPerAccountCount, http.StatusBadRequest, loadtest.ErrPerAccountCount.Error()},
	}

	for _, tt := range tests {
		t.Run(tt.err.Error(), func(t *testing.T) {
			status, message := classifyError(tt.err)
			if status != tt.status || message != tt.message {
				t.Fatalf("classifyError() = (%d,%q), want (%d,%q)", status, message, tt.status, tt.message)
			}
		})
	}

	status, message := classifyError(errors.New("boom"))
	if status != http.StatusInternalServerError {
		t.Fatalf("unknown status = %d, want %d", status, http.StatusInternalServerError)
	}
	if message != internalErrorMessage || strings.Contains(message, "boom") {
		t.Fatalf("unknown message = %q, want fixed internal message", message)
	}

	var value any
	syntaxErr := json.Unmarshal([]byte(`{"x":`), &value)
	if syntaxErr == nil {
		t.Fatal("json.Unmarshal() error = nil, want syntax error")
	}
	status, message = classifyError(syntaxErr)
	if status != http.StatusBadRequest {
		t.Fatalf("syntax error status = %d, want %d", status, http.StatusBadRequest)
	}
	if message == internalErrorMessage || message == "" {
		t.Fatalf("syntax error message = %q, want client-facing parse error", message)
	}
}

func TestCreateTaskInvalidConfigReturnsBadRequest(t *testing.T) {
	accountStore := &fakeHTTPAccountStore{
		listByGroupFunc: func(ctx context.Context, groupID string) ([]accountstore.Account, error) {
			return []accountstore.Account{{ID: "acc-1", Username: "alice", Password: "secret", Enabled: true}}, nil
		},
	}
	taskStore := &fakeHTTPTaskStore{}
	registry := task.NewModuleRegistry()
	registry.Register(loadtest.NewModule(nil, nil))
	manager := taskmanager.NewManager(taskStore, registry)
	router := NewRouter(Dependencies{
		TaskManager:  manager,
		AccountStore: accountStore,
		ResultStore:  &fakeHTTPResultStore{},
		Hub:          logevent.NewHub(),
	})

	tests := []struct {
		name string
		body string
	}{
		{
			name: "missing config",
			body: `{"module_type":"load_test","name":"load","account_group_id":"group-1","concurrency":1}`,
		},
		{
			name: "config is json string",
			body: `{"module_type":"load_test","name":"load","config":"not-json{{{","account_group_id":"group-1","concurrency":1}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := serveJSON(t, router, http.MethodPost, "/api/tasks", tt.body)
			if resp.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusBadRequest, resp.Body.String())
			}
			var body map[string]string
			if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}
			if body["error"] == "" || body["error"] == internalErrorMessage {
				t.Fatalf("error = %q, want meaningful client error", body["error"])
			}
			if got := taskStore.InsertCount(); got != 0 {
				t.Fatalf("InsertCount = %d, want 0", got)
			}
		})
	}
}

func TestTaskExistsFunc(t *testing.T) {
	deps := newHTTPTestDeps()
	deps.tasks.getFunc = func(ctx context.Context, taskID string) (*task.Task, error) {
		switch taskID {
		case "task-1":
			return &task.Task{ID: "task-1"}, nil
		case "missing":
			return nil, taskmanager.ErrNotFound
		default:
			return nil, errors.New("db down")
		}
	}
	exists := NewTaskExistsFunc(deps.tasks)

	ok, err := exists(context.Background(), "task-1")
	if err != nil || !ok {
		t.Fatalf("task-1 exists = (%v,%v), want (true,nil)", ok, err)
	}
	ok, err = exists(context.Background(), "missing")
	if err != nil || ok {
		t.Fatalf("missing exists = (%v,%v), want (false,nil)", ok, err)
	}
	ok, err = exists(context.Background(), "error")
	if err == nil || ok {
		t.Fatalf("error exists = (%v,%v), want (false,error)", ok, err)
	}
}

type httpTestDeps struct {
	tasks          *fakeHTTPTaskManager
	accounts       *fakeHTTPAccountStore
	results        *fakeHTTPResultStore
	hub            *logevent.Hub
	allowedOrigins []string
}

func newHTTPTestDeps() *httpTestDeps {
	return &httpTestDeps{
		tasks:    &fakeHTTPTaskManager{},
		accounts: &fakeHTTPAccountStore{},
		results:  &fakeHTTPResultStore{},
		hub:      logevent.NewHub(),
	}
}

func (d *httpTestDeps) Dependencies() Dependencies {
	return Dependencies{
		TaskManager:    d.tasks,
		AccountStore:   d.accounts,
		ResultStore:    d.results,
		Hub:            d.hub,
		AllowedOrigins: d.allowedOrigins,
	}
}

type fakeHTTPTaskManager struct {
	mu         sync.Mutex
	createReqs []taskmanager.CreateTaskRequest
	createFunc func(context.Context, taskmanager.CreateTaskRequest) (*task.Task, error)
	getFunc    func(context.Context, string) (*task.Task, error)
	listFunc   func(context.Context, taskmanager.ListFilter) ([]*task.Task, error)
	cancelFunc func(context.Context, string) error
}

func (m *fakeHTTPTaskManager) CreateTask(ctx context.Context, req taskmanager.CreateTaskRequest) (*task.Task, error) {
	m.mu.Lock()
	m.createReqs = append(m.createReqs, req)
	m.mu.Unlock()
	if m.createFunc != nil {
		return m.createFunc(ctx, req)
	}
	return &task.Task{ID: "task-1", Status: task.StatusRunning, Config: req.Config, CreatedAt: testHTTPTime()}, nil
}

func (m *fakeHTTPTaskManager) GetTask(ctx context.Context, taskID string) (*task.Task, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, taskID)
	}
	return &task.Task{ID: taskID, CreatedAt: testHTTPTime()}, nil
}

func (m *fakeHTTPTaskManager) ListTasks(ctx context.Context, filter taskmanager.ListFilter) ([]*task.Task, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, filter)
	}
	return []*task.Task{}, nil
}

func (m *fakeHTTPTaskManager) CancelTask(ctx context.Context, taskID string) error {
	if m.cancelFunc != nil {
		return m.cancelFunc(ctx, taskID)
	}
	return nil
}

type fakeHTTPAccountStore struct {
	createFunc      func(context.Context, *accountstore.Account) error
	listByGroupFunc func(context.Context, string) ([]accountstore.Account, error)
}

func (s *fakeHTTPAccountStore) Create(ctx context.Context, acc *accountstore.Account) error {
	if s.createFunc != nil {
		return s.createFunc(ctx, acc)
	}
	acc.ID = "acc-1"
	return nil
}

func (s *fakeHTTPAccountStore) Get(context.Context, string) (*accountstore.Account, error) {
	return nil, accountstore.ErrNotFound
}

func (s *fakeHTTPAccountStore) ListByGroup(ctx context.Context, groupID string) ([]accountstore.Account, error) {
	if s.listByGroupFunc != nil {
		return s.listByGroupFunc(ctx, groupID)
	}
	return nil, nil
}

type fakeHTTPResultStore struct {
	groups []resultstore.AccountResults
	err    error
}

func (s *fakeHTTPResultStore) ListByTaskGroupedByAccount(context.Context, string) ([]resultstore.AccountResults, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.groups, nil
}

type fakeHTTPTaskStore struct {
	mu          sync.Mutex
	insertCount int
}

func (s *fakeHTTPTaskStore) Insert(context.Context, *task.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.insertCount++
	return nil
}

func (s *fakeHTTPTaskStore) Get(context.Context, string) (*task.Task, error) {
	return nil, taskmanager.ErrNotFound
}

func (s *fakeHTTPTaskStore) List(context.Context, taskmanager.ListFilter) ([]*task.Task, error) {
	return nil, nil
}

func (s *fakeHTTPTaskStore) MarkRunning(context.Context, string, time.Time) error {
	return nil
}

func (s *fakeHTTPTaskStore) MarkFinished(context.Context, string, task.Status, time.Time, int, int, string) error {
	return nil
}

func (s *fakeHTTPTaskStore) InsertCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.insertCount
}

func serveJSON(t *testing.T, handler http.Handler, method string, target string, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	return resp
}

func decodeObject(t *testing.T, r io.Reader) map[string]any {
	t.Helper()

	var object map[string]any
	if err := json.NewDecoder(r).Decode(&object); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	return object
}

func assertNoPassword(t *testing.T, object map[string]any) {
	t.Helper()

	if _, ok := object["password"]; ok {
		t.Fatalf("response contains password field: %#v", object)
	}
}

func assertErrorResponse(t *testing.T, resp *httptest.ResponseRecorder, status int, message string) {
	t.Helper()

	if resp.Code != status {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, status, resp.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got := body["error"]; got != message {
		t.Fatalf("error = %q, want %q", got, message)
	}
}

func readBody(t *testing.T, r io.Reader) string {
	t.Helper()

	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	return string(data)
}

func testHTTPTime() time.Time {
	return time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
}
