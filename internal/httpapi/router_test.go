package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/pquerna/otp/totp"
	"github.com/xuri/excelize/v2"
	"golang.org/x/crypto/bcrypt"

	"interface-load-test/internal/accountstore"
	"interface-load-test/internal/auth"
	"interface-load-test/internal/authstore"
	"interface-load-test/internal/environmentstore"
	"interface-load-test/internal/export"
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
	router.ServeHTTP(missing, authenticatedRequest(http.MethodGet, "/api/accounts", nil))
	assertErrorResponse(t, missing, http.StatusBadRequest, "group_id is required")

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, authenticatedRequest(http.MethodGet, "/api/accounts?group_id=group-1", nil))
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

func TestCreateInterface(t *testing.T) {
	deps := newHTTPTestDeps()
	deps.interfaces.createFunc = func(ctx context.Context, iface *interfacestore.Interface) error {
		if iface.Name != "login api" {
			t.Fatalf("interface name = %q, want login api", iface.Name)
		}
		if iface.Step.Method != "POST" || iface.Step.URL != "https://api.test/login" {
			t.Fatalf("interface step = %#v", iface.Step)
		}
		if iface.Step.Headers["Content-Type"] != "application/json" || iface.Step.Extract["token"] != "data.token" {
			t.Fatalf("interface step maps = %#v", iface.Step)
		}
		iface.ID = "interface-1"
		iface.CreatedAt = testHTTPTime()
		return nil
	}
	router := NewRouter(deps.Dependencies())

	body := `{"name":"login api","step":{"name":"login","method":"POST","url":"https://api.test/login","body_tpl":"{}","headers":{"Content-Type":"application/json"},"extract":{"token":"data.token"}}}`
	resp := serveJSON(t, router, http.MethodPost, "/api/interfaces", body)
	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusCreated, resp.Body.String())
	}
	got := decodeObject(t, resp.Body)
	if got["id"] != "interface-1" || got["name"] != "login api" {
		t.Fatalf("response identity = %#v", got)
	}
	step, ok := got["step"].(map[string]any)
	if !ok {
		t.Fatalf("step = %#v, want object", got["step"])
	}
	if step["method"] != "POST" || step["url"] != "https://api.test/login" {
		t.Fatalf("step response = %#v", step)
	}
}

func TestCreateInterfaceValidationError(t *testing.T) {
	deps := newHTTPTestDeps()
	deps.interfaces.createFunc = func(context.Context, *interfacestore.Interface) error {
		return interfacestore.ErrMethodRequired
	}
	router := NewRouter(deps.Dependencies())

	body := `{"name":"login api","step":{"url":"https://api.test/login"}}`
	resp := serveJSON(t, router, http.MethodPost, "/api/interfaces", body)

	assertErrorResponse(t, resp, http.StatusBadRequest, interfacestore.ErrMethodRequired.Error())
}

func TestListInterfaces(t *testing.T) {
	deps := newHTTPTestDeps()
	deps.interfaces.items = []interfacestore.Interface{
		{
			ID:   "interface-1",
			Name: "login api",
			Step: scenario.Step{
				Name:    "login",
				Method:  "POST",
				URL:     "https://api.test/login",
				BodyTpl: "{}",
				Headers: map[string]string{"Content-Type": "application/json"},
				Extract: map[string]string{"token": "data.token"},
			},
			CreatedAt: testHTTPTime(),
		},
	}
	router := NewRouter(deps.Dependencies())

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, authenticatedRequest(http.MethodGet, "/api/interfaces", nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}
	var body []struct {
		ID   string        `json:"id"`
		Name string        `json:"name"`
		Step scenario.Step `json:"step"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(body) != 1 {
		t.Fatalf("len(body) = %d, want 1", len(body))
	}
	if body[0].ID != "interface-1" || body[0].Step.Method != "POST" || body[0].Step.Extract["token"] != "data.token" {
		t.Fatalf("interface response = %#v", body[0])
	}

	deps.interfaces.items = nil
	empty := httptest.NewRecorder()
	router.ServeHTTP(empty, authenticatedRequest(http.MethodGet, "/api/interfaces", nil))
	if empty.Code != http.StatusOK || strings.TrimSpace(empty.Body.String()) != "[]" {
		t.Fatalf("empty interfaces = status %d body %q, want 200 []", empty.Code, empty.Body.String())
	}
}

func TestUpdateInterface(t *testing.T) {
	deps := newHTTPTestDeps()
	deps.interfaces.updateFunc = func(ctx context.Context, iface *interfacestore.Interface) error {
		if iface.ID != "interface-1" {
			t.Fatalf("interface id = %q, want interface-1", iface.ID)
		}
		if iface.Name != "login api updated" {
			t.Fatalf("interface name = %q, want login api updated", iface.Name)
		}
		if iface.Step.Method != "PUT" || iface.Step.URL != "https://api.test/login-v2" {
			t.Fatalf("interface step = %#v", iface.Step)
		}
		iface.CreatedAt = testHTTPTime()
		return nil
	}
	router := NewRouter(deps.Dependencies())

	body := `{"name":"login api updated","step":{"name":"login","method":"PUT","url":"https://api.test/login-v2","body_tpl":"{}","headers":{"Content-Type":"application/json"},"extract":{"token":"data.token"}}}`
	resp := serveJSON(t, router, http.MethodPut, "/api/interfaces/interface-1", body)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}
	got := decodeObject(t, resp.Body)
	if got["id"] != "interface-1" || got["name"] != "login api updated" {
		t.Fatalf("response identity = %#v", got)
	}
	step, ok := got["step"].(map[string]any)
	if !ok {
		t.Fatalf("step = %#v, want object", got["step"])
	}
	if step["method"] != "PUT" || step["url"] != "https://api.test/login-v2" {
		t.Fatalf("step response = %#v", step)
	}
}

func TestUpdateInterfaceNotFound(t *testing.T) {
	deps := newHTTPTestDeps()
	deps.interfaces.updateFunc = func(context.Context, *interfacestore.Interface) error {
		return interfacestore.ErrNotFound
	}
	router := NewRouter(deps.Dependencies())

	body := `{"name":"login api","step":{"method":"POST","url":"https://api.test/login"}}`
	resp := serveJSON(t, router, http.MethodPut, "/api/interfaces/missing", body)

	assertErrorResponse(t, resp, http.StatusNotFound, interfacestore.ErrNotFound.Error())
}

func TestCreateEnvironment(t *testing.T) {
	deps := newHTTPTestDeps()
	deps.environments.createFunc = func(ctx context.Context, env *environmentstore.Environment) error {
		if env.Name != "dev" {
			t.Fatalf("environment name = %q, want dev", env.Name)
		}
		if env.Variables["BASE_URL"] != "https://dev.example.test" {
			t.Fatalf("environment variables = %#v", env.Variables)
		}
		if env.DefaultHeaders["X-Env"] != "{{.env.BASE_URL}}" {
			t.Fatalf("environment default headers = %#v", env.DefaultHeaders)
		}
		env.ID = "env-1"
		env.CreatedAt = testHTTPTime()
		return nil
	}
	router := NewRouter(deps.Dependencies())

	body := `{"name":"dev","variables":{"BASE_URL":"https://dev.example.test"},"default_headers":{"X-Env":"{{.env.BASE_URL}}"}}`
	resp := serveJSON(t, router, http.MethodPost, "/api/environments", body)
	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusCreated, resp.Body.String())
	}
	got := decodeObject(t, resp.Body)
	if got["id"] != "env-1" || got["name"] != "dev" {
		t.Fatalf("response identity = %#v", got)
	}
	variables, ok := got["variables"].(map[string]any)
	if !ok {
		t.Fatalf("variables = %#v, want object", got["variables"])
	}
	if variables["BASE_URL"] != "https://dev.example.test" {
		t.Fatalf("variables response = %#v", variables)
	}
	defaultHeaders, ok := got["default_headers"].(map[string]any)
	if !ok {
		t.Fatalf("default_headers = %#v, want object", got["default_headers"])
	}
	if defaultHeaders["X-Env"] != "{{.env.BASE_URL}}" {
		t.Fatalf("default_headers response = %#v", defaultHeaders)
	}
}

func TestCreateEnvironmentValidationError(t *testing.T) {
	deps := newHTTPTestDeps()
	deps.environments.createFunc = func(context.Context, *environmentstore.Environment) error {
		return environmentstore.ErrNameRequired
	}
	router := NewRouter(deps.Dependencies())

	resp := serveJSON(t, router, http.MethodPost, "/api/environments", `{"variables":{"BASE_URL":"https://dev.example.test"}}`)

	assertErrorResponse(t, resp, http.StatusBadRequest, environmentstore.ErrNameRequired.Error())
}

func TestListEnvironments(t *testing.T) {
	deps := newHTTPTestDeps()
	deps.environments.items = []environmentstore.Environment{
		{
			ID:             "env-1",
			Name:           "dev",
			Variables:      map[string]string{"BASE_URL": "https://dev.example.test"},
			DefaultHeaders: map[string]string{"X-Env": "{{.env.BASE_URL}}"},
			CreatedAt:      testHTTPTime(),
		},
	}
	router := NewRouter(deps.Dependencies())

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, authenticatedRequest(http.MethodGet, "/api/environments", nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}
	var body []struct {
		ID             string            `json:"id"`
		Name           string            `json:"name"`
		Variables      map[string]string `json:"variables"`
		DefaultHeaders map[string]string `json:"default_headers"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(body) != 1 {
		t.Fatalf("len(body) = %d, want 1", len(body))
	}
	if body[0].ID != "env-1" || body[0].Variables["BASE_URL"] != "https://dev.example.test" {
		t.Fatalf("environment response = %#v", body[0])
	}
	if body[0].DefaultHeaders["X-Env"] != "{{.env.BASE_URL}}" {
		t.Fatalf("environment default headers = %#v", body[0])
	}

	deps.environments.items = nil
	empty := httptest.NewRecorder()
	router.ServeHTTP(empty, authenticatedRequest(http.MethodGet, "/api/environments", nil))
	if empty.Code != http.StatusOK || strings.TrimSpace(empty.Body.String()) != "[]" {
		t.Fatalf("empty environments = status %d body %q, want 200 []", empty.Code, empty.Body.String())
	}
}

func TestUpdateEnvironment(t *testing.T) {
	deps := newHTTPTestDeps()
	deps.environments.updateFunc = func(ctx context.Context, env *environmentstore.Environment) error {
		if env.ID != "env-1" {
			t.Fatalf("environment id = %q, want env-1", env.ID)
		}
		if env.Name != "dev updated" {
			t.Fatalf("environment name = %q, want dev updated", env.Name)
		}
		if env.Variables["BASE_URL"] != "https://dev2.example.test" {
			t.Fatalf("environment variables = %#v", env.Variables)
		}
		if env.DefaultHeaders["X-Env"] != "dev2" {
			t.Fatalf("environment default headers = %#v", env.DefaultHeaders)
		}
		env.CreatedAt = testHTTPTime()
		return nil
	}
	router := NewRouter(deps.Dependencies())

	body := `{"name":"dev updated","variables":{"BASE_URL":"https://dev2.example.test"},"default_headers":{"X-Env":"dev2"}}`
	resp := serveJSON(t, router, http.MethodPut, "/api/environments/env-1", body)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}
	got := decodeObject(t, resp.Body)
	if got["id"] != "env-1" || got["name"] != "dev updated" {
		t.Fatalf("response identity = %#v", got)
	}
}

