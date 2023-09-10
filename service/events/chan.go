package events

import (
	"context"
	"fmt"
	"sync"
	"time"

	"xorkevin.dev/governor/util/ksync"
	"xorkevin.dev/kerrors"
)

type (
	ringBuffer struct {
		buf []Msg
		r   int
		w   int
	}
)

func (b *ringBuffer) resize() {
	next := make([]Msg, len(b.buf)*2)
	if b.r == b.w {
		b.w = 0
	} else if b.r < b.w {
		b.w = copy(next, b.buf[b.r:b.w])
	} else {
		p := copy(next, b.buf[b.r:])
		q := 0
		if b.w > 0 {
			q = copy(next[p:], b.buf[:b.w])
		}
		b.w = p + q
	}
	b.buf = next
	b.r = 0
}

func (b *ringBuffer) Write(m Msg) {
	next := (b.w + 1) % len(b.buf)
	if next == b.r {
		b.resize()
		b.Write(m)
		return
	}
	b.buf[b.w] = m
	b.w = next
}

func (b *ringBuffer) Read() (*Msg, error) {
	if b.r == b.w {
		return nil, kerrors.WithKind(nil, ErrReadEmpty, "No messages")
	}
	next := (b.r + 1) % len(b.buf)
	m := b.buf[b.r]
	b.r = next
	return &m, nil
}

func (b *ringBuffer) Peek() (*Msg, error) {
	if b.r == b.w {
		return nil, kerrors.WithKind(nil, ErrReadEmpty, "No messages")
	}
	m := b.buf[b.r]
	return &m, nil
}

type (
	MuxChan struct {
		topics map[string]*chanTopic
		mu     sync.RWMutex
	}

	chanTopic struct {
		groups map[string]*chanGroup
		ring   *ringBuffer
		offset int
	}

	chanGroup struct {
		sub    *chanSubscription
		subs   map[*chanSubscription]struct{}
		offset int
	}

	chanSubscription struct {
		s     *MuxChan
		topic string
		group string
	}
)

func (s *MuxChan) Curate(ctx context.Context, wg *ksync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		}
	}
}

func (s *MuxChan) writeMsgs(ctx context.Context, msgs []PublishMsg) {
}

func (s *MuxChan) Subscribe(ctx context.Context, topic, group string, opts ConsumerOpts) (Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.topics[topic]
	if !ok {
		return nil, kerrors.WithKind(nil, ErrNotFound, fmt.Sprintf("Unknown topic: %s", topic))
	}
	sub := &chanSubscription{
		s:     s,
		topic: topic,
		group: group,
	}
	g, ok := t.groups[group]
	if !ok {
		g = &chanGroup{
			sub:    sub,
			subs:   map[*chanSubscription]struct{}{},
			offset: 0,
		}
		t.groups[group] = g
	}
	g.subs[sub] = struct{}{}
	if g.sub == nil {
		g.sub = sub
	}
	return sub, nil
}

func (s *MuxChan) unsubscribe(ctx context.Context, sub *chanSubscription) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.topics[sub.topic]
	if !ok {
		return
	}
	g, ok := t.groups[sub.group]
	if !ok {
		return
	}
	delete(g.subs, sub)
	if g.sub == sub {
		g.sub = nil
		for k := range g.subs {
			g.sub = k
			break
		}
	}
}

func (s *MuxChan) Publish(ctx context.Context, msgs ...PublishMsg) error {
	if len(msgs) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC().Round(0)
	for _, i := range msgs {
		t, ok := s.topics[i.Topic]
		if !ok {
			return kerrors.WithKind(nil, ErrNotFound, fmt.Sprintf("Unknown topic: %s", i.Topic))
		}
		t.offset++
		ti := i.Time
		if ti.IsZero() {
			ti = now
		}
		t.ring.Write(Msg{
			Topic:     i.Topic,
			Key:       i.Key,
			Value:     i.Value,
			Partition: 0,
			Offset:    t.offset,
			Time:      ti,
		})
	}
	return nil
}

func (s *MuxChan) InitStream(ctx context.Context, topic string, opts StreamOpts) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.topics[topic]; ok {
		return nil
	}

	s.topics[topic] = &chanTopic{
		groups: map[string]*chanGroup{},
		ring: &ringBuffer{
			buf: make([]Msg, 2),
			r:   0,
			w:   0,
		},
	}
	return nil
}

func (s *MuxChan) DeleteStream(ctx context.Context, topic string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.topics[topic]; !ok {
		return nil
	}

	delete(s.topics, topic)
	return nil
}

func (s *chanSubscription) ReadMsg(ctx context.Context) (*Msg, error) {
	return nil, nil
}

func (s *chanSubscription) IsAssigned(msg Msg) bool {
	return false
}

func (s *chanSubscription) MsgUnassigned(msg Msg) <-chan struct{} {
	return nil
}

func (s *chanSubscription) Commit(ctx context.Context, msg Msg) error {
	return nil
}

func (s *chanSubscription) Close(ctx context.Context) error {
	return nil
}

func (s *chanSubscription) IsClosed() bool {
	return false
}
