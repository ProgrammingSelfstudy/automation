package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"interface-load-test/internal/environmentstore"
	"interface-load-test/internal/scenario"
)

const (
	trySendTimeout           = 15 * time.Second
	trySendMaxResponseBytes  = 1 << 20
	maxTrySendInterfaceBytes = maxCreateTaskBodyBytes
)

type trySendInterfaceRequest struct {
	Step          scenario.Step `json:"step"`
	EnvironmentID string        `json:"environment_id,omitempty"`
}

type trySendRequestResponse struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

type trySendHTTPResponse struct {
	StatusCode int    `json:"status_code"`
	Body       string `json:"body"`
	CostMs     int64  `json:"cost_ms"`
	Truncated  bool   `json:"truncated"`
}

type trySendInterfaceResponse struct {
	Request  trySendRequestResponse `json:"request"`
	Response trySendHTTPResponse    `json:"response"`
	Error    string                 `json:"error"`
}

func (h *handler) trySendInterface(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxTrySendInterfaceBytes)

	var req trySendInterfaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "invalid json")
		return
	}

	env := environmentstore.Environment{
		Variables:      map[string]string{},
		DefaultHeaders: map[string]string{},
	}
	if req.EnvironmentID != "" {
		found, err := h.deps.EnvironmentStore.Get(r.Context(), req.EnvironmentID)
		if err != nil {
			writeError(w, err)
			return
		}
		env = *found
	}

	step := req.Step
	step.Headers = scenario.MergeHeaders(env.DefaultHeaders, step.Headers)
	rendered, err := scenario.RenderStep(step, map[string]any{"env": env.Variables})
	if err != nil {
		writeJSON(w, http.StatusOK, trySendInterfaceResponse{
			Request: trySendRequestResponse{
				Headers: map[string]string{},
			},
			Error: err.Error(),
		})
		return
	}

	response := trySendInterfaceResponse{
		Request: trySendRequestResponse{
			Method:  rendered.Method,
			URL:     rendered.URL,
			Headers: rendered.Headers,
			Body:    rendered.Body,
		},
	}

	httpDoer := h.deps.HTTPDoer
	if httpDoer == nil {
		httpDoer = scenario.NewHTTPClient(0)
	}

	ctx, cancel := context.WithTimeout(r.Context(), trySendTimeout)
	defer cancel()
	ctx = scenario.WithResponseBodyLimit(ctx, trySendMaxResponseBytes)

	startedAt := time.Now()
	statusCode, body, err := httpDoer.Do(ctx, rendered.Method, rendered.URL, []byte(rendered.Body), rendered.Headers)
	response.Response.CostMs = time.Since(startedAt).Milliseconds()
	if len(body) > trySendMaxResponseBytes {
		body = body[:trySendMaxResponseBytes]
		response.Response.Truncated = true
	}
	response.Response.StatusCode = statusCode
	response.Response.Body = string(body)
	if err != nil {
		response.Error = err.Error()
	}

	writeJSON(w, http.StatusOK, response)
}
