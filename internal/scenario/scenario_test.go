package scenario

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"interface-load-test/internal/accountpool"
	"interface-load-test/internal/logevent"
)

func TestExecuteRendersVariablesAcrossMultipleStepsAndEvaluatesFormula(t *testing.T) {
	doer := &fakeHTTPDoer{
		responses: []fakeHTTPResponse{
			{statusCode: 200, body: `{"order":{"id":"order-123"},"price":10,"discount":2}`},
			{statusCode: 200, body: `{"qty":3}`},
		},
	}
	executor := NewExecutor(doer, nil)

	result := executor.Execute(context.Background(), "task-1", 7, testAccount("account-1"), Scenario{
		Steps: []Step{
			{
				Name:    "create-order",
				Method:  "POST",
				URL:     "https://api.example.com/orders",
				BodyTpl: `{"username":"{{.account.Username}}"}`,
				Headers: map[string]string{
					"X-Account": "{{.account.ID}}",
				},
				Extract: map[string]string{
					"orderId":  "order.id",
					"price":    "price",
					"discount": "discount",
				},
			},
			{
				Name:    "pay-order",
				Method:  "POST",
				URL:     "https://api.example.com/orders/{{.orderId}}/pay",
				BodyTpl: `{"order_id":"{{.orderId}}","account":"{{.account.Username}}"}`,
				Headers: map[string]string{
					"Authorization": "Bearer {{.orderId}}",
				},
				Extract: map[string]string{
					"qty": "qty",
				},
			},
		},
		Formula: "(price - discount) * qty",
	})

	if !result.Success {
		t.Fatalf("Execute() Success = false, ErrMsg = %q", result.ErrMsg)
	}
	if got, want := result.FormulaResult, 24.0; got != want {
		t.Fatalf("FormulaResult = %v, want %v", got, want)
	}
	if got, want := len(result.Steps), 2; got != want {
		t.Fatalf("len(Result.Steps) = %d, want %d", got, want)
	}

	calls := doer.Calls()
	if got, want := len(calls), 2; got != want {
		t.Fatalf("HTTP call count = %d, want %d", got, want)
	}
	if got, want := calls[0].body, `{"username":"alice"}`; got != want {
		t.Fatalf("first request body = %q, want %q", got, want)
	}
	if got, want := calls[0].headers["X-Account"], "account-1"; got != want {
		t.Fatalf("first X-Account header = %q, want %q", got, want)
	}
	if got, want := calls[1].url, "https://api.example.com/orders/order-123/pay"; got != want {
		t.Fatalf("second URL = %q, want %q", got, want)
	}
	if got, want := calls[1].body, `{"order_id":"order-123","account":"alice"}`; got != want {
		t.Fatalf("second request body = %q, want %q", got, want)
	}
	if got, want := calls[1].headers["Authorization"], "Bearer order-123"; got != want {
		t.Fatalf("second Authorization header = %q, want %q", got, want)
	}
}

func TestExecuteFormulaFailure(t *testing.T) {
	doer := &fakeHTTPDoer{
		responses: []fakeHTTPResponse{
			{statusCode: 200, body: `{"price":10}`},
		},
	}
	executor := NewExecutor(doer, nil)

	result := executor.Execute(context.Background(), "task-1", 1, testAccount("account-1"), Scenario{
		Steps: []Step{
			{
				Name:    "price",
				Method:  "GET",
				URL:     "https://api.example.com/price",
				Extract: map[string]string{"price": "price"},
			},
		},
		Formula: "missing + 1",
	})

	if result.Success {
		t.Fatal("Execute() Success = true, want false")
	}
	if result.ErrMsg == "" {
		t.Fatal("Execute() ErrMsg is empty, want formula error")
	}
	if got, want := len(result.Steps), 1; got != want {
		t.Fatalf("len(Result.Steps) = %d, want %d", got, want)
	}
	if !result.Steps[0].Success {
		t.Fatalf("step Success = false, ErrMsg = %q", result.Steps[0].ErrMsg)
	}
}

