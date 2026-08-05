package taskmanager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"interface-load-test/internal/accountpool"
	"interface-load-test/internal/task"
)

func TestManagerCreateTaskSuccessFlow(t *testing.T) {
	store := newFakeStore()
	module := newFakeModule("load_test")
	module.executionCount = 4
	module.runFunc = func(ctx context.Context, tk *task.Task, accounts []*accountpool.Account) (task.RunResult, error) {
		return task.RunResult{SuccessCount: 3, FailCount: 1}, nil
	}
	manager := newTestManager(store, module)
	accounts := makeManagerAccounts(2)

	tk, err := manager.CreateTask(context.Background(), CreateTaskRequest{
		ModuleType:  "load_test",
		Name:        "task",
		Config:      []byte(`{"ok":true}`),
		Accounts:    accounts,
		Concurrency: 2,
		CreatedBy:   "tester",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if tk.Status != task.StatusRunning {
		t.Fatalf("CreateTask() Status = %q, want %q", tk.Status, task.StatusRunning)
	}
	if tk.StartedAt == nil {
		t.Fatal("CreateTask() StartedAt is nil")
	}
	if got, want := tk.TotalCount, 4; got != want {
		t.Fatalf("CreateTask() TotalCount = %d, want %d", got, want)
	}

	call := module.waitRunCall(t)
	if call.task.ID != tk.ID {
		t.Fatalf("module task ID = %q, want %q", call.task.ID, tk.ID)
	}
	if got, want := len(call.accounts), len(accounts); got != want {
		t.Fatalf("module accounts count = %d, want %d", got, want)
	}

	finished := store.waitFinished(t)
	if finished.status != task.StatusSuccess {
		t.Fatalf("finished status = %q, want %q", finished.status, task.StatusSuccess)
	}
	if finished.successCount != 3 || finished.failCount != 1 {
		t.Fatalf("finished counts = (%d,%d), want (3,1)", finished.successCount, finished.failCount)
	}
	if finished.errMsg != "" {
		t.Fatalf("finished errMsg = %q, want empty", finished.errMsg)
	}
	if got := store.InsertCount(); got != 1 {
		t.Fatalf("InsertCount = %d, want 1", got)
	}
}

func TestManagerRejectsConcurrencyExceedingAccountsBeforeInsert(t *testing.T) {
	store := newFakeStore()
	manager := newTestManager(store, newFakeModule("load_test"))

	_, err := manager.CreateTask(context.Background(), CreateTaskRequest{
		ModuleType:  "load_test",
		Config:      []byte(`{}`),
		Accounts:    makeManagerAccounts(1),
		Concurrency: 2,
	})
	if !errors.Is(err, ErrConcurrencyExceedsAccounts) {
		t.Fatalf("CreateTask() error = %v, want %v", err, ErrConcurrencyExceedsAccounts)
	}
	if got := store.InsertCount(); got != 0 {
		t.Fatalf("InsertCount = %d, want 0", got)
	}
}

func TestManagerRejectsBasicInvalidRequestsBeforeInsert(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*CreateTaskRequest)
		wantErr error
	}{
		{
			name: "no accounts",
			mutate: func(req *CreateTaskRequest) {
				req.Accounts = nil
			},
			wantErr: ErrNoAccounts,
		},
		{
			name: "invalid concurrency",
			mutate: func(req *CreateTaskRequest) {
				req.Concurrency = 0
			},
			wantErr: ErrInvalidConcurrency,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeStore()
			manager := newTestManager(store, newFakeModule("load_test"))
			req := validCreateRequest()
			tt.mutate(&req)

			_, err := manager.CreateTask(context.Background(), req)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("CreateTask() error = %v, want %v", err, tt.wantErr)
			}
			if got := store.InsertCount(); got != 0 {
				t.Fatalf("InsertCount = %d, want 0", got)
			}
		})
	}
}

func TestManagerRejectsInvalidExecutionCountBeforeInsert(t *testing.T) {
	store := newFakeStore()
	module := newFakeModule("load_test")
	module.executionCount = 0
	manager := newTestManager(store, module)

	_, err := manager.CreateTask(context.Background(), validCreateRequest())
	if !errors.Is(err, ErrInvalidTotalCount) {
		t.Fatalf("CreateTask() error = %v, want %v", err, ErrInvalidTotalCount)
	}
	if got := store.InsertCount(); got != 0 {
		t.Fatalf("InsertCount = %d, want 0", got)
	}
}

