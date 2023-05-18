package lifecycle

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"xorkevin.dev/governor/util/ksync"
	"xorkevin.dev/klog"
)

// ErrClosed is returned when the lifecycle has been closed
var ErrClosed errClosed

type (
	errClosed struct{}
)

func (e errClosed) Error() string {
	return "Lifecycle closed"
}

type (
	// Manager is an interface to interact with a lifecycle
	Manager[T any] struct {
		l *Lifecycle[T]
	}

	// State is an interface to interact with lifecycle state
	State[T any] struct {
		l *Lifecycle[T]
	}

	// Lifecycle manages the lifecycle of connecting to external services
	Lifecycle[T any] struct {
		aclient       atomic.Pointer[T]
		sf            ksync.SingleFlight[T]
		doctx         context.Context
		cancelctx     func()
		closed        bool
		constructorfn func(ctx context.Context, m *State[T]) (*T, error)
		stopfn        func(ctx context.Context, client *T)
		heartbeatfn   func(ctx context.Context, m *Manager[T])
		hbinterval    time.Duration
	}
)

// New creates a new [*Lifecycle]
func New[T any](
	doctx context.Context,
	constructorfn func(ctx context.Context, s *State[T]) (*T, error),
	stopfn func(ctx context.Context, client *T),
	heartbeatfn func(ctx context.Context, m *Manager[T]),
	hbinterval time.Duration,
) *Lifecycle[T] {
	doctx, cancel := context.WithCancel(doctx)
	return &Lifecycle[T]{
		aclient:       atomic.Pointer[T]{},
		sf:            ksync.SingleFlight[T]{},
		doctx:         doctx,
		cancelctx:     cancel,
		closed:        false,
		constructorfn: constructorfn,
		stopfn:        stopfn,
		heartbeatfn:   heartbeatfn,
		hbinterval:    hbinterval,
	}
}

// Load returns the cached instance
func (l *Lifecycle[T]) Load(ctx context.Context) *T {
	return l.aclient.Load()
}

func (l *Lifecycle[T]) constructWithState(ctx context.Context) (*T, error) {
	if l.closed {
		return nil, ErrClosed
	}

	return l.constructorfn(ctx, &State[T]{
		l: l,
	})
}

// Construct constructs an instance
func (l *Lifecycle[T]) Construct(ctx context.Context) (*T, error) {
	return l.sf.Do(l.doctx, ctx, l.constructWithState)
}

func (l *Lifecycle[T]) stop(ctx context.Context) {
	l.stopfn(klog.ExtendCtx(context.Background(), ctx), l.aclient.Swap(nil))
}

func (l *Lifecycle[T]) closeConstruction(ctx context.Context) (*T, error) {
	l.closed = true
	return nil, ErrClosed
}

func (l *Lifecycle[T]) waitUntilClosed() {
	for {
		if _, err := l.sf.Do(context.Background(), context.Background(), l.closeConstruction); errors.Is(err, ErrClosed) {
			return
		}
	}
}

// Heartbeat calls the heartbeat function at an interval and calls the stop
// function when the context is closed.
func (l *Lifecycle[T]) Heartbeat(ctx context.Context, wg *ksync.WaitGroup) {
	defer wg.Done()
	defer l.stop(ctx)
	defer l.waitUntilClosed()
	defer l.cancelctx()
	ticker := time.NewTicker(l.hbinterval)
	defer ticker.Stop()
	m := &Manager[T]{
		l: l,
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.heartbeatfn(ctx, m)
		}
	}
}

// Stop stops and removes an instance
func (m *State[T]) Stop(ctx context.Context) {
	m.l.stop(ctx)
}

// Load returns the cached instance
func (m *State[T]) Load(ctx context.Context) *T {
	return m.l.Load(ctx)
}

// Store stores the cached instance
func (m *State[T]) Store(client *T) {
	m.l.aclient.Store(client)
}

// Construct constructs an instance
func (m *Manager[T]) Construct(ctx context.Context) (*T, error) {
	return m.l.Construct(ctx)
}

// Stop stops and removes an instance
func (m *Manager[T]) Stop(ctx context.Context) {
	m.l.stop(ctx)
}