func TestExecuteStopsAfterHTTPError(t *testing.T) {
	hub := logevent.NewHub()
	events, unsubscribe := hub.Subscribe("task-1")
	defer unsubscribe()
	doer := &fakeHTTPDoer{
		responses: []fakeHTTPResponse{
			{statusCode: 500, body: `{"error":"boom"}`},
			{statusCode: 200, body: `{"should":"not run"}`},
		},
	}
	executor := NewExecutor(doer, hub)

	result := executor.Execute(context.Background(), "task-1", 2, testAccount("account-1"), Scenario{
		Steps: []Step{
			{Name: "first", Method: "GET", URL: "https://api.example.com/first"},
			{Name: "second", Method: "GET", URL: "https://api.example.com/second"},
		},
	})

	if result.Success {
		t.Fatal("Execute() Success = true, want false")
	}
	if !strings.Contains(result.ErrMsg, "http status 500") {
		t.Fatalf("ErrMsg = %q, want HTTP status error", result.ErrMsg)
	}
	if got, want := len(result.Steps), 1; got != want {
		t.Fatalf("len(Result.Steps) = %d, want %d", got, want)
	}
	if got, want := doer.CallCount(), 1; got != want {
		t.Fatalf("HTTP call count = %d, want %d", got, want)
	}

	event := receiveEvent(t, events)
	if event.Success {
		t.Fatal("failure event Success = true, want false")
	}
	if got, want := event.StepName, "first"; got != want {
		t.Fatalf("event StepName = %q, want %q", got, want)
	}
	if !strings.Contains(event.ErrMsg, "http status 500") {
		t.Fatalf("event ErrMsg = %q, want HTTP status error", event.ErrMsg)
	}
	assertNoEvent(t, events)
}

func TestExecuteMissingExtractValueDoesNotFail(t *testing.T) {
	doer := &fakeHTTPDoer{
		responses: []fakeHTTPResponse{
			{statusCode: 200, body: `{"ok":true}`},
			{statusCode: 200, body: `{"done":true}`},
		},
	}
	executor := NewExecutor(doer, nil)

	result := executor.Execute(context.Background(), "task-1", 1, testAccount("account-1"), Scenario{
		Steps: []Step{
			{
				Name:    "extract-missing",
				Method:  "GET",
				URL:     "https://api.example.com/missing",
				Extract: map[string]string{"missingValue": "missing.path"},
			},
			{
				Name:    "use-missing",
				Method:  "POST",
				URL:     "https://api.example.com/use-missing",
				BodyTpl: `{{if .missingValue}}present{{else}}missing{{end}}`,
			},
		},
	})

	if !result.Success {
		t.Fatalf("Execute() Success = false, ErrMsg = %q", result.ErrMsg)
	}
	calls := doer.Calls()
	if got, want := len(calls), 2; got != want {
		t.Fatalf("HTTP call count = %d, want %d", got, want)
	}
	if got, want := calls[1].body, "missing"; got != want {
		t.Fatalf("second request body = %q, want %q", got, want)
	}
}

