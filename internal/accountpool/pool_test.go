package accountpool

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestAcquireReleaseReturnsSameAccount(t *testing.T) {
	account := &Account{ID: "1", Username: "user1", Password: "pass1"}
	pool := NewAccountPool([]*Account{account})

	acquired, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if acquired != account {
		t.Fatalf("Acquire() = %p, want %p", acquired, account)
	}

	pool.Release(acquired)

	acquiredAgain, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire() after Release() error = %v", err)
	}
	if acquiredAgain != account {
		t.Fatalf("Acquire() after Release() = %p, want %p", acquiredAgain, account)
	}
}

func TestAcquireBlocksWhenPoolExhaustedUntilRelease(t *testing.T) {
	pool := NewAccountPool([]*Account{
		{ID: "1", Username: "user1", Password: "pass1"},
		{ID: "2", Username: "user2", Password: "pass2"},
	})

	first, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("first Acquire() error = %v", err)
	}
	second, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("second Acquire() error = %v", err)
	}

	acquired := make(chan *Account, 1)
	errs := make(chan error, 1)
	go func() {
		acc, err := pool.Acquire(context.Background())
		if err != nil {
			errs <- err
			return
		}
		acquired <- acc
	}()

	select {
	case acc := <-acquired:
		t.Fatalf("third Acquire() returned before Release(): %v", acc)
	case err := <-errs:
		t.Fatalf("third Acquire() returned unexpected error before Release(): %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	pool.Release(first)

	select {
	case err := <-errs:
		t.Fatalf("third Acquire() returned error after Release(): %v", err)
	case acc := <-acquired:
		if acc != first {
			t.Fatalf("third Acquire() = %p, want released account %p", acc, first)
		}
	case <-time.After(time.Second):
		t.Fatal("third Acquire() did not return after Release()")
	}

	pool.Release(second)
	pool.Release(first)
}

func TestAcquireReturnsContextDeadlineExceededWhenExhausted(t *testing.T) {
	pool := NewAccountPool([]*Account{
		{ID: "1", Username: "user1", Password: "pass1"},
	})

	acquired, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	defer pool.Release(acquired)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	acc, err := pool.Acquire(ctx)
	if acc != nil {
		t.Fatalf("Acquire() account = %v, want nil", acc)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Acquire() error = %v, want %v", err, context.DeadlineExceeded)
	}
	if elapsed := time.Since(start); elapsed < 100*time.Millisecond {
		t.Fatalf("Acquire() returned too early after %v", elapsed)
	}
}

func TestConcurrentAcquireReleaseIsSafe(t *testing.T) {
	pool := NewAccountPool([]*Account{
		{ID: "1", Username: "user1", Password: "pass1"},
		{ID: "2", Username: "user2", Password: "pass2"},
		{ID: "3", Username: "user3", Password: "pass3"},
		{ID: "4", Username: "user4", Password: "pass4"},
		{ID: "5", Username: "user5", Password: "pass5"},
	})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			acc, err := pool.Acquire(context.Background())
			if err != nil {
				t.Errorf("Acquire() error = %v", err)
				return
			}
			time.Sleep(time.Millisecond)
			pool.Release(acc)
		}()
	}

	wg.Wait()

	if got, want := pool.IdleCount(), pool.Len(); got != want {
		t.Fatalf("IdleCount() = %d, want %d", got, want)
	}
}

func TestLenAndIdleCount(t *testing.T) {
	pool := NewAccountPool([]*Account{
		{ID: "1", Username: "user1", Password: "pass1"},
		{ID: "2", Username: "user2", Password: "pass2"},
		{ID: "3", Username: "user3", Password: "pass3"},
	})

	if got, want := pool.Len(), 3; got != want {
		t.Fatalf("Len() = %d, want %d", got, want)
	}
	if got, want := pool.IdleCount(), 3; got != want {
		t.Fatalf("IdleCount() after init = %d, want %d", got, want)
	}

	first, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("first Acquire() error = %v", err)
	}
	if got, want := pool.IdleCount(), 2; got != want {
		t.Fatalf("IdleCount() after first Acquire() = %d, want %d", got, want)
	}

	second, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("second Acquire() error = %v", err)
	}
	if got, want := pool.IdleCount(), 1; got != want {
		t.Fatalf("IdleCount() after second Acquire() = %d, want %d", got, want)
	}
	if got, want := pool.Len(), 3; got != want {
		t.Fatalf("Len() after Acquire() = %d, want %d", got, want)
	}

	pool.Release(first)
	if got, want := pool.IdleCount(), 2; got != want {
		t.Fatalf("IdleCount() after first Release() = %d, want %d", got, want)
	}

	pool.Release(second)
	if got, want := pool.IdleCount(), 3; got != want {
		t.Fatalf("IdleCount() after second Release() = %d, want %d", got, want)
	}
}