func TestUpdateEnvironmentNotFound(t *testing.T) {
	deps := newHTTPTestDeps()
	deps.environments.updateFunc = func(context.Context, *environmentstore.Environment) error {
		return environmentstore.ErrNotFound
	}
	router := NewRouter(deps.Dependencies())

	body := `{"name":"dev","variables":{"BASE_URL":"https://dev.example.test"}}`
	resp := serveJSON(t, router, http.MethodPut, "/api/environments/missing", body)

	assertErrorResponse(t, resp, http.StatusNotFound, environmentstore.ErrNotFound.Error())
}

func TestTrySendInterfaceRendersWithEnvironmentAndMergedHeaders(t *testing.T) {
	deps := newHTTPTestDeps()
	deps.environments.items = []environmentstore.Environment{
		{
			ID:        "env-1",
			Name:      "dev",
			Variables: map[string]string{"BASE_URL": "https://dev.example.test", "TOKEN": "secret"},
			DefaultHeaders: map[string]string{
				"Authorization": "Bearer {{.env.TOKEN}}",
				"X-Source":      "environment",
			},
		},
	}
	deps.httpDoer.responses = []fakeHTTPResponse{{statusCode: http.StatusAccepted, body: `{"ok":true}`}}
	router := NewRouter(deps.Dependencies())

	body := `{"environment_id":"env-1","step":{"name":"preview","method":"POST","url":"{{.env.BASE_URL}}/users","body_tpl":"{\"token\":\"{{.env.TOKEN}}\"}","headers":{"X-Source":"step","X-Step":"{{.env.TOKEN}}"}}}`
	resp := serveJSON(t, router, http.MethodPost, "/api/interfaces/try-send", body)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}

	var got trySendInterfaceResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got.Error != "" {
		t.Fatalf("error = %q, want empty", got.Error)
	}
	if got.Request.URL != "https://dev.example.test/users" || got.Request.Body != `{"token":"secret"}` {
		t.Fatalf("request = %#v", got.Request)
	}
	if got.Request.Headers["Authorization"] != "Bearer secret" || got.Request.Headers["X-Source"] != "step" || got.Request.Headers["X-Step"] != "secret" {
		t.Fatalf("request headers = %#v", got.Request.Headers)
	}
	if got.Response.StatusCode != http.StatusAccepted || got.Response.Body != `{"ok":true}` || got.Response.Truncated {
		t.Fatalf("response = %#v", got.Response)
	}

	calls := deps.httpDoer.Calls()
	if len(calls) != 1 {
		t.Fatalf("HTTP call count = %d, want 1", len(calls))
	}
	if calls[0].url != got.Request.URL || calls[0].headers["Authorization"] != "Bearer secret" {
		t.Fatalf("HTTP call = %#v", calls[0])
	}
}