func TestExecutePublishesStepEventsWithoutBodies(t *testing.T) {
	hub := logevent.NewHub()
	events, unsubscribe := hub.Subscribe("task-1")
	defer unsubscribe()
	doer := &fakeHTTPDoer{
		responses: []fakeHTTPResponse{
			{statusCode: 200, body: `{"token":"secret-response-body"}`},
			{statusCode: 200, body: `{"done":true}`},
		},
	}
	executor := NewExecutor(doer, hub)

	result := executor.Execute(context.Background(), "task-1", 11, testAccount("account-1"), Scenario{
		Steps: []Step{
			{
				Name:    "login",
				Method:  "POST",
				URL:     "https://api.example.com/login",
				BodyTpl: `secret-request-body`,
				Extract: map[string]string{"token": "token"},
			},
			{
				Name:    "use-token",
				Method:  "POST",
				URL:     "https://api.example.com/use-token",
				BodyTpl: `{"token":"{{.token}}"}`,
			},
		},
	})

	if !result.Success {
		t.Fatalf("Execute() Success = false, ErrMsg = %q", result.ErrMsg)
	}
	if !strings.Contains(result.Steps[0].Request, "secret-request-body") {
		t.Fatalf("StepLog request = %q, want full request body", result.Steps[0].Request)
	}
	if !strings.Contains(result.Steps[0].Response, "secret-response-body") {
		t.Fatalf("StepLog response = %q, want full response body", result.Steps[0].Response)
	}

	first := receiveEvent(t, events)
	second := receiveEvent(t, events)
	assertNoEvent(t, events)

	wantEvents := []logevent.Event{
		{TaskID: "task-1", AccountID: "account-1", SeqNo: 11, StepName: "login", Success: true},
		{TaskID: "task-1", AccountID: "account-1", SeqNo: 11, StepName: "use-token", Success: true},
	}
	for i, event := range []logevent.Event{first, second} {
		if got, want := event.TaskID, wantEvents[i].TaskID; got != want {
			t.Fatalf("event %d TaskID = %q, want %q", i+1, got, want)
		}
		if got, want := event.AccountID, wantEvents[i].AccountID; got != want {
			t.Fatalf("event %d AccountID = %q, want %q", i+1, got, want)
		}
		if got, want := event.SeqNo, wantEvents[i].SeqNo; got != want {
			t.Fatalf("event %d SeqNo = %d, want %d", i+1, got, want)
		}
		if got, want := event.StepName, wantEvents[i].StepName; got != want {
			t.Fatalf("event %d StepName = %q, want %q", i+1, got, want)
		}
		if !event.Success {
			t.Fatalf("event %d Success = false, want true", i+1)
		}
		if event.Timestamp.IsZero() {
			t.Fatalf("event %d Timestamp is zero", i+1)
		}

		data, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("Marshal(event %d) error = %v", i+1, err)
		}
		if strings.Contains(string(data), "secret-request-body") || strings.Contains(string(data), "secret-response-body") {
			t.Fatalf("event %d leaked request/response body: %s", i+1, data)
		}
	}
}

func TestExecuteWithCanceledContextDoesNotStartHTTPRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	hub := logevent.NewHub()
	events, unsubscribe := hub.Subscribe("task-1")
	defer unsubscribe()
	doer := &fakeHTTPDoer{
		responses: []fakeHTTPResponse{
			{statusCode: 200, body: `{"should":"not run"}`},
		},
	}
	executor := NewExecutor(doer, hub)

	result := executor.Execute(ctx, "task-1", 1, testAccount("account-1"), Scenario{
		Steps: []Step{
			{Name: "canceled", Method: "GET", URL: "https://api.example.com/canceled"},
		},
	})

	if result.Success {
		t.Fatal("Execute() Success = true, want false")
	}
	if !strings.Contains(result.ErrMsg, context.Canceled.Error()) {
		t.Fatalf("ErrMsg = %q, want context canceled", result.ErrMsg)
	}
	if got := doer.CallCount(); got != 0 {
		t.Fatalf("HTTP call count = %d, want 0", got)
	}
	if got := len(result.Steps); got != 0 {
		t.Fatalf("len(Result.Steps) = %d, want 0", got)
	}
	assertNoEvent(t, events)
}

