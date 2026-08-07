package httpapi

import (
	"context"
	"errors"
	"net/http"

	"interface-load-test/internal/accountstore"
	"interface-load-test/internal/auth"
	"interface-load-test/internal/environmentstore"
	"interface-load-test/internal/interfacestore"
	"interface-load-test/internal/logevent"
	"interface-load-test/internal/perfstore"
	"interface-load-test/internal/resultstore"
	"interface-load-test/internal/scenario"
	"interface-load-test/internal/scenariostore"
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
	TaskManager        TaskManager
	AccountStore       accountstore.Store
	ResultStore        ResultStore
	InterfaceStore     interfacestore.Store
	EnvironmentStore   environmentstore.Store
	ScenarioStore      scenariostore.Store
	PerfStore          perfstore.Store
	PerfAgentAssetsDir string
	HTTPDoer           scenario.HTTPDoer
	AuthService        *auth.Service
	Hub                *logevent.Hub
	AllowedOrigins     []string
	CookieSecure       bool
}

type handler struct {
	deps Dependencies
}

// NewRouter registers API and WebSocket routes.
func NewRouter(deps Dependencies) http.Handler {
	h := &handler{deps: deps}
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/auth/login", h.login)
	mux.HandleFunc("POST /api/auth/totp/setup", h.setupTOTP)
	mux.HandleFunc("POST /api/auth/totp/confirm", h.confirmTOTP)
	mux.HandleFunc("POST /api/auth/logout", requireAuth(deps.AuthService, h.logout))
	mux.HandleFunc("GET /api/auth/me", requireAuth(deps.AuthService, h.me))
	mux.HandleFunc("POST /api/auth/users", requireAuth(deps.AuthService, h.createAuthUser))
	mux.HandleFunc("POST /api/auth/backup-codes/regenerate", requireAuth(deps.AuthService, h.regenerateBackupCodes))

	mux.HandleFunc("POST /api/accounts", requireAuth(deps.AuthService, h.createAccount))
	mux.HandleFunc("GET /api/accounts", requireAuth(deps.AuthService, h.listAccounts))
	mux.HandleFunc("POST /api/interfaces", requireAuth(deps.AuthService, h.createInterface))
	mux.HandleFunc("GET /api/interfaces", requireAuth(deps.AuthService, h.listInterfaces))
	mux.HandleFunc("POST /api/interfaces/try-send", requireAuth(deps.AuthService, h.trySendInterface))
	mux.HandleFunc("PUT /api/interfaces/{id}", requireAuth(deps.AuthService, h.updateInterface))
	mux.HandleFunc("POST /api/environments", requireAuth(deps.AuthService, h.createEnvironment))
	mux.HandleFunc("GET /api/environments", requireAuth(deps.AuthService, h.listEnvironments))
	mux.HandleFunc("PUT /api/environments/{id}", requireAuth(deps.AuthService, h.updateEnvironment))
	mux.HandleFunc("POST /api/scenarios", requireAuth(deps.AuthService, h.createScenario))
	mux.HandleFunc("GET /api/scenarios", requireAuth(deps.AuthService, h.listScenarios))
	mux.HandleFunc("POST /api/perf/tasks", requireAuth(deps.AuthService, h.createPerfTask))
	mux.HandleFunc("GET /api/perf/tasks", requireAuth(deps.AuthService, h.listPerfTasks))
	mux.HandleFunc("GET /api/perf/tasks/{id}", requireAuth(deps.AuthService, h.getPerfTask))
	mux.HandleFunc("DELETE /api/perf/tasks/{id}", requireAuth(deps.AuthService, h.deletePerfTask))
	mux.HandleFunc("GET /api/perf/agent/downloads", requireAuth(deps.AuthService, h.listPerfAgentDownloads))
	mux.HandleFunc("GET /api/perf/agent/downloads/{filename}", requireAuth(deps.AuthService, h.downloadPerfAgent))
	mux.HandleFunc("POST /api/tasks", requireAuth(deps.AuthService, h.createTask))
	mux.HandleFunc("GET /api/tasks", requireAuth(deps.AuthService, h.listTasks))
	mux.HandleFunc("GET /api/tasks/{id}", requireAuth(deps.AuthService, h.getTask))
	mux.HandleFunc("POST /api/tasks/{id}/cancel", requireAuth(deps.AuthService, h.cancelTask))
	mux.HandleFunc("GET /api/tasks/{id}/results", requireAuth(deps.AuthService, h.getTaskResults))
	mux.HandleFunc("GET /api/tasks/{id}/export", requireAuth(deps.AuthService, h.exportTask))
	mux.Handle("GET /ws/tasks/{id}/progress", requireAuthHandler(deps.AuthService, wsapi.NewProgressHandler(
		deps.Hub,
		wsapi.WithTaskExists(NewTaskExistsFunc(deps.TaskManager)),
		wsapi.WithAllowedOrigins(deps.AllowedOrigins),
	)))

	return withAccessLog(withCORS(mux, deps.AllowedOrigins))
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
