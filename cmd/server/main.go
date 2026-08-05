package main

import (
	"context"
	"encoding/base64"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"

	"interface-load-test/internal/accountstore"
	"interface-load-test/internal/auth"
	"interface-load-test/internal/authstore"
	"interface-load-test/internal/httpapi"
	"interface-load-test/internal/loadtest"
	"interface-load-test/internal/logevent"
	"interface-load-test/internal/resultstore"
	"interface-load-test/internal/scenariostore"
	"interface-load-test/internal/task"
	"interface-load-test/internal/taskmanager"
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
	authStore, err := authstore.NewMySQLStore(dsn)
	if err != nil {
		log.Fatalf("create auth store: %v", err)
	}
	authService := auth.NewService(authStore)
	bootstrapAdmin(context.Background(), authStore, authService)

	hub := logevent.NewHub()
	registry := task.NewModuleRegistry()
	registry.Register(loadtest.NewModule(hub, resultStore))
	manager := taskmanager.NewManager(taskStore, registry)

	router := httpapi.NewRouter(httpapi.Dependencies{
		TaskManager:    manager,
		AccountStore:   accountStore,
		ResultStore:    resultStore,
		ScenarioStore:  scenarioStore,
		AuthService:    authService,
		Hub:            hub,
		AllowedOrigins: csvEnvOrDefault("ALLOWED_ORIGINS", "http://localhost:5173"),
		CookieSecure:   boolEnv("COOKIE_SECURE", false),
	})

	addr := envOrDefault("LISTEN_ADDR", ":8080")
	log.Printf("listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, router))
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
	log.Printf("bootstrap admin user %q created; log in and set up 2FA", username)
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
