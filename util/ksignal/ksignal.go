package ksignal

import (
	"context"
	"os"
	"os/signal"
)

// Wait blocks until a signal is received
func Wait(ctx context.Context, signals ...os.Signal) {
	notifyCtx, stop := signal.NotifyContext(ctx, signals...)
	defer stop()
	<-notifyCtx.Done()
}