func TestManagerRejectsUnknownModuleBeforeInsert(t *testing.T) {
	store := newFakeStore()
	manager := NewManager(store, task.NewModuleRegistry())

	_, err := manager.CreateTask(context.Background(), CreateTaskRequest{
		ModuleType:  "missing",
		Config:      []byte(`{}`),
		Accounts:    makeManagerAccounts(1),
		Concurrency: 1,
	})
	if !errors.Is(err, ErrUnknownModuleType) {
		t.Fatalf("CreateTask() error = %v, want %v", err, ErrUnknownModuleType)
	}
	if got := store.InsertCount(); got != 0 {
		t.Fatalf("InsertCount = %d, want 0", got)
	}
}

func TestManagerPropagatesValidateConfigErrorBeforeInsert(t *testing.T) {
	store := newFakeStore()
	wantErr := errors.New("bad config")
	module := newFakeModule("load_test")
	module.validateErr = wantErr
	manager := newTestManager(store, module)

	_, err := manager.CreateTask(context.Background(), CreateTaskRequest{
		ModuleType:  "load_test",
		Config:      []byte(`{"bad":true}`),
		Accounts:    makeManagerAccounts(1),
		Concurrency: 1,
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("CreateTask() error = %v, want %v", err, wantErr)
	}
	if got := store.InsertCount(); got != 0 {
		t.Fatalf("InsertCount = %d, want 0", got)
	}
}

func TestManagerMarksFailedWhenModuleRunReturnsError(t *testing.T) {
	store := newFakeStore()
	module := newFakeModule("load_test")
	module.runFunc = func(ctx context.Context, tk *task.Task, accounts []*accountpool.Account) (task.RunResult, error) {
		return task.RunResult{SuccessCount: 1, FailCount: 2}, errors.New("module failed")
	}
	manager := newTestManager(store, module)

	_, err := manager.CreateTask(context.Background(), validCreateRequest())
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	finished := store.waitFinished(t)
	if finished.status != task.StatusFailed {
		t.Fatalf("finished status = %q, want %q", finished.status, task.StatusFailed)
	}
	if finished.successCount != 1 || finished.failCount != 2 {
		t.Fatalf("finished counts = (%d,%d), want (1,2)", finished.successCount, finished.failCount)
	}
	if finished.errMsg == "" {
		t.Fatal("finished errMsg is empty, want module error")
	}
}

func TestManagerRunContextDoesNotInheritCreateTaskContext(t *testing.T) {
	store := newFakeStore()
	module := newFakeModule("load_test")
	runChecked := make(chan bool, 1)
	module.runFunc = func(ctx context.Context, tk *task.Task, accounts []*accountpool.Account) (task.RunResult, error) {
		time.Sleep(50 * time.Millisecond)
		runChecked <- ctx.Err() == nil
		return task.RunResult{SuccessCount: 1}, nil
	}
	manager := newTestManager(store, module)

	createCtx, cancel := context.WithCancel(context.Background())
	_, err := manager.CreateTask(createCtx, validCreateRequest())
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	cancel()

	select {
	case ok := <-runChecked:
		if !ok {
			t.Fatal("module run context was canceled by CreateTask caller context")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for module context check")
	}

	finished := store.waitFinished(t)
	if finished.status != task.StatusSuccess {
		t.Fatalf("finished status = %q, want %q", finished.status, task.StatusSuccess)
	}
}

func TestManagerCancelTaskCancelsRunningModule(t *testing.T) {
	store := newFakeStore()
	module := newFakeModule("load_test")
	ctxCanceled := make(chan struct{})
	module.runFunc = func(ctx context.Context, tk *task.Task, accounts []*accountpool.Account) (task.RunResult, error) {
		<-ctx.Done()
		close(ctxCanceled)
		return task.RunResult{SuccessCount: 1}, nil
	}
	manager := newTestManager(store, module)

	tk, err := manager.CreateTask(context.Background(), validCreateRequest())
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	module.waitRunCall(t)

	if err := manager.CancelTask(context.Background(), tk.ID); err != nil {
		t.Fatalf("CancelTask() error = %v", err)
	}

	select {
	case <-ctxCanceled:
	case <-time.After(time.Second):
		t.Fatal("module context was not canceled")
	}

	finished := store.waitFinished(t)
	if finished.status != task.StatusCanceled {
		t.Fatalf("finished status = %q, want %q", finished.status, task.StatusCanceled)
	}
}

func TestManagerCancelTaskMissing(t *testing.T) {
	manager := newTestManager(newFakeStore(), newFakeModule("load_test"))

	err := manager.CancelTask(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("CancelTask() error = %v, want %v", err, ErrNotFound)
	}
}

func TestManagerListTasksDelegatesToStore(t *testing.T) {
	store := newFakeStore()
	manager := newTestManager(store, newFakeModule("load_test"))
	filter := ListFilter{Status: task.StatusRunning, Limit: 20}

	tasks, err := manager.ListTasks(context.Background(), filter)
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("len(ListTasks()) = %d, want 0", len(tasks))
	}
	if store.lastListFilter != filter {
		t.Fatalf("lastListFilter = %#v, want %#v", store.lastListFilter, filter)
	}
}

func TestManagerCancelTaskCompletedIsNoop(t *testing.T) {
	store := newFakeStore()
	module := newFakeModule("load_test")
	module.runFunc = func(ctx context.Context, tk *task.Task, accounts []*accountpool.Account) (task.RunResult, error) {
		return task.RunResult{SuccessCount: 1}, nil
	}
	manager := newTestManager(store, module)

	tk, err := manager.CreateTask(context.Background(), validCreateRequest())
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	finished := store.waitFinished(t)
	if finished.status != task.StatusSuccess {
		t.Fatalf("finished status = %q, want %q", finished.status, task.StatusSuccess)
	}

	if err := manager.CancelTask(context.Background(), tk.ID); err != nil {
		t.Fatalf("CancelTask() after finish error = %v", err)
	}
}

func TestManagerConcurrentCreateAndCancelIsSafe(t *testing.T) {
	store := newFakeStore()
	module := newFakeModule("load_test")
	module.runFunc = func(ctx context.Context, tk *task.Task, accounts []*accountpool.Account) (task.RunResult, error) {
		select {
		case <-ctx.Done():
			return task.RunResult{}, nil
		case <-time.After(5 * time.Millisecond):
			return task.RunResult{SuccessCount: 1}, nil
		}
	}
	manager := newTestManager(store, module)

	var wg sync.WaitGroup
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			req := validCreateRequest()
			req.Name = fmt.Sprintf("task-%d", i)
			tk, err := manager.CreateTask(context.Background(), req)
			if err != nil {
				t.Errorf("CreateTask() error = %v", err)
				return
			}
			if i%2 == 0 {
				if err := manager.CancelTask(context.Background(), tk.ID); err != nil {
					t.Errorf("CancelTask() error = %v", err)
				}
			}
		}(i)
	}

	wg.Wait()
	waitForManager(t, time.Second, func() bool {
		return store.FinishedCount() == 25
	})
}

