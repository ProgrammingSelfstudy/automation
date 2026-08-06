package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"interface-load-test/internal/environmentstore"
)

const maxEnvironmentBodyBytes int64 = maxCreateTaskBodyBytes

type environmentRequest struct {
	Name           string            `json:"name"`
	Variables      map[string]string `json:"variables,omitempty"`
	DefaultHeaders map[string]string `json:"default_headers,omitempty"`
}

type environmentResponse struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Variables      map[string]string `json:"variables"`
	DefaultHeaders map[string]string `json:"default_headers"`
	CreatedAt      time.Time         `json:"created_at"`
}

func (h *handler) createEnvironment(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxEnvironmentBodyBytes)

	var req environmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "invalid json")
		return
	}

	env := environmentstore.Environment{
		Name:           req.Name,
		Variables:      req.Variables,
		DefaultHeaders: req.DefaultHeaders,
	}
	if err := h.deps.EnvironmentStore.Create(r.Context(), &env); err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, newEnvironmentResponse(env))
}

func (h *handler) listEnvironments(w http.ResponseWriter, r *http.Request) {
	environments, err := h.deps.EnvironmentStore.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}

	responses := make([]environmentResponse, 0, len(environments))
	for _, env := range environments {
		responses = append(responses, newEnvironmentResponse(env))
	}
	writeJSON(w, http.StatusOK, responses)
}

func (h *handler) updateEnvironment(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxEnvironmentBodyBytes)

	var req environmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "invalid json")
		return
	}

	env := environmentstore.Environment{
		ID:             r.PathValue("id"),
		Name:           req.Name,
		Variables:      req.Variables,
		DefaultHeaders: req.DefaultHeaders,
	}
	if err := h.deps.EnvironmentStore.Update(r.Context(), &env); err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, newEnvironmentResponse(env))
}

func newEnvironmentResponse(env environmentstore.Environment) environmentResponse {
	variables := env.Variables
	if variables == nil {
		variables = map[string]string{}
	}
	defaultHeaders := env.DefaultHeaders
	if defaultHeaders == nil {
		defaultHeaders = map[string]string{}
	}
	return environmentResponse{
		ID:             env.ID,
		Name:           env.Name,
		Variables:      variables,
		DefaultHeaders: defaultHeaders,
		CreatedAt:      env.CreatedAt,
	}
}
