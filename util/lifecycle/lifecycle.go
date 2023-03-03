package lifecycle

import (
	"context"
	"sync/atomic"
	"time"

	"xorkevin.dev/governor/util/ksync"
	"xorkevin.dev/klog"
)

type (
	// Manager is an interface to interact with a lifecycle
	Manager[T any] struct {
		l *Lifecycle[T]
	}

	// Lifecycle manages the lifecycle of connecting to external services
	Lifecycle[T any] struct {
		aclient       *atomic.Pointer[T]
		sf            *ksync.SingleFlight[T]
		doctx         context.Context
		constructorfn func(ctx context.Context, m *Manager[T]) (*T, error)
		stopfn        func(ctx context.Context, client *T)
		heartbeatfn   func(ctx context.Context, m *Manager[T])
		hbinterval    time.Duration
	}
)

// New creates a new [*Lifecycle]
func New[T any](
	doctx context.Context,
	constructorfn func(ctx context.Context, m *Manager[T]) (*T, error),
	stopfn func(ctx context.Context, client *T),
	heartbeatfn func(ctx context.Context, m *Manager[T]),
	hbinterval time.Duration,
) *Lifecycle[T] {
	return &Lifecycle[T]{
		aclient:       &atomic.Pointer[T]{},
		sf:            ksync.NewSingleFlight[T](),
		doctx:         doctx,
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

func (l *Lifecycle[T]) constructWithManager(ctx context.Context) (*T, error) {
	m := &Manager[T]{
		l: l,
	}
	return l.constructorfn(ctx, m)
}

// Construct constructs an instance
func (l *Lifecycle[T]) Construct(ctx context.Context) (*T, error) {
	return l.sf.Do(l.doctx, ctx, l.constructWithManager)
}

// Heartbeat calls the heartbeat function at an interval and calls the stop
// function when the context is closed.
func (l *Lifecycle[T]) Heartbeat(ctx context.Context, wg *ksync.WaitGroup) {
	defer wg.Done()
	ticker := time.NewTicker(l.hbinterval)
	defer ticker.Stop()
	m := &Manager[T]{
		l: l,
	}
	for {
		select {
		case <-ctx.Done():
			l.stopfn(klog.ExtendCtx(context.Background(), ctx), l.aclient.Swap(nil))
			return
		case <-ticker.C:
			l.heartbeatfn(ctx, m)
		}
	}
}

// Construct constructs an instance
func (m *Manager[T]) Construct(ctx context.Context) (*T, error) {
	return m.l.Construct(ctx)
}

// Stop stops and removes an instance
func (m *Manager[T]) Stop(ctx context.Context) {
	m.l.stopfn(klog.ExtendCtx(context.Background(), ctx), m.l.aclient.Swap(nil))
}

// Load returns the cached instance
func (m *Manager[T]) Load(ctx context.Context) *T {
	return m.l.Load(ctx)
}

// Store stores the cached instance
func (m *Manager[T]) Store(client *T) {
	m.l.aclient.Store(client)
}
