package task

import (
	"context"
	"encoding/json"
	"time"

	"interface-load-test/internal/accountpool"
)

// Status is the lifecycle state of a task.
type Status string

const (
	StatusPending  Status = "pending"
	StatusRunning  Status = "running"
	StatusSuccess  Status = "success"
	StatusFailed   Status = "failed"
	StatusCanceled Status = "canceled"
)

// Task is the platform-level execution task.
type Task struct {
	ID           string
	ModuleType   string
	Name         string
	Status       Status
	Config       json.RawMessage
	Concurrency  int
	TotalCount   int
	SuccessCount int
	FailCount    int
	ErrMsg       string
	CreatedBy    string
	CreatedAt    time.Time
	StartedAt    *time.Time
	FinishedAt   *time.Time
}

// RunResult summarizes a module run.
type RunResult struct {
	SuccessCount int
	FailCount    int
}

// Module is implemented by business modules such as load testing.
type Module interface {
	Type() string
	ValidateConfig(cfg json.RawMessage) error
	ExecutionCount(cfg json.RawMessage) (int, error)
	Run(ctx context.Context, t *Task, accounts []*accountpool.Account) (RunResult, error)
}
