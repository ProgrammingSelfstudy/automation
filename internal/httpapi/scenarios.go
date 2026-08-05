package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"interface-load-test/internal/scenario"
	"interface-load-test/internal/scenariostore"
)

const maxCreateScenarioBodyBytes int64 = maxCreateTaskBodyBytes

type createScenarioRequest struct {
	Name       string            `json:"name"`
	Definition scenario.Scenario `json:"definition"`
}

type scenarioResponse struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Definition scenario.Scenario `json:"definition"`
	CreatedAt  time.Time         `json:"created_at"`
}

func (h *handler) createScenario(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCreateScenarioBodyBytes)

	var req createScenarioRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "invalid json")
		return
	}

	scen := scenariostore.Scenario{
		Name:       req.Name,
		Definition: req.Definition,
	}
	if err := h.deps.ScenarioStore.Create(r.Context(), &scen); err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, newScenarioResponse(scen))
}

func (h *handler) listScenarios(w http.ResponseWriter, r *http.Request) {
	scenarios, err := h.deps.ScenarioStore.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}

	responses := make([]scenarioResponse, 0, len(scenarios))
	for _, scen := range scenarios {
		responses = append(responses, newScenarioResponse(scen))
	}
	writeJSON(w, http.StatusOK, responses)
}

func newScenarioResponse(scen scenariostore.Scenario) scenarioResponse {
	return scenarioResponse{
		ID:         scen.ID,
		Name:       scen.Name,
		Definition: scen.Definition,
		CreatedAt:  scen.CreatedAt,
	}
}
