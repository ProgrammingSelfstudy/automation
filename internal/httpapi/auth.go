package httpapi

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"time"

	"interface-load-test/internal/auth"
	"interface-load-test/internal/authstore"
)

const maxAuthBodyBytes int64 = 1 << 20

type loginRequest struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	Code       string `json:"code,omitempty"`
	BackupCode string `json:"backup_code,omitempty"`
}

type totpSetupRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type totpConfirmRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Code     string `json:"code"`
}

type createUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type regenerateBackupCodesRequest struct {
	Password string `json:"password"`
}

type authUserResponse struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	TOTPEnabled bool      `json:"totp_enabled"`
	CreatedAt   time.Time `json:"created_at"`
}

func (h *handler) login(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBodyBytes)
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "invalid json")
		return
	}

	user, session, err := h.deps.AuthService.Login(r.Context(), clientIP(r), req.Username, req.Password, req.Code, req.BackupCode)
	if errors.Is(err, auth.ErrTOTPSetupRequired) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "totp_setup_required"})
		return
	}
	if errors.Is(err, auth.ErrTOTPCodeRequired) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "totp_code_required"})
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}

	setSessionCookie(w, session, h.deps.CookieSecure)
	writeJSON(w, http.StatusOK, map[string]authUserResponse{"user": newAuthUserResponse(user)})
}

func (h *handler) setupTOTP(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBodyBytes)
	var req totpSetupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "invalid json")
		return
	}

	secret, otpauthURL, err := h.deps.AuthService.SetupTOTP(r.Context(), req.Username, req.Password)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"secret":      secret,
		"otpauth_url": otpauthURL,
	})
}

func (h *handler) confirmTOTP(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBodyBytes)
	var req totpConfirmRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "invalid json")
		return
	}

	user, session, backupCodes, err := h.deps.AuthService.ConfirmTOTP(r.Context(), req.Username, req.Password, req.Code)
	if err != nil {
		writeError(w, err)
		return
	}

	setSessionCookie(w, session, h.deps.CookieSecure)
	writeJSON(w, http.StatusOK, map[string]any{
		"user":         newAuthUserResponse(user),
		"backup_codes": backupCodes,
	})
}

func (h *handler) logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && cookie.Value != "" {
		if err := h.deps.AuthService.Logout(r.Context(), cookie.Value); err != nil {
			writeError(w, err)
			return
		}
	}
	clearSessionCookie(w, h.deps.CookieSecure)
	writeJSON(w, http.StatusOK, map[string]bool{"logged_out": true})
}

func (h *handler) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]authUserResponse{"user": newAuthUserResponse(currentUser(r.Context()))})
}

func (h *handler) createAuthUser(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBodyBytes)
	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "invalid json")
		return
	}
	user, err := h.deps.AuthService.CreateUser(r.Context(), req.Username, req.Password)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]authUserResponse{"user": newAuthUserResponse(user)})
}

func (h *handler) regenerateBackupCodes(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBodyBytes)
	var req regenerateBackupCodesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "invalid json")
		return
	}
	user := currentUser(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	codes, err := h.deps.AuthService.RegenerateBackupCodes(r.Context(), user.ID, req.Password)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string][]string{"backup_codes": codes})
}

func newAuthUserResponse(user *authstore.User) authUserResponse {
	if user == nil {
		return authUserResponse{}
	}
	return authUserResponse{
		ID:          user.ID,
		Username:    user.Username,
		TOTPEnabled: user.TOTPEnabled,
		CreatedAt:   user.CreatedAt,
	}
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
