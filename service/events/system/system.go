package system

import (
	"context"
	"encoding/json"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/kerrors"
)

type (
	// SysEventTimestampProps
	SysEventTimestampProps struct {
		Timestamp int64 `json:"timestamp"`
	}

	// WorkerFuncGC is a type alias for a gc event consumer
	WorkerFuncGC = func(ctx context.Context, props SysEventTimestampProps) error

	// SystemEvents is a subscriber for system events
	SystemEvents struct {
		SysChannels governor.SysChannels
		Events      events.Events
	}
)

// New creates a new [*SystemEvents]
func New(c governor.SysChannels, ev events.Events) *SystemEvents {
	return &SystemEvents{
		SysChannels: c,
	}
}

// SubscribeGC subscribes to system gc events
func (s *SystemEvents) SubscribeGC(group string, worker WorkerFuncGC) (events.Subscription, error) {
	sub, err := s.Events.Subscribe(s.SysChannels.GC, group, func(ctx context.Context, topic string, msgdata []byte) error {
		props, err := decodeSysEventTimestampProps(msgdata)
		if err != nil {
			return err
		}
		return worker(ctx, *props)
	})
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to subscribe to system gc channel")
	}
	return sub, nil
}

func decodeSysEventTimestampProps(msgdata []byte) (*SysEventTimestampProps, error) {
	m := &SysEventTimestampProps{}
	if err := json.Unmarshal(msgdata, m); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to decode sys event timestamp props")
	}
	return m, nil
}
