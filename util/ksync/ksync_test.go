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
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			assert.NoError(wg.Wait(ctx))
			select {
			case <-ctx.Done():
			case <-wg.C():
				assert.Fail("Prematurely closed done channel")
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

func TestOnce(t *testing.T) {
	t.Parallel()

	t.Run("runs a function once", func(t *testing.T) {
		t.Parallel()

		assert := require.New(t)

		once := NewOnce[testobj]()
		count := &atomic.Int64{}
		fn := func() (*testobj, error) {
			return &testobj{
				field: count.Add(1),
			}, nil
		}
		v1, err := once.Do(fn)
		assert.NoError(err)
		v2, err := once.Do(fn)
		assert.NoError(err)
		assert.True(v1 == v2)
	})
}

func TestSingleFlight(t *testing.T) {
	t.Parallel()

	t.Run("shares return values among multiple callers", func(t *testing.T) {
		t.Parallel()

		assert := require.New(t)

		var sf SingleFlight[testobj]

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		block := make(chan struct{})
		count := &atomic.Int64{}
		fn := func(innerctx context.Context) (*testobj, error) {
			assert.True(innerctx == ctx)
			select {
			case <-block:
				return &testobj{
					field: count.Add(1),
				}, nil
			case <-innerctx.Done():
				return nil, innerctx.Err()
			}
		}
		wg := NewWaitGroup()
		var ret1, ret2 *testobj
		var err1, err2 error
		wg.Add(1)
		go func() {
			defer wg.Done()
			ret1, err1 = sf.Do(ctx, ctx, fn)
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			ret2, err2 = sf.Do(ctx, ctx, fn)
		}()
		<-time.After(10 * time.Millisecond)
		close(block)
		wg.Wait(ctx)
		assert.NoError(err1)
		assert.NoError(err2)
		assert.NotNil(ret1)
		assert.True(ret1 == ret2)
	})

	t.Run("allows function to finish despite wait context cancellation", func(t *testing.T) {
		t.Parallel()

		assert := require.New(t)

		var sf SingleFlight[testobj]

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
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			ret1, err1 = sf.Do(context.Background(), ctx, fn)
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			ret2, err2 = sf.Do(context.Background(), context.Background(), fn)
		}()
		<-time.After(10 * time.Millisecond)
		close(block)
		wg.Wait(context.Background())
		assert.ErrorIs(err1, context.Canceled)
		assert.NoError(err2)
		assert.Nil(ret1)
		assert.Equal(&testobj{
			field: 1,
		}, ret2)
	})

	t.Run("shares panics among callers", func(t *testing.T) {
		t.Parallel()

		assert := require.New(t)

		var sf SingleFlight[testobj]

		block := make(chan struct{})
		count := &atomic.Int64{}
		fn := func(ctx context.Context) (*testobj, error) {
			select {
			case <-block:
				panic(&testobj{
					field: count.Add(1),
				})
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		wg := NewWaitGroup()
		var pan1, pan2 *testobj
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					p, ok := r.(*testobj)
					assert.True(ok)
					pan1 = p
				}
			}()
			sf.Do(context.Background(), context.Background(), fn)
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					p, ok := r.(*testobj)
					assert.True(ok)
					pan2 = p
				}
			}()
			sf.Do(context.Background(), context.Background(), fn)
		}()
		<-time.After(10 * time.Millisecond)
		close(block)
		wg.Wait(context.Background())
		assert.NotNil(pan1)
		assert.True(pan1 == pan2)
	})
}