func TestTrySendInterfaceReturnsRenderErrorWithoutHTTPCall(t *testing.T) {
	deps := newHTTPTestDeps()
	router := NewRouter(deps.Dependencies())

	body := `{"step":{"name":"preview","method":"GET","url":"https://api.test/{{.account.Username}}"}}`
	resp := serveJSON(t, router, http.MethodPost, "/api/interfaces/try-send", body)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}

	var got trySendInterfaceResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got.Error == "" || !strings.Contains(got.Error, "render url") {
		t.Fatalf("error = %q, want render url error", got.Error)
	}
	if got.Request.URL != "" || got.Response.StatusCode != 0 {
		t.Fatalf("try-send response = %#v", got)
	}
	if calls := deps.httpDoer.Calls(); len(calls) != 0 {
		t.Fatalf("HTTP call count = %d, want 0", len(calls))
	}
}

func TestTrySendInterfaceTruncatesLargeResponse(t *testing.T) {
	deps := newHTTPTestDeps()
	deps.httpDoer.responses = []fakeHTTPResponse{{statusCode: http.StatusOK, body: strings.Repeat("a", trySendMaxResponseBytes+1)}}
	router := NewRouter(deps.Dependencies())

	body := `{"step":{"name":"preview","method":"GET","url":"https://api.test/ping"}}`
	resp := serveJSON(t, router, http.MethodPost, "/api/interfaces/try-send", body)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}

	var got trySendInterfaceResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !got.Response.Truncated {
		t.Fatalf("truncated = false, want true")
	}
	if len(got.Response.Body) != trySendMaxResponseBytes {
		t.Fatalf("response body len = %d, want %d", len(got.Response.Body), trySendMaxResponseBytes)
	}
}

func TestCreateScenario(t *testing.T) {
	deps := newHTTPTestDeps()
	deps.scenarios.createFunc = func(ctx context.Context, scen *scenariostore.Scenario) error {
		if scen.Name != "checkout" {
			t.Fatalf("scenario name = %q, want checkout", scen.Name)
		}
		if scen.Definition.Formula != "score * 2" || len(scen.Definition.Steps) != 1 {
			t.Fatalf("scenario definition = %#v", scen.Definition)
		}
		scen.ID = "scenario-1"
		scen.CreatedAt = testHTTPTime()
		return nil
	}
	router := NewRouter(deps.Dependencies())

	body := `{"name":"checkout","definition":{"steps":[{"name":"login","method":"POST","url":"https://api.test/login","headers":{"Content-Type":"application/json"},"extract":{"token":"data.token"}}],"formula":"score * 2"}}`
	resp := serveJSON(t, router, http.MethodPost, "/api/scenarios", body)
	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusCreated, resp.Body.String())
	}
	got := decodeObject(t, resp.Body)
	if got["id"] != "scenario-1" || got["name"] != "checkout" {
		t.Fatalf("response identity = %#v", got)
	}
	definition, ok := got["definition"].(map[string]any)
	if !ok {
		t.Fatalf("definition = %#v, want object", got["definition"])
	}
	if got, want := definition["formula"], "score * 2"; got != want {
		t.Fatalf("formula = %v, want %v", got, want)
	}
}

func TestCreateScenarioValidationError(t *testing.T) {
	deps := newHTTPTestDeps()
	deps.scenarios.createFunc = func(context.Context, *scenariostore.Scenario) error {
		return scenariostore.ErrNameRequired
	}
	router := NewRouter(deps.Dependencies())

	body := `{"name":"","definition":{"steps":[{"name":"login","method":"POST","url":"https://api.test/login"}]}}`
	resp := serveJSON(t, router, http.MethodPost, "/api/scenarios", body)

	assertErrorResponse(t, resp, http.StatusBadRequest, scenariostore.ErrNameRequired.Error())
}

