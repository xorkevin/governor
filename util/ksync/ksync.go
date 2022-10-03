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

type (
	flightCall[T any] struct {
		wg       *WaitGroup
		val      *T
		err      error
		panicked interface{}
	}

	// SingleFlight ensures that only one function call may be performed at a
	// given time, and the results of that function call may be shared among any
	// callers waiting on its completion
	SingleFlight[T any] struct {
		mu   *sync.Mutex
		call *flightCall[T]
	}
)

// NewSingleFlight creates a new [*SingleFlight]
func NewSingleFlight[T any]() *SingleFlight[T] {
	return &SingleFlight[T]{
		mu: &sync.Mutex{},
	}
}

// Do executes a function, ensuring that only one is in-flight, and sharing the
// results among callers waiting on its completion.
func (s *SingleFlight[T]) Do(ctx context.Context, fn func(ctx context.Context) (*T, error)) (*T, error) {
	s.mu.Lock()

	if s.call != nil {
		c := s.call
		s.mu.Unlock()
		if err := c.wg.Wait(ctx); err != nil {
			return nil, err
		}
		if c.panicked != nil {
			panic(c.panicked)
		}
		return c.val, c.err
	}

	c := &flightCall[T]{
		wg: NewWaitGroup(),
	}
	c.wg.Add(1)
	s.call = c
	s.mu.Unlock()

	return func() (*T, error) {
		defer func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			c.wg.Done()
			if s.call == c {
				s.call = nil
			}
		}()

		defer func() {
			if r := recover(); r != nil {
				// save panicked value for other callers
				c.panicked = r
				panic(r)
			}
		}()

		c.val, c.err = fn(ctx)
		return c.val, c.err
	}()
}
