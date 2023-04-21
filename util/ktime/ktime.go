package ktime

import (
	"context"
	"fmt"
	"time"
)

func After(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("Context closed: %w", context.Cause(ctx))
	case <-t.C:
		return nil
	}
}
