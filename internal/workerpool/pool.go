package workerpool

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"interface-load-test/internal/accountpool"
	"interface-load-test/internal/scenario"
)

// ResultSink persists one scenario execution result.
type ResultSink interface {
	// Save persists one execution result. A non-nil error does not stop the pool;
	// it is counted in Summary.SaveErrorCount only.
	Save(ctx context.Context, taskID string, acc *accountpool.Account, seqNo int, result scenario.Result) error
}

// Pool coordinates account-scoped scenario execution.
type Pool struct {
	accounts *accountpool.AccountPool
	executor *scenario.Executor
	sink     ResultSink
}

// Summary describes one Run outcome.
type Summary struct {
	TaskID         string
	TotalCount     int
	SuccessCount   int
	FailCount      int
	SaveErrorCount int
	Duration       time.Duration
}

type noopSink struct{}

func (noopSink) Save(context.Context, string, *accountpool.Account, int, scenario.Result) error {
	return nil
}

// NewPool creates a worker pool.
func NewPool(accounts *accountpool.AccountPool, executor *scenario.Executor, sink ResultSink) *Pool {
	if sink == nil {
		sink = noopSink{}
	}
	return &Pool{
		accounts: accounts,
		executor: executor,
		sink:     sink,
	}
}

// Run executes perAccountCount scenario rows for each account, with concurrency
// clamped to [1, accounts.Len()].
func (p *Pool) Run(ctx context.Context, taskID string, sc scenario.Scenario, perAccountCount int, concurrency int) (summary Summary) {
	if ctx == nil {
		ctx = context.Background()
	}

	startedAt := time.Now()
	summary = Summary{TaskID: taskID}
	defer func() {
		summary.Duration = time.Since(startedAt)
	}()

	if p == nil || p.accounts == nil || p.executor == nil || perAccountCount <= 0 {
		return summary
	}

	accountCount := p.accounts.Len()
	if accountCount <= 0 {
		return summary
	}

	summary.TotalCount = accountCount * perAccountCount
	workerCount := clampConcurrency(concurrency, accountCount)

	var tickets atomic.Int64
	var successCount atomic.Int64
	var failCount atomic.Int64
	var saveErrorCount atomic.Int64

	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				if ctx.Err() != nil {
					return
				}

				ticket := int(tickets.Add(1))
				if ticket > accountCount {
					return
				}

				acc, err := p.accounts.Acquire(ctx)
				if err != nil {
					return
				}
				p.runAccount(ctx, taskID, sc, perAccountCount, acc, &successCount, &failCount, &saveErrorCount)
			}
		}()
	}

	wg.Wait()

	summary.SuccessCount = int(successCount.Load())
	summary.FailCount = int(failCount.Load())
	summary.SaveErrorCount = int(saveErrorCount.Load())
	return summary
}

func (p *Pool) runAccount(
	ctx context.Context,
	taskID string,
	sc scenario.Scenario,
	perAccountCount int,
	acc *accountpool.Account,
	successCount *atomic.Int64,
	failCount *atomic.Int64,
	saveErrorCount *atomic.Int64,
) {
	defer p.accounts.Release(acc)

	for seqNo := 1; seqNo <= perAccountCount; seqNo++ {
		if ctx.Err() != nil {
			return
		}

		result := p.executor.Execute(ctx, taskID, seqNo, acc, sc)
		if result.Success {
			successCount.Add(1)
		} else {
			failCount.Add(1)
		}

		if err := p.sink.Save(ctx, taskID, acc, seqNo, result); err != nil {
			saveErrorCount.Add(1)
		}

		if ctx.Err() != nil {
			return
		}
	}
}

func clampConcurrency(concurrency int, accountCount int) int {
	if accountCount <= 0 {
		return 0
	}
	if concurrency <= 0 {
		return 1
	}
	if concurrency > accountCount {
		return accountCount
	}
	return concurrency
}
