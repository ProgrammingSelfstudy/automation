package httpapi

import (
	"context"
	"errors"
	"net/http"

	"interface-load-test/internal/accountstore"
	"interface-load-test/internal/logevent"
	"interface-load-test/internal/resultstore"
	"interface-load-test/internal/task"
	"interface-load-test/internal/taskmanager"
	"interface-load-test/internal/wsapi"
)

// TaskManager is the minimal task manager dependency used by HTTP handlers.
type TaskManager interface {
	CreateTask(ctx context.Context, req taskmanager.CreateTaskRequest) (*task.Task, error)
	GetTask(ctx context.Context, taskID string) (*task.Task, error)
	ListTasks(ctx context.Context, filter taskmanager.ListFilter) ([]*task.Task, error)
	CancelTask(ctx context.Context, taskID string) error
}

// ResultStore is the minimal result store dependency used by HTTP handlers.
type ResultStore interface {
	ListByTaskGroupedByAccount(ctx context.Context, taskID string) ([]resultstore.AccountResults, error)
}

// Dependencies contains HTTP handler dependencies.
type Dependencies struct {
	TaskManager    TaskManager
	AccountStore   accountstore.Store
	ResultStore    ResultStore
	Hub            *logevent.Hub
	AllowedOrigins []string
}

type handler struct {
	deps Dependencies
}

// NewRouter registers API and WebSocket routes.
func NewRouter(deps Dependencies) http.Handler {
	h := &handler{deps: deps}
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/accounts", h.createAccount)
	mux.HandleFunc("GET /api/accounts", h.listAccounts)
	mux.HandleFunc("POST /api/tasks", h.createTask)
	mux.HandleFunc("GET /api/tasks", h.listTasks)
	mux.HandleFunc("GET /api/tasks/{id}", h.getTask)
	mux.HandleFunc("POST /api/tasks/{id}/cancel", h.cancelTask)
	mux.HandleFunc("GET /api/tasks/{id}/results", h.getTaskResults)
	mux.HandleFunc("GET /api/tasks/{id}/export", h.exportTask)
	mux.Handle("GET /ws/tasks/{id}/progress", wsapi.NewProgressHandler(
		deps.Hub,
		wsapi.WithTaskExists(NewTaskExistsFunc(deps.TaskManager)),
		wsapi.WithAllowedOrigins(deps.AllowedOrigins),
	))

	return withCORS(mux, deps.AllowedOrigins)
}

// NewTaskExistsFunc adapts TaskManager.GetTask to wsapi task existence checks.
func NewTaskExistsFunc(manager TaskManager) wsapi.TaskExistsFunc {
	return func(ctx context.Context, taskID string) (bool, error) {
		_, err := manager.GetTask(ctx, taskID)
		if err == nil {
			return true, nil
		}
		if errors.Is(err, taskmanager.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
}
