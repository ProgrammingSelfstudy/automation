package workerpool

import (
	"context"
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
)

func TestRunCoversEachAccountExactlyOnceWithPerAccountSeqNos(t *testing.T) {
	for iteration := 0; iteration < 10; iteration++ {
		t.Run(fmt.Sprintf("iteration-%02d", iteration), func(t *testing.T) {
			accounts := makeAccounts(10)
			accountPool := accountpool.NewAccountPool(accounts)
			doer := &recordingHTTPDoer{
				delay: func(accID string, callNo int) time.Duration {
					return time.Duration((accountNumber(accID)*callNo+iteration)%7) * time.Millisecond
				},
			}
			sink := &memorySink{}
			pool := NewPool(accountPool, scenario.NewExecutor(doer, nil), sink)

			summary := pool.Run(context.Background(), "task-coverage", workerScenario(), 5, 3)

			if got, want := summary.TotalCount, 50; got != want {
				t.Fatalf("TotalCount = %d, want %d", got, want)
			}
			if got, want := summary.SuccessCount, 50; got != want {
				t.Fatalf("SuccessCount = %d, want %d", got, want)
			}
			if summary.FailCount != 0 || summary.SaveErrorCount != 0 {
				t.Fatalf("unexpected failures: %+v", summary)
			}
			assertPoolIdle(t, accountPool, len(accounts))

			records := sink.Records()
			if got, want := len(records), 50; got != want {
				t.Fatalf("record count = %d, want %d", got, want)
			}
			assertPerAccountRecords(t, records, accounts, 5)
		})
	}
}

func TestRunRespectsConcurrencyLimit(t *testing.T) {
	accounts := makeAccounts(8)
	doer := &recordingHTTPDoer{
		delay: fixedDelay(10 * time.Millisecond),
	}
	pool := NewPool(accountpool.NewAccountPool(accounts), scenario.NewExecutor(doer, nil), &memorySink{})

	summary := pool.Run(context.Background(), "task-concurrency", workerScenario(), 4, 3)

	if got, want := summary.SuccessCount, 32; got != want {
		t.Fatalf("SuccessCount = %d, want %d", got, want)
	}
	if got, want := doer.MaxActive(), int64(3); got > want {
		t.Fatalf("max active executions = %d, want <= %d", got, want)
	}
}

func TestRunClampsConcurrencyAboveAccountCount(t *testing.T) {
	accounts := makeAccounts(4)
	doer := &recordingHTTPDoer{
		delay: fixedDelay(10 * time.Millisecond),
	}
	pool := NewPool(accountpool.NewAccountPool(accounts), scenario.NewExecutor(doer, nil), &memorySink{})

	summary := pool.Run(context.Background(), "task-clamp-high", workerScenario(), 3, 100)

	if got, want := summary.SuccessCount, 12; got != want {
		t.Fatalf("SuccessCount = %d, want %d", got, want)
	}
	if got, want := doer.MaxActive(), int64(len(accounts)); got > want {
		t.Fatalf("max active executions = %d, want <= %d", got, want)
	}
}

func TestRunClampsConcurrencyBelowOne(t *testing.T) {
	accounts := makeAccounts(3)
	doer := &recordingHTTPDoer{
		delay: fixedDelay(5 * time.Millisecond),
	}
	pool := NewPool(accountpool.NewAccountPool(accounts), scenario.NewExecutor(doer, nil), &memorySink{})

	summary := pool.Run(context.Background(), "task-clamp-low", workerScenario(), 2, 0)

	if got, want := summary.SuccessCount, 6; got != want {
		t.Fatalf("SuccessCount = %d, want %d", got, want)
	}
	if got, want := doer.MaxActive(), int64(1); got > want {
		t.Fatalf("max active executions = %d, want <= %d", got, want)
	}
}

func TestRunReturnsPromptlyWhenContextIsCanceled(t *testing.T) {
	accounts := makeAccounts(5)
	accountPool := accountpool.NewAccountPool(accounts)
	doer := &recordingHTTPDoer{
		delay: fixedDelay(100 * time.Millisecond),
	}
	sink := &memorySink{}
	pool := NewPool(accountPool, scenario.NewExecutor(doer, nil), sink)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan Summary, 1)
	go func() {
		done <- pool.Run(ctx, "task-cancel", workerScenario(), 100, 5)
	}()

	waitFor(t, time.Second, func() bool {
		return doer.CurrentActive() == int64(len(accounts))
	})
	cancel()

	var summary Summary
	select {
	case summary = <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return promptly after context cancellation")
	}

	if got, want := summary.TotalCount, 500; got != want {
		t.Fatalf("TotalCount = %d, want %d", got, want)
	}
	completed := summary.SuccessCount + summary.FailCount
	if completed < 0 || completed > summary.TotalCount {
		t.Fatalf("completed count = %d is outside [0, %d]", completed, summary.TotalCount)
	}
	if completed >= summary.TotalCount {
		t.Fatalf("completed count = %d, want less than total after cancellation", completed)
	}
	if got := len(sink.Records()); got != completed {
		t.Fatalf("saved records = %d, want completed count %d", got, completed)
	}
	assertPoolIdle(t, accountPool, len(accounts))
}