type runCall struct {
	ctx      context.Context
	task     *task.Task
	accounts []*accountpool.Account
}

type fakeTaskModule struct {
	moduleType     string
	validateErr    error
	executionCount int
	executionErr   error
	runFunc        func(context.Context, *task.Task, []*accountpool.Account) (task.RunResult, error)
	runCalls       chan runCall
}

func newFakeModule(moduleType string) *fakeTaskModule {
	return &fakeTaskModule{
		moduleType:     moduleType,
		executionCount: 1,
		runCalls:       make(chan runCall, 100),
	}
}

func (m *fakeTaskModule) Type() string {
	return m.moduleType
}

func (m *fakeTaskModule) ValidateConfig(json.RawMessage) error {
	return m.validateErr
}

func (m *fakeTaskModule) ExecutionCount(json.RawMessage) (int, error) {
	return m.executionCount, m.executionErr
}

func (m *fakeTaskModule) Run(ctx context.Context, tk *task.Task, accounts []*accountpool.Account) (task.RunResult, error) {
	m.runCalls <- runCall{
		ctx:      ctx,
		task:     tk,
		accounts: append([]*accountpool.Account(nil), accounts...),
	}
	if m.runFunc != nil {
		return m.runFunc(ctx, tk, accounts)
	}
	return task.RunResult{}, nil
}

