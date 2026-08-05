package loadtest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"interface-load-test/internal/accountpool"
	"interface-load-test/internal/scenario"
	"interface-load-test/internal/task"
)

func TestValidateConfig(t *testing.T) {
	module := NewModule(nil, nil)
	valid := mustConfigJSON(t, Config{
		Scenario: scenario.Scenario{
			Steps: []scenario.Step{{Name: "run", Method: "GET", URL: "https://load.test/{{.account.ID}}"}},
		},
		PerAccountCount: 1,
	})

	if err := module.ValidateConfig(valid); err != nil {
		t.Fatalf("ValidateConfig(valid) error = %v", err)
	}
}

func TestValidateConfigRejectsInvalidInputs(t *testing.T) {
	module := NewModule(nil, nil)
	tests := []struct {
		name string
		cfg  json.RawMessage
	}{
		{
			name: "empty steps",
			cfg: mustConfigJSON(t, Config{
				PerAccountCount: 1,
			}),
		},
		{
			name: "invalid per account count",
			cfg: mustConfigJSON(t, Config{
				Scenario: scenario.Scenario{
					Steps: []scenario.Step{{Name: "run", Method: "GET", URL: "https://load.test"}},
				},
				PerAccountCount: 0,
			}),
		},
		{
			name: "bad json",
			cfg:  []byte(`{"scenario":`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := module.ValidateConfig(tt.cfg); err == nil {
				t.Fatal("ValidateConfig() error = nil, want error")
			}
		})
	}
}

func TestValidateConfigWrapsJSONSyntaxError(t *testing.T) {
	module := NewModule(nil, nil)

	err := module.ValidateConfig([]byte(`{"scenario":`))
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("ValidateConfig() error = %v, want %v", err, ErrInvalidConfig)
	}
}

func TestExecutionCount(t *testing.T) {
	module := NewModule(nil, nil)

	count, err := module.ExecutionCount(mustConfigJSON(t, loadTestConfig(7)))
	if err != nil {
		t.Fatalf("ExecutionCount() error = %v", err)
	}
	if got, want := count, 7; got != want {
		t.Fatalf("ExecutionCount() = %d, want %d", got, want)
	}

	if _, err := module.ExecutionCount([]byte(`{"scenario":`)); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("ExecutionCount() error = %v, want %v", err, ErrInvalidConfig)
	}
}

func TestRunExecutesScenarioAndSavesResults(t *testing.T) {
	doer := &loadTestHTTPDoer{
		statusCode: func(accID string, callNo int) int {
			if accID == "acc-02" && callNo == 2 {
				return 500
			}
			return 200
		},
	}
	sink := &loadTestSink{}
	module := NewModule(nil, sink)
	module.httpDoer = doer
	accounts := loadTestAccounts(2)

	result, err := module.Run(context.Background(), &task.Task{
		ID:          "task-1",
		ModuleType:  Type,
		Config:      mustConfigJSON(t, loadTestConfig(2)),
		Concurrency: 2,
	}, accounts)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if got, want := result.SuccessCount, 3; got != want {
		t.Fatalf("SuccessCount = %d, want %d", got, want)
	}
	if got, want := result.FailCount, 1; got != want {
		t.Fatalf("FailCount = %d, want %d", got, want)
	}

	records := sink.Records()
	if got, want := len(records), 4; got != want {
		t.Fatalf("saved records = %d, want %d", got, want)
	}
	assertLoadTestRecords(t, records, accounts, 2)
}

func TestRunReturnsEarlyWhenContextCanceled(t *testing.T) {
	doer := &loadTestHTTPDoer{
		delay: 50 * time.Millisecond,
	}
	sink := &loadTestSink{}
	module := NewModule(nil, sink)
	module.httpDoer = doer
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan task.RunResult, 1)
	go func() {
		result, err := module.Run(ctx, &task.Task{
			ID:          "task-cancel",
			ModuleType:  Type,
			Config:      mustConfigJSON(t, loadTestConfig(100)),
			Concurrency: 3,
		}, loadTestAccounts(3))
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}
		done <- result
	}()

	waitForLoadTest(t, time.Second, func() bool {
		return doer.CurrentActive() == 3
	})
	cancel()

	select {
	case result := <-done:
		completed := result.SuccessCount + result.FailCount
		if completed >= 300 {
			t.Fatalf("completed = %d, want fewer than all rows after cancellation", completed)
		}
	case <-time.After(time.Second):
		t.Fatal("Run() did not return after context cancellation")
	}
}

