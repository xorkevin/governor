package ksync

import (
	"context"
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
}
