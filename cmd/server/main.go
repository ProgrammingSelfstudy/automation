package main

import (
	"context"
	"encoding/base64"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"interface-load-test/internal/accountstore"
	"interface-load-test/internal/auth"
	"interface-load-test/internal/authstore"
	"interface-load-test/internal/environmentstore"
	"interface-load-test/internal/httpapi"
	"interface-load-test/internal/interfacestore"
	"interface-load-test/internal/loadtest"
	"interface-load-test/internal/logevent"
	"interface-load-test/internal/perfstore"
	"interface-load-test/internal/resultstore"
	"interface-load-test/internal/scenario"
	"interface-load-test/internal/scenariostore"
	"interface-load-test/internal/task"
	"interface-load-test/internal/taskmanager"
)

const (
	defaultHTTPShutdownTimeout = 10 * time.Second
	defaultTaskShutdownTimeout = 30 * time.Second
)

func main() {
	dsn := mustEnv("MYSQL_DSN")
	key := mustDecodeBase64Env("ACCOUNT_ENCRYPTION_KEY")
	if len(key) != 32 {
		log.Fatalf("ACCOUNT_ENCRYPTION_KEY must decode to exactly 32 bytes, got %d", len(key))
	}

	accountStore, err := accountstore.NewMySQLStore(dsn, key)
	if err != nil {
		log.Fatalf("create account store: %v", err)
	}
	resultStore, err := resultstore.NewMySQLStore(dsn)
	if err != nil {
		log.Fatalf("create result store: %v", err)
	}
	taskStore, err := taskmanager.NewMySQLStore(dsn)
	if err != nil {
		log.Fatalf("create task store: %v", err)
	}
	scenarioStore, err := scenariostore.NewMySQLStore(dsn)
	if err != nil {
		log.Fatalf("create scenario store: %v", err)
	}
	interfaceStore, err := interfacestore.NewMySQLStore(dsn)
	if err != nil {
		log.Fatalf("create interface store: %v", err)
	}
	environmentStore, err := environmentstore.NewMySQLStore(dsn)
	if err != nil {
		log.Fatalf("create environment store: %v", err)
	}
	authStore, err := authstore.NewMySQLStore(dsn)
	if err != nil {
		log.Fatalf("create auth store: %v", err)
	}
	perfStore, err := perfstore.NewMySQLStore(dsn)
	if err != nil {
		log.Fatalf("create perf store: %v", err)
	}
	authService := auth.NewService(authStore)
	bootstrapAdmin(context.Background(), authStore, authService)
	httpDoer := scenario.NewHTTPClient(0)

	hub := logevent.NewHub()
	registry := task.NewModuleRegistry()
	registry.Register(loadtest.NewModule(hub, resultStore))
	manager := taskmanager.NewManager(taskStore, registry)

	router := httpapi.NewRouter(httpapi.Dependencies{
		TaskManager:        manager,
		AccountStore:       accountStore,
		ResultStore:        resultStore,
		InterfaceStore:     interfaceStore,
		EnvironmentStore:   environmentStore,
		ScenarioStore:      scenarioStore,
		PerfStore:          perfStore,
		PerfAgentAssetsDir: envOrDefault("PERF_AGENT_ASSETS_DIR", "./assets/perf-agent"),
		HTTPDoer:           httpDoer,
		AuthService:        authService,
		Hub:                hub,
		AllowedOrigins:     csvEnvOrDefault("ALLOWED_ORIGINS", "http://localhost:5173"),
		CookieSecure:       boolEnv("COOKIE_SECURE", false),
	})

	addr := envOrDefault("LISTEN_ADDR", ":8080")
	server := &http.Server{
		Addr:    addr,
		Handler: router,
	}
	serverErr := make(chan error, 1)
	go func() {
		log.Printf("listening on %s", addr)
		serverErr <- server.ListenAndServe()
	}()

	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serverErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server: %v", err)
		}
	case <-signalCtx.Done():
		stop()
		log.Printf("shutdown signal received")

		// Stop accepting new HTTP requests (and let in-flight ones finish) before
		// telling the task manager to drain, so no new task can slip in through
		// POST /api/tasks while we're waiting for existing ones to stop.
		//
		// Known limitation: http.Server.Shutdown does not wait on hijacked
		// connections, and a WebSocket upgrade counts as hijacked — so a browser
		// watching /ws/tasks/{id}/progress will see its connection cut abruptly
		// during shutdown rather than closed gracefully. Fixing that would require
		// tracking live WS connections ourselves; not done here.
		shutdownHTTP(server, durationEnvOrDefault("HTTP_SHUTDOWN_TIMEOUT", defaultHTTPShutdownTimeout))
		shutdownTasks(manager, durationEnvOrDefault("TASK_SHUTDOWN_TIMEOUT", defaultTaskShutdownTimeout))
	}
}

func shutdownHTTP(server *http.Server, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("http shutdown: %v", err)
	}
}

func shutdownTasks(manager *taskmanager.Manager, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := manager.Shutdown(ctx); err != nil {
		log.Printf("task shutdown: %v", err)
	}
}

func bootstrapAdmin(ctx context.Context, store authstore.Store, service *auth.Service) {
	username := os.Getenv("BOOTSTRAP_ADMIN_USERNAME")
	if username == "" {
		return
	}
	password := mustEnv("BOOTSTRAP_ADMIN_PASSWORD")
	if _, err := store.GetUserByUsername(ctx, username); err == nil {
		return
	} else if !errors.Is(err, authstore.ErrNotFound) {
		log.Fatalf("bootstrap admin lookup: %v", err)
	}
	if _, err := service.CreateUser(ctx, username, password); err != nil {
		log.Fatalf("bootstrap admin user: %v", err)
	}
	log.Printf("bootstrap admin user %q created without TOTP; configure 2FA before browser login", username)
}

func mustEnv(name string) string {
	value := os.Getenv(name)
	if value == "" {
		log.Fatalf("%s is required", name)
	}
	return value
}

func mustDecodeBase64Env(name string) []byte {
	value := mustEnv(name)
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		log.Fatalf("%s must be base64 encoded: %v", name, err)
	}
	return decoded
}

func envOrDefault(name string, defaultValue string) string {
	value := os.Getenv(name)
	if value == "" {
		return defaultValue
	}
	return value
}

func durationEnvOrDefault(name string, defaultValue time.Duration) time.Duration {
	value := os.Getenv(name)
	if value == "" {
		return defaultValue
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		log.Printf("%s=%q is not a valid duration, using default %s", name, value, defaultValue)
		return defaultValue
	}
	return parsed
}

func csvEnvOrDefault(name string, defaultValue string) []string {
	raw := envOrDefault(name, defaultValue)
	values := strings.Split(raw, ",")
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func boolEnv(name string, defaultValue bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	if value == "" {
		return defaultValue
	}
	return value == "1" || value == "true" || value == "yes" || value == "on"
}
