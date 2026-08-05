package httpapi

import (
	"bufio"
	"errors"
	"log"
	"net"
	"net/http"
	"time"
)

// withAccessLog logs one line per request: method, path, status code, and
// duration. It wraps the ResponseWriter to capture the status code, which
// http.ResponseWriter does not expose on its own.
func withAccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		log.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.status, time.Since(start))
	})
}

// statusRecorder wraps http.ResponseWriter to capture the status code.
//
// It forwards Hijack (used directly by some libraries) and Unwrap (used by
// net/http's ResponseController, which coder/websocket relies on) so that
// wrapping the ResponseWriter here does not break WebSocket upgrades on
// /ws/tasks/{id}/progress — a wrapper that does neither silently turns every
// WS connection attempt into a "does not support hijacking" error.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("httpapi: underlying ResponseWriter does not support hijacking")
	}
	return hijacker.Hijack()
}

func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}