type loadTestRecord struct {
	taskID string
	accID  string
	seqNo  int
	result scenario.Result
}

type loadTestSink struct {
	mu      sync.Mutex
	records []loadTestRecord
}

func (s *loadTestSink) Save(_ context.Context, taskID string, acc *accountpool.Account, seqNo int, result scenario.Result) error {
	record := loadTestRecord{
		taskID: taskID,
		seqNo:  seqNo,
		result: result,
	}
	if acc != nil {
		record.accID = acc.ID
	}

	s.mu.Lock()
	s.records = append(s.records, record)
	s.mu.Unlock()
	return nil
}

func (s *loadTestSink) Records() []loadTestRecord {
	s.mu.Lock()
	defer s.mu.Unlock()

	records := make([]loadTestRecord, len(s.records))
	copy(records, s.records)
	return records
}

type loadTestHTTPDoer struct {
	mu         sync.Mutex
	calls      map[string]int
	statusCode func(accID string, callNo int) int
	delay      time.Duration
	active     atomic.Int64
}

func (d *loadTestHTTPDoer) Do(ctx context.Context, method, rawURL string, body []byte, headers map[string]string) (int, []byte, error) {
	accID := accountIDFromURL(rawURL)
	callNo := d.nextCall(accID)

	d.active.Add(1)
	defer d.active.Add(-1)

	if d.delay > 0 {
		select {
		case <-time.After(d.delay):
		case <-ctx.Done():
			return 0, nil, ctx.Err()
		}
	}
	if err := ctx.Err(); err != nil {
		return 0, nil, err
	}

	statusCode := 200
	if d.statusCode != nil {
		statusCode = d.statusCode(accID, callNo)
	}
	return statusCode, []byte(`{"ok":true}`), nil
}

func (d *loadTestHTTPDoer) nextCall(accID string) int {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.calls == nil {
		d.calls = make(map[string]int)
	}
	d.calls[accID]++
	return d.calls[accID]
}

func (d *loadTestHTTPDoer) CurrentActive() int64 {
	return d.active.Load()
}

func loadTestConfig(perAccountCount int) Config {
	return Config{
		Scenario: scenario.Scenario{
			Steps: []scenario.Step{
				{Name: "run", Method: "POST", URL: "https://load.test/run/{{.account.ID}}"},
			},
		},
		PerAccountCount: perAccountCount,
	}
}

func mustConfigJSON(t *testing.T, cfg Config) json.RawMessage {
	t.Helper()

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	return data
}

func loadTestAccounts(count int) []*accountpool.Account {
	accounts := make([]*accountpool.Account, 0, count)
	for i := 1; i <= count; i++ {
		accounts = append(accounts, &accountpool.Account{
			ID:       fmt.Sprintf("acc-%02d", i),
			Username: fmt.Sprintf("user-%02d", i),
			Password: "secret",
		})
	}
	return accounts
}

func accountIDFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func assertLoadTestRecords(t *testing.T, records []loadTestRecord, accounts []*accountpool.Account, perAccountCount int) {
	t.Helper()

	byAccount := make(map[string]map[int]int, len(accounts))
	for _, record := range records {
		if byAccount[record.accID] == nil {
			byAccount[record.accID] = make(map[int]int)
		}
		byAccount[record.accID][record.seqNo]++
	}

	for _, acc := range accounts {
		seqs := byAccount[acc.ID]
		if got, want := len(seqs), perAccountCount; got != want {
			t.Fatalf("account %s seq count = %d, want %d", acc.ID, got, want)
		}
		for seqNo := 1; seqNo <= perAccountCount; seqNo++ {
			if got := seqs[seqNo]; got != 1 {
				t.Fatalf("account %s seq %d count = %d, want 1", acc.ID, seqNo, got)
			}
		}
	}
}

func waitForLoadTest(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	if condition() {
		return
	}
	t.Fatalf("condition was not met within %v", timeout)
}
