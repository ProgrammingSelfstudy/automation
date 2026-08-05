package httpapi

import (
	"context"
	"net/http"

	"interface-load-test/internal/auth"
	"interface-load-test/internal/authstore"
)

const sessionCookieName = "session"

type currentUserContextKey struct{}

func requireAuth(svc *auth.Service, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := authenticateRequest(w, r, svc)
		if !ok {
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), currentUserContextKey{}, user)))
	}
}

func requireAuthHandler(svc *auth.Service, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := authenticateRequest(w, r, svc)
		if !ok {
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), currentUserContextKey{}, user)))
	})
}

func authenticateRequest(w http.ResponseWriter, r *http.Request, svc *auth.Service) (*authstore.User, bool) {
	if svc == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return nil, false
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return nil, false
	}
	user, err := svc.CurrentUser(r.Context(), cookie.Value)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return nil, false
	}
	return user, true
}

func currentUser(ctx context.Context) *authstore.User {
	user, _ := ctx.Value(currentUserContextKey{}).(*authstore.User)
	return user
}

func setSessionCookie(w http.ResponseWriter, session *authstore.Session, secure bool) {
	sameSite := http.SameSiteLaxMode
	if secure {
		sameSite = http.SameSiteNoneMode
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    session.ID,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
		Expires:  session.ExpiresAt,
	})
}

func clearSessionCookie(w http.ResponseWriter, secure bool) {
	sameSite := http.SameSiteLaxMode
	if secure {
		sameSite = http.SameSiteNoneMode
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
		MaxAge:   -1,
	})
}
