package accountpool

import "context"

// Account is a test account managed by AccountPool.
type Account struct {
	ID       string
	Username string
	Password string
}

// AccountPool manages a fixed-size pool of test accounts.
type AccountPool struct {
	accounts chan *Account
	total    int
}

// NewAccountPool initializes a pool with all supplied accounts marked idle.
func NewAccountPool(accounts []*Account) *AccountPool {
	p := &AccountPool{
		accounts: make(chan *Account, len(accounts)),
		total:    len(accounts),
	}

	for _, acc := range accounts {
		p.accounts <- acc
	}

	return p
}

// Acquire returns an idle account, blocking until one is released or ctx is done.
func (p *AccountPool) Acquire(ctx context.Context) (*Account, error) {
	select {
	case acc := <-p.accounts:
		return acc, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Release returns an account to the pool.
//
// The caller must guarantee that each successful Acquire is matched by exactly
// one Release. Calling Release more times than Acquire, releasing the same
// Account twice, or forgetting to Release an acquired Account is a caller bug.
func (p *AccountPool) Release(acc *Account) {
	p.accounts <- acc
}

// Len returns the total number of accounts in the pool.
func (p *AccountPool) Len() int {
	return p.total
}

// IdleCount returns the current number of idle accounts in the pool.
func (p *AccountPool) IdleCount() int {
	return len(p.accounts)
}
