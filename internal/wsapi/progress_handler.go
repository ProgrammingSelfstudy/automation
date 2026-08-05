package wsapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"

	"interface-load-test/internal/logevent"
)

const defaultWriteTimeout = 5 * time.Second

// TaskExistsFunc checks whether a task may be watched before WebSocket upgrade.
type TaskExistsFunc func(ctx context.Context, taskID string) (bool, error)

// ProgressHandlerOption configures ProgressHandler.
type ProgressHandlerOption func(*ProgressHandler)

// ProgressHandler streams task progress events over WebSocket.
type ProgressHandler struct {
	hub            *logevent.Hub
	taskExists     TaskExistsFunc
	writeTimeout   time.Duration
	allowedOrigins []string
}

// NewProgressHandler creates a WebSocket progress handler.
//
// By default, task checks deny all task IDs. Wire WithTaskExists to the platform
// task manager before registering the handler.
func NewProgressHandler(hub *logevent.Hub, opts ...ProgressHandlerOption) *ProgressHandler {
	h := &ProgressHandler{
		hub: hub,
		taskExists: func(context.Context, string) (bool, error) {
			return false, nil
		},
		writeTimeout: defaultWriteTimeout,
	}

	for _, opt := range opts {
		opt(h)
	}

	return h
}

// WithTaskExists configures the pre-upgrade task existence check.
func WithTaskExists(fn TaskExistsFunc) ProgressHandlerOption {
	return func(h *ProgressHandler) {
		if fn != nil {
			h.taskExists = fn
		}
	}
}

// WithWriteTimeout configures the per-message WebSocket write timeout.
func WithWriteTimeout(timeout time.Duration) ProgressHandlerOption {
	return func(h *ProgressHandler) {
		if timeout > 0 {
			h.writeTimeout = timeout
		}
	}
}

// WithAllowedOrigins configures cross-origin WebSocket requests.
func WithAllowedOrigins(origins []string) ProgressHandlerOption {
	return func(h *ProgressHandler) {
		h.allowedOrigins = normalizeOrigins(origins)
	}
}

func (h *ProgressHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.hub == nil {
		http.Error(w, "log event hub is not configured", http.StatusInternalServerError)
		return
	}

	taskID := r.PathValue("id")
	if taskID == "" {
		http.NotFound(w, r)
		return
	}

	exists, err := h.taskExists(r.Context(), taskID)
	if err != nil {
		http.Error(w, "failed to check task", http.StatusInternalServerError)
		return
	}
	if !exists {
		http.NotFound(w, r)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: h.allowedOrigins,
	})
	if err != nil {
		return
	}
	defer conn.CloseNow()

	events, unsubscribe := h.hub.Subscribe(taskID)
	defer unsubscribe()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	go discardClientMessages(ctx, cancel, conn)

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}

			data, err := json.Marshal(event)
			if err != nil {
				return
			}

			writeCtx, writeCancel := context.WithTimeout(ctx, h.writeTimeout)
			err = conn.Write(writeCtx, websocket.MessageText, data)
			writeCancel()
			if err != nil {
				cancel()
				return
			}
		}
	}
}

func normalizeOrigins(origins []string) []string {
	normalized := make([]string, 0, len(origins))
	for _, origin := range origins {
		origin = strings.TrimSpace(origin)
		if origin != "" {
			normalized = append(normalized, origin)
		}
	}
	return normalized
}

func discardClientMessages(ctx context.Context, cancel context.CancelFunc, conn *websocket.Conn) {
	defer cancel()

	for {
		_, reader, err := conn.Reader(ctx)
		if err != nil {
			return
		}
		if _, err := io.Copy(io.Discard, reader); err != nil {
			return
		}
	}
}