func TestRunContinuesWhenResultSinkSaveFails(t *testing.T) {
	accounts := makeAccounts(3)
	sink := &memorySink{
		saveErr: func(record savedRecord) error {
			if record.seqNo%2 == 0 {
				return errors.New("save failed")
			}
			return nil
		},
	}
	pool := NewPool(
		accountpool.NewAccountPool(accounts),
		scenario.NewExecutor(&recordingHTTPDoer{}, nil),
		sink,
	)

	summary := pool.Run(context.Background(), "task-save-error", workerScenario(), 4, 3)

	if got, want := summary.TotalCount, 12; got != want {
		t.Fatalf("TotalCount = %d, want %d", got, want)
	}
	if got, want := summary.SuccessCount, 12; got != want {
		t.Fatalf("SuccessCount = %d, want %d", got, want)
	}
	if got, want := summary.FailCount, 0; got != want {
		t.Fatalf("FailCount = %d, want %d", got, want)
	}
	if got, want := summary.SaveErrorCount, 6; got != want {
		t.Fatalf("SaveErrorCount = %d, want %d", got, want)
	}
	if got, want := len(sink.Records()), 12; got != want {
		t.Fatalf("saved call count = %d, want %d", got, want)
	}
}

func TestRunSummarizesSuccessAndFailureCounts(t *testing.T) {
	accounts := makeAccounts(4)
	doer := &recordingHTTPDoer{
		statusCode: func(accID string, callNo int) int {
			if accID == "acc-02" && callNo == 2 {
				return 500
			}
			if accID == "acc-03" && callNo == 1 {
				return 503
			}
			return 200
		},
	}
	pool := NewPool(accountpool.NewAccountPool(accounts), scenario.NewExecutor(doer, nil), &memorySink{})

	summary := pool.Run(context.Background(), "task-summary", workerScenario(), 3, 4)

	if got, want := summary.TotalCount, 12; got != want {
		t.Fatalf("TotalCount = %d, want %d", got, want)
	}
	if got, want := summary.SuccessCount, 10; got != want {
		t.Fatalf("SuccessCount = %d, want %d", got, want)
	}
	if got, want := summary.FailCount, 2; got != want {
		t.Fatalf("FailCount = %d, want %d", got, want)
	}
	if got, want := summary.SaveErrorCount, 0; got != want {
		t.Fatalf("SaveErrorCount = %d, want %d", got, want)
	}
}

func TestRunWithNonPositivePerAccountCountDoesNothing(t *testing.T) {
	accounts := makeAccounts(3)
	accountPool := accountpool.NewAccountPool(accounts)
	doer := &recordingHTTPDoer{}
	sink := &memorySink{}
	pool := NewPool(accountPool, scenario.NewExecutor(doer, nil), sink)

	for _, perAccountCount := range []int{0, -1} {
		summary := pool.Run(context.Background(), "task-empty", workerScenario(), perAccountCount, 3)
		if summary.TaskID != "task-empty" {
			t.Fatalf("TaskID = %q, want %q", summary.TaskID, "task-empty")
		}
		if summary.TotalCount != 0 || summary.SuccessCount != 0 || summary.FailCount != 0 || summary.SaveErrorCount != 0 {
			t.Fatalf("summary = %+v, want empty counts", summary)
		}
	}

	if got := doer.CallCount(); got != 0 {
		t.Fatalf("HTTP call count = %d, want 0", got)
	}
	if got := len(sink.Records()); got != 0 {
		t.Fatalf("saved records = %d, want 0", got)
	}
	assertPoolIdle(t, accountPool, len(accounts))
}

func TestPoolCanRunMultipleTimes(t *testing.T) {
	accounts := makeAccounts(3)
	accountPool := accountpool.NewAccountPool(accounts)
	pool := NewPool(accountPool, scenario.NewExecutor(&recordingHTTPDoer{}, nil), &memorySink{})

	first := pool.Run(context.Background(), "task-1", workerScenario(), 2, 2)
	second := pool.Run(context.Background(), "task-2", workerScenario(), 2, 2)

	for i, summary := range []Summary{first, second} {
		if got, want := summary.TotalCount, 6; got != want {
			t.Fatalf("run %d TotalCount = %d, want %d", i+1, got, want)
		}
		if got, want := summary.SuccessCount, 6; got != want {
			t.Fatalf("run %d SuccessCount = %d, want %d", i+1, got, want)
		}
		if summary.FailCount != 0 || summary.SaveErrorCount != 0 {
			t.Fatalf("run %d unexpected failures: %+v", i+1, summary)
		}
	}
	assertPoolIdle(t, accountPool, len(accounts))
}

