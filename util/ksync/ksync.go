package ksync

import (
	"context"
	"sync"
	"sync/atomic"
)

type (
	// Waiter waits for completion
	Waiter interface {
		Done()
	}

	// WaitGroup waits on multiple completions
	WaitGroup struct {
		done  chan struct{}
		once  *sync.Once
		count *atomic.Int32
	}
)

// NewWaitGroup creates a new [*WaitGroup]
func NewWaitGroup() *WaitGroup {
	return &WaitGroup{
		done:  make(chan struct{}),
		once:  &sync.Once{},
		count: &atomic.Int32{},
	}
}

// C returns the done channel
func (w *WaitGroup) C() <-chan struct{} {
	if w.count.Load() < 1 {
		closedChan := make(chan struct{})
		close(closedChan)
		return closedChan
	}
	return w.done
}

// Add adds a delta to complete
func (w *WaitGroup) Add(delta int) {
	count := w.count.Add(int32(delta))
	if count < 1 {
		w.once.Do(func() {
			if w.done != nil {
				close(w.done)
			}
		})
	}
}

// Done decrements the counter by 1
func (w *WaitGroup) Done() {
	w.Add(-1)
}

// Wait waits for the group to complete
func (w *WaitGroup) Wait(ctx context.Context) error {
	if w.count.Load() < 1 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-w.done:
		return nil
	}
}
