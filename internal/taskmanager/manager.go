package taskmanager

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"

	"interface-load-test/internal/accountpool"
	"interface-load-test/internal/task"
)

var (
	ErrNoAccounts                 = errors.New("at least one account is required")
	ErrInvalidConcurrency         = errors.New("concurrency must be positive")
	ErrConcurrencyExceedsAccounts = errors.New("concurrency must not exceed the number of accounts")
	ErrInvalidTotalCount          = errors.New("total count must be positive")
	ErrUnknownModuleType          = errors.New("unknown module type")
)

// Manager validates, persists, starts, and cancels tasks.
type Manager struct {
	store    Store
	registry *task.ModuleRegistry

	mu      sync.Mutex
	running map[string]context.CancelFunc
}

// NewManager creates a task manager.
func NewManager(store Store, registry *task.ModuleRegistry) *Manager {
	if registry == nil {
		registry = task.NewModuleRegistry()
	}
	return &Manager{
		store:    store,
		registry: registry,
		running:  make(map[string]context.CancelFunc),
	}
}

// CreateTask validates, stores, marks running, and starts background execution.
func (m *Manager) CreateTask(ctx context.Context, req CreateTaskRequest) (*task.Task, error) {
	module, totalCount, err := m.validateCreateRequest(req)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	t := &task.Task{
		ID:          uuid.NewString(),
		ModuleType:  req.ModuleType,
		Name:        req.Name,
		Status:      task.StatusPending,
		Config:      append(json.RawMessage(nil), req.Config...),
		Concurrency: req.Concurrency,
		TotalCount:  totalCount,
		CreatedBy:   req.CreatedBy,
		CreatedAt:   now,
	}

	if err := m.store.Insert(ctx, t); err != nil {
		return nil, err
	}

	startedAt := time.Now()
	if err := m.store.MarkRunning(ctx, t.ID, startedAt); err != nil {
		return nil, err
	}

	t.Status = task.StatusRunning
	t.StartedAt = &startedAt

	runCtx, cancel := context.WithCancel(context.Background())
	accounts := append([]*accountpool.Account(nil), req.Accounts...)

	m.mu.Lock()
	m.running[t.ID] = cancel
	m.mu.Unlock()

	go m.runTask(runCtx, t, accounts, module)

	return t, nil
}

// CancelTask cancels a running task if it exists.
func (m *Manager) CancelTask(ctx context.Context, taskID string) error {
	if _, err := m.store.Get(ctx, taskID); err != nil {
		return err
	}

	m.mu.Lock()
	cancel, ok := m.running[taskID]
	m.mu.Unlock()
	if ok {
		cancel()
	}
	return nil
}

// GetTask returns a task by ID.
func (m *Manager) GetTask(ctx context.Context, taskID string) (*task.Task, error) {
	return m.store.Get(ctx, taskID)
}

// ListTasks returns recent tasks with optional filtering.
func (m *Manager) ListTasks(ctx context.Context, filter ListFilter) ([]*task.Task, error) {
	return m.store.List(ctx, filter)
}

type CreateTaskRequest struct {
	ModuleType  string
	Name        string
	Config      json.RawMessage
	Accounts    []*accountpool.Account
	Concurrency int
	CreatedBy   string
}

func (m *Manager) validateCreateRequest(req CreateTaskRequest) (task.Module, int, error) {
	if len(req.Accounts) == 0 {
		return nil, 0, ErrNoAccounts
	}
	if req.Concurrency <= 0 {
		return nil, 0, ErrInvalidConcurrency
	}
	if req.Concurrency > len(req.Accounts) {
		return nil, 0, ErrConcurrencyExceedsAccounts
	}

	module, ok := m.registry.Get(req.ModuleType)
	if !ok {
		return nil, 0, ErrUnknownModuleType
	}
	if err := module.ValidateConfig(req.Config); err != nil {
		return nil, 0, err
	}

	totalCount, err := module.ExecutionCount(req.Config)
	if err != nil {
		return nil, 0, err
	}
	if totalCount <= 0 {
		return nil, 0, ErrInvalidTotalCount
	}
	return module, totalCount, nil
}

func (m *Manager) runTask(runCtx context.Context, t *task.Task, accounts []*accountpool.Account, module task.Module) {
	defer func() {
		m.mu.Lock()
		delete(m.running, t.ID)
		m.mu.Unlock()
	}()

	runResult, err := module.Run(runCtx, t, accounts)

	status := task.StatusSuccess
	errMsg := ""
	if runCtx.Err() != nil {
		status = task.StatusCanceled
		errMsg = runCtx.Err().Error()
	} else if err != nil {
		status = task.StatusFailed
		errMsg = err.Error()
	}

	_ = m.store.MarkFinished(
		context.Background(),
		t.ID,
		status,
		time.Now(),
		runResult.SuccessCount,
		runResult.FailCount,
		truncateUTF8Bytes(errMsg, maxTaskErrMsgBytes),
	)
}