func TestListScenarios(t *testing.T) {
	deps := newHTTPTestDeps()
	deps.scenarios.items = []scenariostore.Scenario{
		{
			ID:   "scenario-1",
			Name: "checkout",
			Definition: scenario.Scenario{
				Steps: []scenario.Step{
					{Name: "login", Method: "POST", URL: "https://api.test/login"},
					{Name: "profile", Method: "GET", URL: "https://api.test/profile"},
				},
				Formula: "score * 2",
			},
			CreatedAt: testHTTPTime(),
		},
	}
	router := NewRouter(deps.Dependencies())

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, authenticatedRequest(http.MethodGet, "/api/scenarios", nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}
	var body []struct {
		ID         string            `json:"id"`
		Name       string            `json:"name"`
		Definition scenario.Scenario `json:"definition"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(body) != 1 {
		t.Fatalf("len(body) = %d, want 1", len(body))
	}
	if body[0].ID != "scenario-1" || len(body[0].Definition.Steps) != 2 || body[0].Definition.Formula != "score * 2" {
		t.Fatalf("scenario response = %#v", body[0])
	}

	deps.scenarios.items = nil
	empty := httptest.NewRecorder()
	router.ServeHTTP(empty, authenticatedRequest(http.MethodGet, "/api/scenarios", nil))
	if empty.Code != http.StatusOK || strings.TrimSpace(empty.Body.String()) != "[]" {
		t.Fatalf("empty scenarios = status %d body %q, want 200 []", empty.Code, empty.Body.String())
	}
}

func TestCreatePerfTask(t *testing.T) {
	deps := newHTTPTestDeps()
	deps.perfTasks.createFunc = func(ctx context.Context, task *perfstore.PerfTask) error {
		if task.UserID != "user-1" {
			t.Fatalf("perf task user id = %q, want user-1 (from session, not request body)", task.UserID)
		}
		if task.DeviceID != "device-1" || task.Platform != "android" {
			t.Fatalf("perf task fields = %#v", task.PerfTaskSummary)
		}
		if string(task.Samples) != `[{"cpu":1.2}]` {
			t.Fatalf("perf task samples = %s", task.Samples)
		}
		task.ID = 1
		task.CreatedAt = testHTTPTime()
		return nil
	}
	router := NewRouter(deps.Dependencies())

	body := `{"device_id":"device-1","package_name":"com.example.app","process_name":"com.example.app","platform":"android","status":"finished","sample_count":1,"samples":[{"cpu":1.2}]}`
	resp := serveJSON(t, router, http.MethodPost, "/api/perf/tasks", body)
	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusCreated, resp.Body.String())
	}
	got := decodeObject(t, resp.Body)
	if got["id"] != float64(1) || got["user_id"] != "user-1" {
		t.Fatalf("response identity = %#v", got)
	}
	if _, hasSamples := got["samples"]; hasSamples {
		t.Fatalf("create response = %#v, should not echo samples back", got)
	}
}

func TestCreatePerfTaskValidationError(t *testing.T) {
	deps := newHTTPTestDeps()
	deps.perfTasks.createFunc = func(context.Context, *perfstore.PerfTask) error {
		return perfstore.ErrDeviceIDRequired
	}
	router := NewRouter(deps.Dependencies())

	resp := serveJSON(t, router, http.MethodPost, "/api/perf/tasks", `{"platform":"android"}`)

	assertErrorResponse(t, resp, http.StatusBadRequest, perfstore.ErrDeviceIDRequired.Error())
}

func TestListPerfTasksOmitsSamples(t *testing.T) {
	deps := newHTTPTestDeps()
	deps.perfTasks.items = []perfstore.PerfTaskSummary{
		{ID: 1, UserID: "user-1", DeviceID: "device-1", Platform: "android", Status: "finished", CreatedAt: testHTTPTime()},
	}
	router := NewRouter(deps.Dependencies())

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, authenticatedRequest(http.MethodGet, "/api/perf/tasks", nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}
	var body []map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(body) != 1 || body[0]["id"] != float64(1) {
		t.Fatalf("perf task list response = %#v", body)
	}
	if _, hasSamples := body[0]["samples"]; hasSamples {
		t.Fatalf("list response = %#v, should not include samples", body[0])
	}

	deps.perfTasks.items = nil
	empty := httptest.NewRecorder()
	router.ServeHTTP(empty, authenticatedRequest(http.MethodGet, "/api/perf/tasks", nil))
	if empty.Code != http.StatusOK || strings.TrimSpace(empty.Body.String()) != "[]" {
		t.Fatalf("empty perf tasks = status %d body %q, want 200 []", empty.Code, empty.Body.String())
	}
}

func TestGetPerfTaskIncludesSamples(t *testing.T) {
	deps := newHTTPTestDeps()
	deps.perfTasks.items = []perfstore.PerfTaskSummary{
		{ID: 1, UserID: "user-1", DeviceID: "device-1", Platform: "android", Status: "finished", CreatedAt: testHTTPTime()},
	}
	deps.perfTasks.samples = map[int64]json.RawMessage{1: json.RawMessage(`[{"cpu":1.2}]`)}
	router := NewRouter(deps.Dependencies())

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, authenticatedRequest(http.MethodGet, "/api/perf/tasks/1", nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}
	got := decodeObject(t, resp.Body)
	samples, ok := got["samples"].([]any)
	if !ok || len(samples) != 1 {
		t.Fatalf("samples = %#v, want one-element array", got["samples"])
	}
}

func TestGetPerfTaskNotFound(t *testing.T) {
	deps := newHTTPTestDeps()
	router := NewRouter(deps.Dependencies())

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, authenticatedRequest(http.MethodGet, "/api/perf/tasks/999", nil))

	assertErrorResponse(t, resp, http.StatusNotFound, perfstore.ErrNotFound.Error())
}

func TestGetPerfTaskInvalidID(t *testing.T) {
	deps := newHTTPTestDeps()
	router := NewRouter(deps.Dependencies())

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, authenticatedRequest(http.MethodGet, "/api/perf/tasks/not-a-number", nil))

	assertErrorResponse(t, resp, http.StatusBadRequest, "invalid id")
}

func TestDeletePerfTask(t *testing.T) {
	deps := newHTTPTestDeps()
	var deletedID int64
	deps.perfTasks.deleteFunc = func(ctx context.Context, id int64) error {
		deletedID = id
		return nil
	}
	router := NewRouter(deps.Dependencies())

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, authenticatedRequest(http.MethodDelete, "/api/perf/tasks/1", nil))
	if resp.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusNoContent, resp.Body.String())
	}
	if deletedID != 1 {
		t.Fatalf("deleted id = %d, want 1", deletedID)
	}
}

func TestDeletePerfTaskNotFound(t *testing.T) {
	deps := newHTTPTestDeps()
	deps.perfTasks.deleteFunc = func(context.Context, int64) error {
		return perfstore.ErrNotFound
	}
	router := NewRouter(deps.Dependencies())

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, authenticatedRequest(http.MethodDelete, "/api/perf/tasks/999", nil))

	assertErrorResponse(t, resp, http.StatusNotFound, perfstore.ErrNotFound.Error())
}

func TestListPerfAgentDownloadsReportsAvailability(t *testing.T) {
	deps := newHTTPTestDeps()
	deps.perfAgentAssetsDir = t.TempDir()
	if err := os.WriteFile(filepath.Join(deps.perfAgentAssetsDir, "perf-agent-darwin-arm64"), []byte("fake-binary"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	router := NewRouter(deps.Dependencies())

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, authenticatedRequest(http.MethodGet, "/api/perf/agent/downloads", nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}

	var options []perfAgentDownloadOption
	if err := json.Unmarshal(resp.Body.Bytes(), &options); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(options) != len(perfAgentDownloads) {
		t.Fatalf("len(options) = %d, want %d", len(options), len(perfAgentDownloads))
	}

	byFilename := make(map[string]bool)
	for _, option := range options {
		byFilename[option.Filename] = option.Available
	}
	if !byFilename["perf-agent-darwin-arm64"] {
		t.Fatalf("options = %#v, want perf-agent-darwin-arm64 available", options)
	}
	if byFilename["perf-agent-windows-amd64.exe"] {
		t.Fatalf("options = %#v, want perf-agent-windows-amd64.exe unavailable", options)
	}
}

func TestDownloadPerfAgentServesFile(t *testing.T) {
	deps := newHTTPTestDeps()
	deps.perfAgentAssetsDir = t.TempDir()
	if err := os.WriteFile(filepath.Join(deps.perfAgentAssetsDir, "perf-agent-darwin-arm64"), []byte("fake-binary"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	router := NewRouter(deps.Dependencies())

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, authenticatedRequest(http.MethodGet, "/api/perf/agent/downloads/perf-agent-darwin-arm64", nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}
	if resp.Body.String() != "fake-binary" {
		t.Fatalf("body = %q, want %q", resp.Body.String(), "fake-binary")
	}
	if got := resp.Header().Get("Content-Disposition"); !strings.Contains(got, "perf-agent-darwin-arm64") {
		t.Fatalf("Content-Disposition = %q, want it to reference the filename", got)
	}
}

func TestDownloadPerfAgentRejectsUnknownFilename(t *testing.T) {
	deps := newHTTPTestDeps()
	deps.perfAgentAssetsDir = t.TempDir()
	router := NewRouter(deps.Dependencies())

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, authenticatedRequest(http.MethodGet, "/api/perf/agent/downloads/..%2F..%2Fetc%2Fpasswd", nil))
	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusNotFound, resp.Body.String())
	}
}

func TestDownloadPerfAgentMissingFileReturnsNotFound(t *testing.T) {
	deps := newHTTPTestDeps()
	deps.perfAgentAssetsDir = t.TempDir()
	router := NewRouter(deps.Dependencies())

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, authenticatedRequest(http.MethodGet, "/api/perf/agent/downloads/perf-agent-darwin-arm64", nil))
	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusNotFound, resp.Body.String())
	}
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
		if req.CreatedBy != "tester" {
			t.Fatalf("CreateTask CreatedBy = %q, want tester from session", req.CreatedBy)
		}
		return &task.Task{ID: "task-1", ModuleType: req.ModuleType, Name: req.Name, Status: task.StatusRunning, Config: req.Config, Concurrency: req.Concurrency, TotalCount: 5, CreatedBy: req.CreatedBy, CreatedAt: testHTTPTime()}, nil
	}
	router := NewRouter(deps.Dependencies())

	body := `{"module_type":"load_test","name":"load","config":{"scenario":{"steps":[{"name":"run","method":"GET","url":"https://load.test"}]},"per_account_count":5},"account_group_id":"group-1","concurrency":2,"created_by":"mallory"}`
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

func TestRequireAuthMiddleware(t *testing.T) {
	store := newFakeHTTPAuthStore()
	store.usersByID["user-1"] = &authstore.User{ID: "user-1", Username: "tester", TOTPEnabled: true, CreatedAt: testHTTPTime()}
	store.usersByUsername["tester"] = store.usersByID["user-1"]
	store.sessions["valid"] = &authstore.Session{ID: "valid", UserID: "user-1", ExpiresAt: time.Now().Add(time.Hour)}
	store.sessions["expired"] = &authstore.Session{ID: "expired", UserID: "user-1", ExpiresAt: time.Now().Add(-time.Hour)}
	protected := requireAuth(auth.NewService(store), func(w http.ResponseWriter, r *http.Request) {
		user := currentUser(r.Context())
		if user == nil || user.Username != "tester" {
			t.Fatalf("current user = %#v, want tester", user)
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	tests := []struct {
		name       string
		cookie     string
		wantStatus int
	}{
		{name: "no cookie", wantStatus: http.StatusUnauthorized},
		{name: "invalid cookie", cookie: "missing", wantStatus: http.StatusUnauthorized},
		{name: "expired session", cookie: "expired", wantStatus: http.StatusUnauthorized},
		{name: "valid session", cookie: "valid", wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if tt.cookie != "" {
				req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: tt.cookie})
			}
			resp := httptest.NewRecorder()
			protected(resp, req)
			if resp.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", resp.Code, tt.wantStatus, resp.Body.String())
			}
		})
	}
}

func TestProtectedRoutesRequireSession(t *testing.T) {
	deps := newHTTPTestDeps()
	router := NewRouter(deps.Dependencies())

	getTasks := httptest.NewRecorder()
	router.ServeHTTP(getTasks, httptest.NewRequest(http.MethodGet, "/api/tasks", nil))
	assertErrorResponse(t, getTasks, http.StatusUnauthorized, "unauthorized")

	createAccount := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/api/accounts", strings.NewReader(`{"group_id":"group-1","username":"alice","password":"secret"}`))
	createReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(createAccount, createReq)
	assertErrorResponse(t, createAccount, http.StatusUnauthorized, "unauthorized")
}

func TestLoginHTTPRequiresTOTPSetup(t *testing.T) {
	deps := newHTTPTestDeps()
	deps.authStore.usersByID["admin-id"] = &authstore.User{
		ID:           "admin-id",
		Username:     "admin",
		PasswordHash: hashHTTPPasswordForTest(t, "correct-password"),
		TOTPEnabled:  false,
		CreatedAt:    testHTTPTime(),
	}
	deps.authStore.usersByUsername["admin"] = deps.authStore.usersByID["admin-id"]
	router := NewRouter(deps.Dependencies())

	resp := serveJSON(t, router, http.MethodPost, "/api/auth/login", `{"username":"admin","password":"correct-password"}`)

	assertErrorResponse(t, resp, http.StatusUnauthorized, "totp_setup_required")
}

func TestLoginHTTPRequiresTOTPCode(t *testing.T) {
	deps := newHTTPTestDeps()
	secret := testHTTPTOTPSecret(t, "admin")
	deps.authStore.usersByID["admin-id"] = &authstore.User{
		ID:           "admin-id",
		Username:     "admin",
		PasswordHash: hashHTTPPasswordForTest(t, "correct-password"),
		TOTPSecret:   secret,
		TOTPEnabled:  true,
		CreatedAt:    testHTTPTime(),
	}
	deps.authStore.usersByUsername["admin"] = deps.authStore.usersByID["admin-id"]
	router := NewRouter(deps.Dependencies())

	resp := serveJSON(t, router, http.MethodPost, "/api/auth/login", `{"username":"admin","password":"correct-password"}`)

	assertErrorResponse(t, resp, http.StatusUnauthorized, "totp_code_required")
}

func TestLoginHTTPWithTOTPCode(t *testing.T) {
	deps := newHTTPTestDeps()
	secret := testHTTPTOTPSecret(t, "admin")
	deps.authStore.usersByID["admin-id"] = &authstore.User{
		ID:           "admin-id",
		Username:     "admin",
		PasswordHash: hashHTTPPasswordForTest(t, "correct-password"),
		TOTPSecret:   secret,
		TOTPEnabled:  true,
		CreatedAt:    testHTTPTime(),
	}
	deps.authStore.usersByUsername["admin"] = deps.authStore.usersByID["admin-id"]
	router := NewRouter(deps.Dependencies())
	code := currentHTTPTOTPCode(t, secret)

	resp := serveJSON(t, router, http.MethodPost, "/api/auth/login", fmt.Sprintf(`{"username":"admin","password":"correct-password","code":%q}`, code))

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}
	if cookies := resp.Result().Cookies(); len(cookies) == 0 || cookies[0].Name != sessionCookieName || cookies[0].Value == "" {
		t.Fatalf("cookies = %#v, want session cookie", cookies)
	}
	body := decodeObject(t, resp.Body)
	user := body["user"].(map[string]any)
	if user["username"] != "admin" {
		t.Fatalf("username = %v, want admin", user["username"])
	}
}

func TestLoginHTTPWithBackupCode(t *testing.T) {
	deps := newHTTPTestDeps()
	secret := testHTTPTOTPSecret(t, "admin")
	deps.authStore.usersByID["admin-id"] = &authstore.User{
		ID:           "admin-id",
		Username:     "admin",
		PasswordHash: hashHTTPPasswordForTest(t, "correct-password"),
		TOTPSecret:   secret,
		TOTPEnabled:  true,
		CreatedAt:    testHTTPTime(),
	}
	deps.authStore.usersByUsername["admin"] = deps.authStore.usersByID["admin-id"]
	deps.authStore.backupCodes["admin-id"] = []authstore.BackupCodeRef{
		{ID: 7, Hash: hashHTTPPasswordForTest(t, "RECOVERY-CODE")},
	}
	router := NewRouter(deps.Dependencies())

	resp := serveJSON(t, router, http.MethodPost, "/api/auth/login", `{"username":"admin","password":"correct-password","backup_code":"RECOVERY-CODE"}`)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}
	if cookies := resp.Result().Cookies(); len(cookies) == 0 || cookies[0].Name != sessionCookieName || cookies[0].Value == "" {
		t.Fatalf("cookies = %#v, want session cookie", cookies)
	}
	if got := deps.authStore.markedBackupCodeIDs; len(got) != 1 || got[0] != 7 {
		t.Fatalf("marked backup code IDs = %#v, want [7]", got)
	}
}

func TestLoginHTTPRateLimit(t *testing.T) {
	deps := newHTTPTestDeps()
	secret := testHTTPTOTPSecret(t, "admin")
	deps.authStore.usersByID["admin-id"] = &authstore.User{
		ID:           "admin-id",
		Username:     "admin",
		PasswordHash: hashHTTPPasswordForTest(t, "correct-password"),
		TOTPSecret:   secret,
		TOTPEnabled:  true,
		CreatedAt:    testHTTPTime(),
	}
	deps.authStore.usersByUsername["admin"] = deps.authStore.usersByID["admin-id"]
	router := NewRouter(deps.Dependencies())

	for i := 0; i < 5; i++ {
		resp := serveJSON(t, router, http.MethodPost, "/api/auth/login", `{"username":"admin","password":"wrong-password"}`)
		assertErrorResponse(t, resp, http.StatusUnauthorized, auth.ErrInvalidCredentials.Error())
	}

	code := currentHTTPTOTPCode(t, secret)
	resp := serveJSON(t, router, http.MethodPost, "/api/auth/login", fmt.Sprintf(`{"username":"admin","password":"correct-password","code":%q}`, code))
	assertErrorResponse(t, resp, http.StatusTooManyRequests, auth.ErrTooManyAttempts.Error())
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
		router.ServeHTTP(resp, authenticatedRequest(http.MethodGet, "/api/tasks", nil))
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
		router.ServeHTTP(resp, authenticatedRequest(http.MethodGet, "/api/tasks?status=running&limit=20", nil))
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
		router.ServeHTTP(resp, authenticatedRequest(http.MethodGet, "/api/tasks?limit=many", nil))
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
	router.ServeHTTP(getResp, authenticatedRequest(http.MethodGet, "/api/tasks/task-1", nil))
	if getResp.Code != http.StatusOK {
		t.Fatalf("GET task status = %d, want %d", getResp.Code, http.StatusOK)
	}

	getMissing := httptest.NewRecorder()
	router.ServeHTTP(getMissing, authenticatedRequest(http.MethodGet, "/api/tasks/missing", nil))
	assertErrorResponse(t, getMissing, http.StatusNotFound, taskmanager.ErrNotFound.Error())

	cancelResp := httptest.NewRecorder()
	router.ServeHTTP(cancelResp, authenticatedRequest(http.MethodPost, "/api/tasks/task-1/cancel", nil))
	if cancelResp.Code != http.StatusOK {
		t.Fatalf("cancel status = %d, want %d", cancelResp.Code, http.StatusOK)
	}

	cancelMissing := httptest.NewRecorder()
	router.ServeHTTP(cancelMissing, authenticatedRequest(http.MethodPost, "/api/tasks/missing/cancel", nil))
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
	router.ServeHTTP(missingTask, authenticatedRequest(http.MethodGet, "/api/tasks/missing/results", nil))
	assertErrorResponse(t, missingTask, http.StatusNotFound, taskmanager.ErrNotFound.Error())

	empty := httptest.NewRecorder()
	router.ServeHTTP(empty, authenticatedRequest(http.MethodGet, "/api/tasks/task-1/results?account_id=missing-acc", nil))
	if empty.Code != http.StatusOK || strings.TrimSpace(empty.Body.String()) != "[]" {
		t.Fatalf("empty results = status %d body %q, want 200 []", empty.Code, empty.Body.String())
	}

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, authenticatedRequest(http.MethodGet, "/api/tasks/task-1/results?account_id=acc-1", nil))
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
	router.ServeHTTP(all, authenticatedRequest(http.MethodGet, "/api/tasks/task-1/results", nil))
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
	allowedReq := authenticatedRequest(http.MethodGet, "/api/tasks", nil)
	allowedReq.Header.Set("Origin", "http://localhost:5173")
	router.ServeHTTP(allowed, allowedReq)
	if got, want := allowed.Header().Get("Access-Control-Allow-Origin"), "http://localhost:5173"; got != want {
		t.Fatalf("allowed origin header = %q, want %q", got, want)
	}
	if got, want := allowed.Header().Get("Access-Control-Allow-Credentials"), "true"; got != want {
		t.Fatalf("credentials header = %q, want %q", got, want)
	}

	disallowed := httptest.NewRecorder()
	disallowedReq := authenticatedRequest(http.MethodGet, "/api/tasks", nil)
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

	// Regression check: PUT /api/interfaces/{id} and PUT /api/environments/{id}
	// are real routes, so a browser's PUT preflight must be allowed too — a
	// prior version of corsAllowMethods omitted PUT, which made every browser
	// silently refuse to ever send the actual PUT (curl-based checks never
	// caught it because curl doesn't enforce CORS preflight at all).
	if got := options.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, "PUT") {
		t.Fatalf("Access-Control-Allow-Methods = %q, want it to contain PUT", got)
	}

	// Same regression class: DELETE /api/perf/tasks/{id} is a real route too.
	if got := options.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, "DELETE") {
		t.Fatalf("Access-Control-Allow-Methods = %q, want it to contain DELETE", got)
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
	router.ServeHTTP(resp, authenticatedRequest(http.MethodGet, "/api/tasks/task-1/export", nil))
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
	router.ServeHTTP(empty, authenticatedRequest(http.MethodGet, "/api/tasks/task-1/export", nil))
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
		{interfacestore.ErrNameRequired, http.StatusBadRequest, interfacestore.ErrNameRequired.Error()},
		{interfacestore.ErrMethodRequired, http.StatusBadRequest, interfacestore.ErrMethodRequired.Error()},
		{interfacestore.ErrURLRequired, http.StatusBadRequest, interfacestore.ErrURLRequired.Error()},
		{scenariostore.ErrNameRequired, http.StatusBadRequest, scenariostore.ErrNameRequired.Error()},
		{scenariostore.ErrStepsRequired, http.StatusBadRequest, scenariostore.ErrStepsRequired.Error()},
		{auth.ErrInvalidCredentials, http.StatusUnauthorized, auth.ErrInvalidCredentials.Error()},
		{auth.ErrInvalidCode, http.StatusUnauthorized, auth.ErrInvalidCode.Error()},
		{auth.ErrTOTPCodeRequired, http.StatusUnauthorized, auth.ErrTOTPCodeRequired.Error()},
		{auth.ErrTOTPSetupRequired, http.StatusUnauthorized, auth.ErrTOTPSetupRequired.Error()},
		{auth.ErrTooManyAttempts, http.StatusTooManyRequests, auth.ErrTooManyAttempts.Error()},
		{taskmanager.ErrShuttingDown, http.StatusServiceUnavailable, taskmanager.ErrShuttingDown.Error()},
		{authstore.ErrUsernameTaken, http.StatusBadRequest, authstore.ErrUsernameTaken.Error()},
		{auth.ErrWeakPassword, http.StatusBadRequest, auth.ErrWeakPassword.Error()},
		{auth.ErrTOTPAlreadyEnabled, http.StatusBadRequest, auth.ErrTOTPAlreadyEnabled.Error()},
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
	authStore := newFakeHTTPAuthStore()
	authStore.usersByID["user-1"] = &authstore.User{ID: "user-1", Username: "tester", TOTPEnabled: true, CreatedAt: testHTTPTime()}
	authStore.usersByUsername["tester"] = authStore.usersByID["user-1"]
	authStore.sessions["session-1"] = &authstore.Session{ID: "session-1", UserID: "user-1", ExpiresAt: time.Now().Add(time.Hour)}
	router := NewRouter(Dependencies{
		TaskManager:  manager,
		AccountStore: accountStore,
		ResultStore:  &fakeHTTPResultStore{},
		AuthService:  auth.NewService(authStore),
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

// TestWebSocketProgressUpgradesThroughFullRouter dials the WS progress route
// through the complete NewRouter (access log + CORS + auth middleware all
// wrapping the ResponseWriter). This exists specifically to catch a wrapper
// that forwards Write/WriteHeader but not Hijack/Unwrap, which would silently
// turn every WS upgrade into a "does not support hijacking" failure without
// any of the other tests noticing (they exercise the WS handler directly,
// not through this full middleware chain).
func TestWebSocketProgressUpgradesThroughFullRouter(t *testing.T) {
	deps := newHTTPTestDeps()
	server := httptest.NewServer(NewRouter(deps.Dependencies()))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/tasks/task-1/progress"
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	header := http.Header{}
	header.Set("Cookie", (&http.Cookie{Name: sessionCookieName, Value: "session-1"}).String())
	conn, resp, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("Dial() error = %v, status = %d", err, status)
	}
	defer conn.CloseNow()

	if err := conn.Close(websocket.StatusNormalClosure, "test done"); err != nil {
		t.Fatalf("Close() error = %v", err)
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
	tasks              *fakeHTTPTaskManager
	accounts           *fakeHTTPAccountStore
	results            *fakeHTTPResultStore
	interfaces         *fakeHTTPInterfaceStore
	environments       *fakeHTTPEnvironmentStore
	scenarios          *fakeHTTPScenarioStore
	perfTasks          *fakeHTTPPerfStore
	httpDoer           *fakeHTTPDoer
	authStore          *fakeHTTPAuthStore
	hub                *logevent.Hub
	allowedOrigins     []string
	perfAgentAssetsDir string
}

func newHTTPTestDeps() *httpTestDeps {
	authStore := newFakeHTTPAuthStore()
	authStore.usersByID["user-1"] = &authstore.User{
		ID:          "user-1",
		Username:    "tester",
		TOTPEnabled: true,
		CreatedAt:   testHTTPTime(),
	}
	authStore.usersByUsername["tester"] = authStore.usersByID["user-1"]
	authStore.sessions["session-1"] = &authstore.Session{
		ID:        "session-1",
		UserID:    "user-1",
		ExpiresAt: time.Now().Add(time.Hour),
	}
	return &httpTestDeps{
		tasks:        &fakeHTTPTaskManager{},
		accounts:     &fakeHTTPAccountStore{},
		results:      &fakeHTTPResultStore{},
		interfaces:   &fakeHTTPInterfaceStore{},
		environments: &fakeHTTPEnvironmentStore{},
		scenarios:    &fakeHTTPScenarioStore{},
		perfTasks:    &fakeHTTPPerfStore{},
		httpDoer:     &fakeHTTPDoer{},
		authStore:    authStore,
		hub:          logevent.NewHub(),
	}
}

func (d *httpTestDeps) Dependencies() Dependencies {
	return Dependencies{
		TaskManager:        d.tasks,
		AccountStore:       d.accounts,
		ResultStore:        d.results,
		InterfaceStore:     d.interfaces,
		EnvironmentStore:   d.environments,
		ScenarioStore:      d.scenarios,
		PerfStore:          d.perfTasks,
		PerfAgentAssetsDir: d.perfAgentAssetsDir,
		HTTPDoer:           d.httpDoer,
		AuthService:        auth.NewService(d.authStore),
		Hub:                d.hub,
		AllowedOrigins:     d.allowedOrigins,
	}
}

type fakeHTTPAuthStore struct {
	mu                  sync.Mutex
	usersByID           map[string]*authstore.User
	usersByUsername     map[string]*authstore.User
	sessions            map[string]*authstore.Session
	backupCodes         map[string][]authstore.BackupCodeRef
	usedBackupCodeIDs   map[int64]bool
	failedLoginAttempts map[string][]time.Time
	markedBackupCodeIDs []int64
}

func newFakeHTTPAuthStore() *fakeHTTPAuthStore {
	return &fakeHTTPAuthStore{
		usersByID:           make(map[string]*authstore.User),
		usersByUsername:     make(map[string]*authstore.User),
		sessions:            make(map[string]*authstore.Session),
		backupCodes:         make(map[string][]authstore.BackupCodeRef),
		usedBackupCodeIDs:   make(map[int64]bool),
		failedLoginAttempts: make(map[string][]time.Time),
	}
}

func (s *fakeHTTPAuthStore) CreateUser(_ context.Context, user *authstore.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.usersByUsername[user.Username]; ok {
		return authstore.ErrUsernameTaken
	}
	if user.ID == "" {
		user.ID = fmt.Sprintf("user-%d", len(s.usersByID)+1)
	}
	cloned := cloneHTTPUser(user)
	s.usersByID[cloned.ID] = cloned
	s.usersByUsername[cloned.Username] = cloned
	return nil
}

func (s *fakeHTTPAuthStore) GetUserByUsername(_ context.Context, username string) (*authstore.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.usersByUsername[username]
	if !ok {
		return nil, authstore.ErrNotFound
	}
	return cloneHTTPUser(user), nil
}

func (s *fakeHTTPAuthStore) GetUserByID(_ context.Context, id string) (*authstore.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.usersByID[id]
	if !ok {
		return nil, authstore.ErrNotFound
	}
	return cloneHTTPUser(user), nil
}

func (s *fakeHTTPAuthStore) UpdateTOTP(_ context.Context, userID, secret string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.usersByID[userID]
	if !ok {
		return authstore.ErrNotFound
	}
	user.TOTPSecret = secret
	user.TOTPEnabled = enabled
	s.usersByUsername[user.Username] = user
	return nil
}

func (s *fakeHTTPAuthStore) ReplaceBackupCodes(_ context.Context, userID string, hashes []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	refs := make([]authstore.BackupCodeRef, 0, len(hashes))
	for i, hash := range hashes {
		refs = append(refs, authstore.BackupCodeRef{ID: int64(i + 1), Hash: hash})
	}
	s.backupCodes[userID] = refs
	s.usedBackupCodeIDs = make(map[int64]bool)
	return nil
}

func (s *fakeHTTPAuthStore) ListUnusedBackupCodes(_ context.Context, userID string) ([]authstore.BackupCodeRef, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	refs := make([]authstore.BackupCodeRef, 0, len(s.backupCodes[userID]))
	for _, ref := range s.backupCodes[userID] {
		if !s.usedBackupCodeIDs[ref.ID] {
			refs = append(refs, ref)
		}
	}
	return refs, nil
}

func (s *fakeHTTPAuthStore) MarkBackupCodeUsed(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.usedBackupCodeIDs[id] = true
	s.markedBackupCodeIDs = append(s.markedBackupCodeIDs, id)
	return nil
}

func (s *fakeHTTPAuthStore) CreateSession(_ context.Context, session *authstore.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.ID] = &authstore.Session{ID: session.ID, UserID: session.UserID, ExpiresAt: session.ExpiresAt}
	return nil
}

func (s *fakeHTTPAuthStore) GetSession(_ context.Context, id string) (*authstore.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[id]
	if !ok || time.Now().After(session.ExpiresAt) {
		return nil, authstore.ErrNotFound
	}
	return &authstore.Session{ID: session.ID, UserID: session.UserID, ExpiresAt: session.ExpiresAt}, nil
}

func (s *fakeHTTPAuthStore) DeleteSession(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
	return nil
}

func (s *fakeHTTPAuthStore) RecordFailedLoginAttempt(_ context.Context, ip string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failedLoginAttempts[ip] = append(s.failedLoginAttempts[ip], time.Now())
	return nil
}

func (s *fakeHTTPAuthStore) CountRecentFailedLoginAttempts(_ context.Context, ip string, since time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, at := range s.failedLoginAttempts[ip] {
		if !at.Before(since) {
			count++
		}
	}
	return count, nil
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

type fakeHTTPResponse struct {
	statusCode int
	body       string
	err        error
}

type fakeHTTPCall struct {
	method  string
	url     string
	body    string
	headers map[string]string
}

type fakeHTTPDoer struct {
	mu        sync.Mutex
	responses []fakeHTTPResponse
	calls     []fakeHTTPCall
}

func (d *fakeHTTPDoer) Do(ctx context.Context, method, url string, body []byte, headers map[string]string) (int, []byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	copiedHeaders := make(map[string]string, len(headers))
	for key, value := range headers {
		copiedHeaders[key] = value
	}
	d.calls = append(d.calls, fakeHTTPCall{
		method:  method,
		url:     url,
		body:    string(body),
		headers: copiedHeaders,
	})

	if err := ctx.Err(); err != nil {
		return 0, nil, err
	}
	index := len(d.calls) - 1
	if index >= len(d.responses) {
		return 0, nil, errors.New("unexpected HTTP call")
	}
	resp := d.responses[index]
	return resp.statusCode, []byte(resp.body), resp.err
}

func (d *fakeHTTPDoer) Calls() []fakeHTTPCall {
	d.mu.Lock()
	defer d.mu.Unlock()

	calls := make([]fakeHTTPCall, len(d.calls))
	for i, call := range d.calls {
		copiedHeaders := make(map[string]string, len(call.headers))
		for key, value := range call.headers {
			copiedHeaders[key] = value
		}
		calls[i] = fakeHTTPCall{
			method:  call.method,
			url:     call.url,
			body:    call.body,
			headers: copiedHeaders,
		}
	}
	return calls
}

type fakeHTTPInterfaceStore struct {
	items      []interfacestore.Interface
	createFunc func(context.Context, *interfacestore.Interface) error
	listFunc   func(context.Context) ([]interfacestore.Interface, error)
	updateFunc func(context.Context, *interfacestore.Interface) error
}

func (s *fakeHTTPInterfaceStore) Create(ctx context.Context, iface *interfacestore.Interface) error {
	if s.createFunc != nil {
		return s.createFunc(ctx, iface)
	}
	iface.ID = "interface-1"
	iface.CreatedAt = testHTTPTime()
	return nil
}

func (s *fakeHTTPInterfaceStore) List(ctx context.Context) ([]interfacestore.Interface, error) {
	if s.listFunc != nil {
		return s.listFunc(ctx)
	}
	if s.items == nil {
		return []interfacestore.Interface{}, nil
	}
	return s.items, nil
}

func (s *fakeHTTPInterfaceStore) Update(ctx context.Context, iface *interfacestore.Interface) error {
	if s.updateFunc != nil {
		return s.updateFunc(ctx, iface)
	}
	return nil
}

type fakeHTTPEnvironmentStore struct {
	items      []environmentstore.Environment
	createFunc func(context.Context, *environmentstore.Environment) error
	getFunc    func(context.Context, string) (*environmentstore.Environment, error)
	listFunc   func(context.Context) ([]environmentstore.Environment, error)
	updateFunc func(context.Context, *environmentstore.Environment) error
}

func (s *fakeHTTPEnvironmentStore) Create(ctx context.Context, env *environmentstore.Environment) error {
	if s.createFunc != nil {
		return s.createFunc(ctx, env)
	}
	env.ID = "env-1"
	env.CreatedAt = testHTTPTime()
	if env.Variables == nil {
		env.Variables = map[string]string{}
	}
	if env.DefaultHeaders == nil {
		env.DefaultHeaders = map[string]string{}
	}
	return nil
}

func (s *fakeHTTPEnvironmentStore) Get(ctx context.Context, id string) (*environmentstore.Environment, error) {
	if s.getFunc != nil {
		return s.getFunc(ctx, id)
	}
	for _, env := range s.items {
		if env.ID == id {
			return &env, nil
		}
	}
	return nil, environmentstore.ErrNotFound
}

func (s *fakeHTTPEnvironmentStore) List(ctx context.Context) ([]environmentstore.Environment, error) {
	if s.listFunc != nil {
		return s.listFunc(ctx)
	}
	if s.items == nil {
		return []environmentstore.Environment{}, nil
	}
	return s.items, nil
}

func (s *fakeHTTPEnvironmentStore) Update(ctx context.Context, env *environmentstore.Environment) error {
	if s.updateFunc != nil {
		return s.updateFunc(ctx, env)
	}
	if env.Variables == nil {
		env.Variables = map[string]string{}
	}
	if env.DefaultHeaders == nil {
		env.DefaultHeaders = map[string]string{}
	}
	return nil
}

type fakeHTTPPerfStore struct {
	items      []perfstore.PerfTaskSummary
	samples    map[int64]json.RawMessage
	createFunc func(context.Context, *perfstore.PerfTask) error
	getFunc    func(context.Context, int64) (*perfstore.PerfTask, error)
	listFunc   func(context.Context) ([]perfstore.PerfTaskSummary, error)
	deleteFunc func(context.Context, int64) error
}

func (s *fakeHTTPPerfStore) Create(ctx context.Context, task *perfstore.PerfTask) error {
	if s.createFunc != nil {
		return s.createFunc(ctx, task)
	}
	task.ID = 1
	task.CreatedAt = testHTTPTime()
	return nil
}

func (s *fakeHTTPPerfStore) Get(ctx context.Context, id int64) (*perfstore.PerfTask, error) {
	if s.getFunc != nil {
		return s.getFunc(ctx, id)
	}
	for _, summary := range s.items {
		if summary.ID == id {
			return &perfstore.PerfTask{PerfTaskSummary: summary, Samples: s.samples[id]}, nil
		}
	}
	return nil, perfstore.ErrNotFound
}

func (s *fakeHTTPPerfStore) List(ctx context.Context) ([]perfstore.PerfTaskSummary, error) {
	if s.listFunc != nil {
		return s.listFunc(ctx)
	}
	if s.items == nil {
		return []perfstore.PerfTaskSummary{}, nil
	}
	return s.items, nil
}

func (s *fakeHTTPPerfStore) Delete(ctx context.Context, id int64) error {
	if s.deleteFunc != nil {
		return s.deleteFunc(ctx, id)
	}
	return nil
}

type fakeHTTPScenarioStore struct {
	items      []scenariostore.Scenario
	createFunc func(context.Context, *scenariostore.Scenario) error
	listFunc   func(context.Context) ([]scenariostore.Scenario, error)
}

func (s *fakeHTTPScenarioStore) Create(ctx context.Context, scen *scenariostore.Scenario) error {
	if s.createFunc != nil {
		return s.createFunc(ctx, scen)
	}
	scen.ID = "scenario-1"
	scen.CreatedAt = testHTTPTime()
	return nil
}

func (s *fakeHTTPScenarioStore) List(ctx context.Context) ([]scenariostore.Scenario, error) {
	if s.listFunc != nil {
		return s.listFunc(ctx)
	}
	if s.items == nil {
		return []scenariostore.Scenario{}, nil
	}
	return s.items, nil
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

	req := authenticatedRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	return resp
}

func authenticatedRequest(method string, target string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, target, body)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-1"})
	return req
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

func hashHTTPPasswordForTest(t *testing.T, password string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword() error = %v", err)
	}
	return string(hash)
}

func testHTTPTOTPSecret(t *testing.T, username string) string {
	t.Helper()
	key, err := totp.Generate(totp.GenerateOpts{Issuer: "test", AccountName: username})
	if err != nil {
		t.Fatalf("totp.Generate() error = %v", err)
	}
	return key.Secret()
}

func currentHTTPTOTPCode(t *testing.T, secret string) string {
	t.Helper()
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("GenerateCode() error = %v", err)
	}
	return code
}

func cloneHTTPUser(user *authstore.User) *authstore.User {
	if user == nil {
		return nil
	}
	cloned := *user
	return &cloned
}
