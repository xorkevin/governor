package sysevent

import (
	"xorkevin.dev/governor/util/kjson"
	"xorkevin.dev/kerrors"
)

type (
	// GCEvent is a gc event
	GCEvent struct {
		Timestamp int64 `json:"timestamp"`
	}
)

const (
	// GCChannel is a gc event channel
	GCChannel = "gov.sys.gc"
)

// DecodeGCEvent decodes a gc event
func DecodeGCEvent(msgdata []byte) (*GCEvent, error) {
	m := &GCEvent{}
	if err := kjson.Unmarshal(msgdata, m); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to decode gc event")
	}
	return m, nil
}
