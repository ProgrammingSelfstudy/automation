package wsapi

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"interface-load-test/internal/logevent"
)

func TestProgressHandlerStreamsPublishedEvents(t *testing.T) {
	hub := logevent.NewHub()
	server := newProgressTestServer(t, hub, nil, WithTaskExists(existingTasks("task-1")))

	conn := dialProgress(t, server.URL, "task-1")
	defer conn.CloseNow()
	waitFor(t, time.Second, func() bool {
		return hub.SubscriberCount("task-1") == 1
	})

	event := testProgressEvent("task-1", 1)
	hub.Publish(event)

	raw, got := readProgressEvent(t, conn)
	assertProgressEventEqual(t, got, event)
	assertJSONFields(t, raw, "task_id", "account_id", "seq_no", "step_name", "success", "cost_ms", "timestamp")
}

func TestProgressHandlerUnsubscribesWhenClientDisconnects(t *testing.T) {
	hub := logevent.NewHub()
	server := newProgressTestServer(t, hub, nil, WithTaskExists(existingTasks("task-1")))

	conn := dialProgress(t, server.URL, "task-1")
	waitFor(t, time.Second, func() bool {
		return hub.SubscriberCount("task-1") == 1
	})

	if err := conn.Close(websocket.StatusNormalClosure, "done"); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	waitFor(t, time.Second, func() bool {
		return hub.SubscriberCount("task-1") == 0
	})
}

func TestProgressHandlerReturns404BeforeUpgradeForMissingTask(t *testing.T) {
	hub := logevent.NewHub()
	server := newProgressTestServer(t, hub, nil, WithTaskExists(existingTasks("task-1")))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	conn, resp, err := websocket.Dial(ctx, wsURL(server.URL, "missing-task"), nil)
	if err == nil {
		conn.CloseNow()
		t.Fatal("Dial() succeeded for missing task, want 404")
	}
	if resp == nil {
		t.Fatalf("Dial() response is nil, error = %v", err)
	}
	if got, want := resp.StatusCode, http.StatusNotFound; got != want {
		t.Fatalf("Dial() status = %d, want %d", got, want)
	}
	if got := hub.SubscriberCount("missing-task"); got != 0 {
		t.Fatalf("SubscriberCount() = %d, want 0", got)
	}
}

func TestProgressHandlerAllowedOrigins(t *testing.T) {
	hub := logevent.NewHub()
	server := newProgressTestServer(t, hub, nil,
		WithTaskExists(existingTasks("task-1")),
		WithAllowedOrigins([]string{"http://localhost:5173"}),
	)

	allowed, resp, err := dialProgressWithOrigin(t, server.URL, "task-1", "http://localhost:5173")
	if err != nil {
		if resp != nil {
			t.Fatalf("allowed Dial() error = %v, status = %d", err, resp.StatusCode)
		}
		t.Fatalf("allowed Dial() error = %v", err)
	}
	allowed.CloseNow()

	disallowed, resp, err := dialProgressWithOrigin(t, server.URL, "task-1", "http://evil.local")
	if err == nil {
		disallowed.CloseNow()
		t.Fatal("disallowed Dial() succeeded, want 403")
	}
	if resp == nil {
		t.Fatalf("disallowed Dial() response is nil, error = %v", err)
	}
	if got, want := resp.StatusCode, http.StatusForbidden; got != want {
		t.Fatalf("disallowed status = %d, want %d", got, want)
	}
}

func TestProgressHandlerWriteTimeoutUnsubscribesStuckClient(t *testing.T) {
	hub := logevent.NewHub()
	server := newProgressTestServer(t, hub, &smallWriteBufferListener{writeBufferSize: 1024},
		WithTaskExists(existingTasks("task-1")),
		WithWriteTimeout(50*time.Millisecond),
	)

	conn := dialProgress(t, server.URL, "task-1")
	defer conn.CloseNow()
	waitFor(t, time.Second, func() bool {
		return hub.SubscriberCount("task-1") == 1
	})

	largeErrMsg := strings.Repeat("write-timeout-padding", 1<<20)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && hub.SubscriberCount("task-1") != 0 {
		event := testProgressEvent("task-1", 1)
		event.Success = false
		event.ErrMsg = largeErrMsg
		hub.Publish(event)
		time.Sleep(5 * time.Millisecond)
	}

	waitFor(t, 2*time.Second, func() bool {
		return hub.SubscriberCount("task-1") == 0
	})
}

