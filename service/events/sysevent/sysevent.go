package sysevent

import (
	"context"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/pubsub"
	"xorkevin.dev/governor/util/kjson"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	// TimestampProps
	TimestampProps struct {
		Timestamp int64 `json:"timestamp"`
	}

	// HandlerFuncGC is a type alias for a gc event consumer
	HandlerFuncGC = func(ctx context.Context, props TimestampProps) error

	// SystemEvents is a subscriber for system events
	SystemEvents struct {
		SysChannels governor.SysChannels
		Pubsub      pubsub.Pubsub
	}
)

// New creates a new [*SystemEvents]
func New(c governor.SysChannels, ps pubsub.Pubsub) *SystemEvents {
	return &SystemEvents{
		SysChannels: c,
		Pubsub:      ps,
	}
}

// WatchGC subscribes to system gc events
func (s *SystemEvents) WatchGC(ctx context.Context, log klog.Logger, group string, handler HandlerFuncGC, reqidprefix string) *pubsub.Watcher {
	return pubsub.NewWatcher(s.Pubsub, log, s.SysChannels.GC, group, pubsub.HandlerFunc(func(ctx context.Context, m pubsub.Msg) error {
		props, err := decodeTimestampProps(m.Data)
		if err != nil {
			return err
		}
		return handler(ctx, *props)
	}), reqidprefix)
}

func decodeTimestampProps(msgdata []byte) (*TimestampProps, error) {
	m := &TimestampProps{}
	if err := kjson.Unmarshal(msgdata, m); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to decode sys event timestamp props")
	}
	return m, nil
}