type savedRecord struct {
	taskID string
	accID  string
	seqNo  int
	result scenario.Result
}

type memorySink struct {
	mu      sync.Mutex
	records []savedRecord
	saveErr func(savedRecord) error
}

func (s *memorySink) Save(_ context.Context, taskID string, acc *accountpool.Account, seqNo int, result scenario.Result) error {
	record := savedRecord{
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

	if s.saveErr != nil {
		return s.saveErr(record)
	}
	return nil
}

func (s *memorySink) Records() []savedRecord {
	s.mu.Lock()
	defer s.mu.Unlock()

	records := make([]savedRecord, len(s.records))
	copy(records, s.records)
	return records
}

type recordingHTTPDoer struct {
	mu         sync.Mutex
	calls      map[string]int
	delay      func(accID string, callNo int) time.Duration
	statusCode func(accID string, callNo int) int
	active     atomic.Int64
	maxActive  atomic.Int64
	totalCalls atomic.Int64
}

func (d *recordingHTTPDoer) Do(ctx context.Context, method, rawURL string, body []byte, headers map[string]string) (int, []byte, error) {
	accID := accountIDFromURL(rawURL)
	callNo := d.nextCall(accID)

	active := d.active.Add(1)
	updateMax(&d.maxActive, active)
	defer d.active.Add(-1)

	if d.delay != nil {
		delay := d.delay(accID, callNo)
		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return 0, nil, ctx.Err()
			}
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

func (d *recordingHTTPDoer) nextCall(accID string) int {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.calls == nil {
		d.calls = make(map[string]int)
	}
	d.calls[accID]++
	d.totalCalls.Add(1)
	return d.calls[accID]
}

func (d *recordingHTTPDoer) MaxActive() int64 {
	return d.maxActive.Load()
}

func (d *recordingHTTPDoer) CurrentActive() int64 {
	return d.active.Load()
}

func (d *recordingHTTPDoer) CallCount() int {
	return int(d.totalCalls.Load())
}

func makeAccounts(count int) []*accountpool.Account {
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

func workerScenario() scenario.Scenario {
	return scenario.Scenario{
		Steps: []scenario.Step{
			{
				Name:   "run",
				Method: "POST",
				URL:    "https://worker.test/run/{{.account.ID}}",
			},
		},
	}
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

func accountNumber(accID string) int {
	var number int
	_, _ = fmt.Sscanf(accID, "acc-%02d", &number)
	return number
}

func fixedDelay(delay time.Duration) func(string, int) time.Duration {
	return func(string, int) time.Duration {
		return delay
	}
}

func updateMax(max *atomic.Int64, value int64) {
	for {
		current := max.Load()
		if value <= current {
			return
		}
		if max.CompareAndSwap(current, value) {
			return
		}
	}
}

func assertPerAccountRecords(t *testing.T, records []savedRecord, accounts []*accountpool.Account, perAccountCount int) {
	t.Helper()

	byAccount := make(map[string]map[int]int, len(accounts))
	for _, record := range records {
		if record.taskID == "" {
			t.Fatal("record taskID is empty")
		}
		if byAccount[record.accID] == nil {
			byAccount[record.accID] = make(map[int]int)
		}
		byAccount[record.accID][record.seqNo]++
	}

	for _, acc := range accounts {
		seqs := byAccount[acc.ID]
		if got, want := len(seqs), perAccountCount; got != want {
			t.Fatalf("account %s seq count = %d, want %d; seqs=%v", acc.ID, got, want, seqs)
		}
		for seqNo := 1; seqNo <= perAccountCount; seqNo++ {
			if got := seqs[seqNo]; got != 1 {
				t.Fatalf("account %s seq %d count = %d, want 1; seqs=%v", acc.ID, seqNo, got, seqs)
			}
		}
	}
	if got, want := len(byAccount), len(accounts); got != want {
		t.Fatalf("account coverage count = %d, want %d", got, want)
	}
}

func assertPoolIdle(t *testing.T, accountPool *accountpool.AccountPool, want int) {
	t.Helper()

	if got := accountPool.IdleCount(); got != want {
		t.Fatalf("IdleCount = %d, want %d", got, want)
	}
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
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