func TestProgressHandlerBroadcastsToMultipleClients(t *testing.T) {
	hub := logevent.NewHub()
	server := newProgressTestServer(t, hub, nil, WithTaskExists(existingTasks("task-1")))

	var conns []*websocket.Conn
	for i := 0; i < 3; i++ {
		conn := dialProgress(t, server.URL, "task-1")
		defer conn.CloseNow()
		conns = append(conns, conn)
	}
	waitFor(t, time.Second, func() bool {
		return hub.SubscriberCount("task-1") == 3
	})

	event := testProgressEvent("task-1", 1)
	hub.Publish(event)

	for i, conn := range conns {
		_, got := readProgressEvent(t, conn)
		if !reflect.DeepEqual(got, event) {
			t.Fatalf("client %d received event = %#v, want %#v", i+1, got, event)
		}
	}
}

type smallWriteBufferListener struct {
	net.Listener
	writeBufferSize int
}

func (l *smallWriteBufferListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		_ = tcpConn.SetWriteBuffer(l.writeBufferSize)
	}
	return conn, nil
}

func newProgressTestServer(t *testing.T, hub *logevent.Hub, listenerWrapper *smallWriteBufferListener, opts ...ProgressHandlerOption) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.Handle("GET /ws/tasks/{id}/progress", NewProgressHandler(hub, opts...))

	server := httptest.NewUnstartedServer(mux)
	if listenerWrapper != nil {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("Listen() error = %v", err)
		}
		listenerWrapper.Listener = listener
		server.Listener = listenerWrapper
	}
	server.Start()
	t.Cleanup(server.Close)

	return server
}

func existingTasks(taskIDs ...string) TaskExistsFunc {
	tasks := make(map[string]bool, len(taskIDs))
	for _, taskID := range taskIDs {
		tasks[taskID] = true
	}
	return func(_ context.Context, taskID string) (bool, error) {
		return tasks[taskID], nil
	}
}

func dialProgress(t *testing.T, baseURL string, taskID string) *websocket.Conn {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	conn, resp, err := websocket.Dial(ctx, wsURL(baseURL, taskID), nil)
	if err != nil {
		if resp != nil {
			t.Fatalf("Dial() error = %v, status = %d", err, resp.StatusCode)
		}
		t.Fatalf("Dial() error = %v", err)
	}
	return conn
}

func dialProgressWithOrigin(t *testing.T, baseURL string, taskID string, origin string) (*websocket.Conn, *http.Response, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	return websocket.Dial(ctx, wsURL(baseURL, taskID), &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{origin}},
	})
}

func wsURL(baseURL string, taskID string) string {
	return "ws" + strings.TrimPrefix(baseURL, "http") + "/ws/tasks/" + taskID + "/progress"
}

func testProgressEvent(taskID string, seqNo int) logevent.Event {
	return logevent.Event{
		TaskID:    taskID,
		AccountID: "account-1",
		SeqNo:     seqNo,
		StepName:  "login",
		Success:   true,
		CostMs:    123,
		Timestamp: time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC),
	}
}

func readProgressEvent(t *testing.T, conn *websocket.Conn) ([]byte, logevent.Event) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	messageType, reader, err := conn.Reader(ctx)
	if err != nil {
		t.Fatalf("Reader() error = %v", err)
	}
	if messageType != websocket.MessageText {
		t.Fatalf("message type = %v, want %v", messageType, websocket.MessageText)
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}

	var event logevent.Event
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatalf("Unmarshal() error = %v, data = %s", err, data)
	}

	return data, event
}

func assertProgressEventEqual(t *testing.T, got logevent.Event, want logevent.Event) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("event = %#v, want %#v", got, want)
	}
}

func assertJSONFields(t *testing.T, data []byte, fields ...string) {
	t.Helper()

	var message map[string]any
	if err := json.Unmarshal(data, &message); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	for _, field := range fields {
		if _, ok := message[field]; !ok {
			t.Fatalf("JSON field %q missing from %s", field, data)
		}
	}
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	if condition() {
		return
	}
	t.Fatalf("condition was not met within %v", timeout)
}
