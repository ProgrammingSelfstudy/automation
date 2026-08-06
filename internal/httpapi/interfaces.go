package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"interface-load-test/internal/interfacestore"
	"interface-load-test/internal/scenario"
)

const maxCreateInterfaceBodyBytes int64 = maxCreateTaskBodyBytes

type createInterfaceRequest struct {
	Name string        `json:"name"`
	Step scenario.Step `json:"step"`
}

type interfaceResponse struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Step      scenario.Step `json:"step"`
	CreatedAt time.Time     `json:"created_at"`
}

func (h *handler) createInterface(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCreateInterfaceBodyBytes)

	var req createInterfaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "invalid json")
		return
	}

	iface := interfacestore.Interface{
		Name: req.Name,
		Step: req.Step,
	}
	if err := h.deps.InterfaceStore.Create(r.Context(), &iface); err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, newInterfaceResponse(iface))
}

func (h *handler) listInterfaces(w http.ResponseWriter, r *http.Request) {
	interfaces, err := h.deps.InterfaceStore.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}

	responses := make([]interfaceResponse, 0, len(interfaces))
	for _, iface := range interfaces {
		responses = append(responses, newInterfaceResponse(iface))
	}
	writeJSON(w, http.StatusOK, responses)
}

func (h *handler) updateInterface(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCreateInterfaceBodyBytes)

	var req createInterfaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "invalid json")
		return
	}

	iface := interfacestore.Interface{
		ID:   r.PathValue("id"),
		Name: req.Name,
		Step: req.Step,
	}
	if err := h.deps.InterfaceStore.Update(r.Context(), &iface); err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, newInterfaceResponse(iface))
}

func newInterfaceResponse(iface interfacestore.Interface) interfaceResponse {
	return interfaceResponse{
		ID:        iface.ID,
		Name:      iface.Name,
		Step:      iface.Step,
		CreatedAt: iface.CreatedAt,
	}
}
