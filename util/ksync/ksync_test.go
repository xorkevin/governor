package ksync

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWaitGroup(t *testing.T) {
	t.Parallel()

	t.Run("waits with no count", func(t *testing.T) {
		t.Parallel()

		assert := require.New(t)

		wg := NewWaitGroup()
		func() {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			assert.NoError(wg.Wait(ctx))
			select {
			case <-ctx.Done():
				assert.Fail("Failed to close done channel")
			case <-wg.C():
			}
		}()
	})

	t.Run("waits with a count", func(t *testing.T) {
		t.Parallel()

		assert := require.New(t)

		wg := NewWaitGroup()
		wg.Add(1)
		go func(wg Waiter) {
			defer wg.Done()
			time.Sleep(50 * time.Millisecond)
		}(wg)
		func() {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			assert.NoError(wg.Wait(ctx))
			select {
			case <-ctx.Done():
				assert.Fail("Failed to close done channel")
			case <-wg.C():
			}
		}()
	})

	t.Run("does not panic on multiple dones", func(t *testing.T) {
		t.Parallel()

		wg := NewWaitGroup()
		wg.Done()
		wg.Done()
		wg.Add(3)
		wg.Done()
		wg.Done()
	})

	t.Run("errors if deadline exceeded", func(t *testing.T) {
		t.Parallel()

		assert := require.New(t)

		wg := NewWaitGroup()
		wg.Add(1)
		func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()
			assert.ErrorIs(wg.Wait(ctx), context.DeadlineExceeded)
			select {
			case <-ctx.Done():
			case <-wg.C():
				assert.Fail("Should not have closed done channel")
			}
		}()
	})

	t.Run("context cancellation takes priority in wait", func(t *testing.T) {
		t.Parallel()

		assert := require.New(t)

		wg := NewWaitGroup()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		assert.ErrorIs(wg.Wait(ctx), context.Canceled)
	})
}

type (
	testobj struct {
		field int64
	}
)

func TestSingleFlight(t *testing.T) {
	t.Parallel()

	t.Run("waits with no count", func(t *testing.T) {
		t.Parallel()

		assert := require.New(t)

		sf := NewSingleFlight[testobj]()

		block := make(chan struct{})
		count := &atomic.Int64{}
		fn := func(ctx context.Context) (*testobj, error) {
			select {
			case <-block:
				return &testobj{
					field: count.Add(1),
				}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		wg := NewWaitGroup()
		var ret1, ret2 *testobj
		var err1, err2 error
		wg.Add(1)
		go func() {
			defer wg.Done()
			ret1, err1 = sf.Do(context.Background(), context.Background(), fn)
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			ret2, err2 = sf.Do(context.Background(), context.Background(), fn)
		}()
		<-time.After(10 * time.Millisecond)
		close(block)
		wg.Wait(context.Background())
		assert.NoError(err1)
		assert.NoError(err2)
		assert.NotNil(ret1)
		assert.True(ret1 == ret2)
	})
}
