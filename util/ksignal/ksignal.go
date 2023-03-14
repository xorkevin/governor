package ksignal

import (
	"context"
	"os"
	"os/signal"
)

// Wait blocks until a signal is received
func Wait(ctx context.Context, signals ...os.Signal) {
	if len(signals) == 0 {
		signals = []os.Signal{os.Interrupt}
	}
	notifyCtx, stop := signal.NotifyContext(ctx, signals...)
	defer stop()
	<-notifyCtx.Done()
}