func TestExecutorCanBeSharedConcurrently(t *testing.T) {
	doer := doerFunc(func(ctx context.Context, method, url string, body []byte, headers map[string]string) (int, []byte, error) {
		if err := ctx.Err(); err != nil {
			return 0, nil, err
		}
		return 200, []byte(`{"value":41}`), nil
	})
	executor := NewExecutor(doer, logevent.NewHub())
	sc := Scenario{
		Steps: []Step{
			{
				Name:    "value",
				Method:  "GET",
				URL:     "https://api.example.com/{{.account.ID}}/value",
				Extract: map[string]string{"value": "value"},
			},
		},
		Formula: "value + 1",
	}

	var wg sync.WaitGroup
	errs := make(chan error, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			acc := testAccount(fmt.Sprintf("account-%d", i))
			result := executor.Execute(context.Background(), fmt.Sprintf("task-%d", i), i, acc, sc)
			if !result.Success {
				errs <- fmt.Errorf("Execute(%d) failed: %s", i, result.ErrMsg)
				return
			}
			if result.FormulaResult != 42 {
				errs <- fmt.Errorf("Execute(%d) FormulaResult = %v, want 42", i, result.FormulaResult)
			}
		}(i)
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

type fakeHTTPResponse struct {
	statusCode int
	body       string
	err        error
}

type fakeHTTPCall struct {
	method  string
	url     string
	body    string
	headers map[string]string
}

type fakeHTTPDoer struct {
	mu        sync.Mutex
	responses []fakeHTTPResponse
	calls     []fakeHTTPCall
}

func (d *fakeHTTPDoer) Do(ctx context.Context, method, url string, body []byte, headers map[string]string) (int, []byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	copiedHeaders := make(map[string]string, len(headers))
	for key, value := range headers {
		copiedHeaders[key] = value
	}
	d.calls = append(d.calls, fakeHTTPCall{
		method:  method,
		url:     url,
		body:    string(append([]byte(nil), body...)),
		headers: copiedHeaders,
	})

	if err := ctx.Err(); err != nil {
		return 0, nil, err
	}
	index := len(d.calls) - 1
	if index >= len(d.responses) {
		return 0, nil, errors.New("unexpected HTTP call")
	}
	resp := d.responses[index]
	return resp.statusCode, []byte(resp.body), resp.err
}

func (d *fakeHTTPDoer) Calls() []fakeHTTPCall {
	d.mu.Lock()
	defer d.mu.Unlock()

	calls := make([]fakeHTTPCall, len(d.calls))
	for i, call := range d.calls {
		calls[i] = fakeHTTPCall{
			method:  call.method,
			url:     call.url,
			body:    call.body,
			headers: make(map[string]string, len(call.headers)),
		}
		for key, value := range call.headers {
			calls[i].headers[key] = value
		}
	}
	return calls
}

func (d *fakeHTTPDoer) CallCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	return len(d.calls)
}

type doerFunc func(ctx context.Context, method, url string, body []byte, headers map[string]string) (int, []byte, error)

func (f doerFunc) Do(ctx context.Context, method, url string, body []byte, headers map[string]string) (int, []byte, error) {
	return f(ctx, method, url, body, headers)
}

func testAccount(id string) *accountpool.Account {
	return &accountpool.Account{
		ID:       id,
		Username: "alice",
		Password: "secret",
	}
}

func receiveEvent(t *testing.T, ch <-chan logevent.Event) logevent.Event {
	t.Helper()

	select {
	case event, ok := <-ch:
		if !ok {
			t.Fatal("event channel closed before receiving event")
		}
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
		return logevent.Event{}
	}
}

func assertNoEvent(t *testing.T, ch <-chan logevent.Event) {
	t.Helper()

	select {
	case event, ok := <-ch:
		if ok {
			t.Fatalf("unexpected event received: %#v", event)
		}
		t.Fatal("event channel closed unexpectedly")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestFakeHTTPDoerCopiesCalls(t *testing.T) {
	doer := &fakeHTTPDoer{
		responses: []fakeHTTPResponse{{statusCode: 200}},
	}
	headers := map[string]string{"X-Test": "one"}
	_, _, err := doer.Do(context.Background(), "GET", "https://api.example.com", []byte("body"), headers)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	headers["X-Test"] = "mutated"

	calls := doer.Calls()
	want := []fakeHTTPCall{
		{
			method:  "GET",
			url:     "https://api.example.com",
			body:    "body",
			headers: map[string]string{"X-Test": "one"},
		},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("Calls() = %#v, want %#v", calls, want)
	}
}
