package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"interface-load-test/internal/accountstore"
)

type accountResponse struct {
	ID        string          `json:"id"`
	GroupID   string          `json:"group_id"`
	Username  string          `json:"username"`
	Extra     json.RawMessage `json:"extra,omitempty"`
	Enabled   bool            `json:"enabled"`
	CreatedAt time.Time       `json:"created_at"`
}

type createAccountRequest struct {
	ID       string          `json:"id,omitempty"`
	GroupID  string          `json:"group_id"`
	Username string          `json:"username"`
	Password string          `json:"password"`
	Extra    json.RawMessage `json:"extra,omitempty"`
	Enabled  *bool           `json:"enabled,omitempty"`
}

func (h *handler) createAccount(w http.ResponseWriter, r *http.Request) {
	var req createAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "invalid json")
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	acc := accountstore.Account{
		ID:       req.ID,
		GroupID:  req.GroupID,
		Username: req.Username,
		Password: req.Password,
		Extra:    req.Extra,
		Enabled:  enabled,
	}

	if err := h.deps.AccountStore.Create(r.Context(), &acc); err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, newAccountResponse(acc))
}

func (h *handler) listAccounts(w http.ResponseWriter, r *http.Request) {
	groupID := r.URL.Query().Get("group_id")
	if groupID == "" {
		writeBadRequest(w, "group_id is required")
		return
	}

	accounts, err := h.deps.AccountStore.ListByGroup(r.Context(), groupID)
	if err != nil {
		writeError(w, err)
		return
	}

	responses := make([]accountResponse, 0, len(accounts))
	for _, acc := range accounts {
		responses = append(responses, newAccountResponse(acc))
	}
	writeJSON(w, http.StatusOK, responses)
}

func newAccountResponse(acc accountstore.Account) accountResponse {
	return accountResponse{
		ID:        acc.ID,
		GroupID:   acc.GroupID,
		Username:  acc.Username,
		Extra:     acc.Extra,
		Enabled:   acc.Enabled,
		CreatedAt: acc.CreatedAt,
	}
}