func (m *fakeTaskModule) waitRunCall(t *testing.T) runCall {
	t.Helper()

	select {
	case call := <-m.runCalls:
		return call
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for module Run call")
		return runCall{}
	}
}

type finishedCall struct {
	taskID       string
	status       task.Status
	successCount int
	failCount    int
	errMsg       string
}

type fakeStore struct {
	mu             sync.Mutex
	tasks          map[string]*task.Task
	lastListFilter ListFilter
	insertCount    int
	finishedCalls  []finishedCall
	finishedCh     chan finishedCall
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		tasks:      make(map[string]*task.Task),
		finishedCh: make(chan finishedCall, 100),
	}
}

func (s *fakeStore) Insert(_ context.Context, tk *task.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.insertCount++
	s.tasks[tk.ID] = cloneTask(tk)
	return nil
}

func (s *fakeStore) Get(_ context.Context, taskID string) (*task.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tk, ok := s.tasks[taskID]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneTask(tk), nil
}

func (s *fakeStore) List(_ context.Context, filter ListFilter) ([]*task.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastListFilter = filter
	tasks := make([]*task.Task, 0, len(s.tasks))
	for _, tk := range s.tasks {
		if filter.Status != "" && tk.Status != filter.Status {
			continue
		}
		tasks = append(tasks, cloneTask(tk))
	}
	return tasks, nil
}

func (s *fakeStore) MarkRunning(_ context.Context, taskID string, startedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tk, ok := s.tasks[taskID]
	if !ok {
		return ErrNotFound
	}
	tk.Status = task.StatusRunning
	tk.StartedAt = &startedAt
	return nil
}

func (s *fakeStore) MarkFinished(_ context.Context, taskID string, status task.Status, finishedAt time.Time, successCount, failCount int, errMsg string) error {
	s.mu.Lock()
	tk, ok := s.tasks[taskID]
	if !ok {
		s.mu.Unlock()
		return ErrNotFound
	}
	tk.Status = status
	tk.FinishedAt = &finishedAt
	tk.SuccessCount = successCount
	tk.FailCount = failCount
	tk.ErrMsg = errMsg
	call := finishedCall{
		taskID:       taskID,
		status:       status,
		successCount: successCount,
		failCount:    failCount,
		errMsg:       errMsg,
	}
	s.finishedCalls = append(s.finishedCalls, call)
	s.mu.Unlock()

	s.finishedCh <- call
	return nil
}

func (s *fakeStore) InsertCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.insertCount
}

func (s *fakeStore) FinishedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.finishedCalls)
}

func (s *fakeStore) waitFinished(t *testing.T) finishedCall {
	t.Helper()

	select {
	case call := <-s.finishedCh:
		return call
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for MarkFinished")
		return finishedCall{}
	}
}

func newTestManager(store Store, module task.Module) *Manager {
	registry := task.NewModuleRegistry()
	registry.Register(module)
	return NewManager(store, registry)
}

func validCreateRequest() CreateTaskRequest {
	return CreateTaskRequest{
		ModuleType:  "load_test",
		Name:        "task",
		Config:      []byte(`{"ok":true}`),
		Accounts:    makeManagerAccounts(2),
		Concurrency: 2,
		CreatedBy:   "tester",
	}
}

func makeManagerAccounts(count int) []*accountpool.Account {
	accounts := make([]*accountpool.Account, 0, count)
	for i := 1; i <= count; i++ {
		accounts = append(accounts, &accountpool.Account{
			ID:       fmt.Sprintf("acc-%02d", i),
			Username: fmt.Sprintf("user-%02d", i),
			Password: "secret",
		})
	}
	return accounts
}

func cloneTask(tk *task.Task) *task.Task {
	if tk == nil {
		return nil
	}
	cloned := *tk
	cloned.Config = append([]byte(nil), tk.Config...)
	if tk.StartedAt != nil {
		startedAt := *tk.StartedAt
		cloned.StartedAt = &startedAt
	}
	if tk.FinishedAt != nil {
		finishedAt := *tk.FinishedAt
		cloned.FinishedAt = &finishedAt
	}
	return &cloned
}

func waitForManager(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	if condition() {
		return
	}
	t.Fatalf("condition was not met within %v", timeout)
}
