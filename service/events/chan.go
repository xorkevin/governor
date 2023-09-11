package events

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

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
		offset int
	}

	chanGroup struct {
		ring *ringBuffer
		sub  *chanSubscription
		subs map[*chanSubscription]struct{}
	}

	chanSubscription struct {
		s      *MuxChan
		topic  string
		group  string
		rCond  *sync.Cond
		closed bool
		done   chan struct{}
	}
)

func (s *MuxChan) Subscribe(ctx context.Context, topic, group string, opts ConsumerOpts) (Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.topics[topic]
	if !ok {
		return nil, kerrors.WithKind(nil, ErrNotFound, fmt.Sprintf("Unknown topic: %s", topic))
	}
	sub := &chanSubscription{
		s:      s,
		topic:  topic,
		group:  group,
		rCond:  sync.NewCond(s.mu.RLocker()),
		closed: false,
		done:   make(chan struct{}),
	}
	g, ok := t.groups[group]
	if !ok {
		g = &chanGroup{
			ring: &ringBuffer{
				buf: make([]Msg, 2),
				r:   0,
				w:   0,
			},
			sub:  sub,
			subs: map[*chanSubscription]struct{}{},
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

	if sub.closed {
		return
	}

	sub.closed = true
	close(sub.done)

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
		m := Msg{
			Topic:     i.Topic,
			Key:       i.Key,
			Value:     i.Value,
			Partition: 0,
			Offset:    t.offset,
			Time:      i.Time,
		}
		if m.Time.IsZero() {
			m.Time = now
		}
		for _, g := range t.groups {
			g.ring.Write(m)
			if g.sub != nil {
				g.sub.rCond.Broadcast()
			}
		}
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
		offset: 0,
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
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	var rMsg *Msg
	var rerr error

	done := make(chan struct{})
	defer func() {
		<-done
	}()
	exiting := make(chan struct{})

	go func() {
		defer close(done)
		s.rCond.L.Lock()
		defer s.rCond.L.Unlock()

		for {
			select {
			case <-exiting:
				return
			default:
			}

			if s.closed {
				rerr = kerrors.WithKind(nil, ErrClientClosed, "Client closed")
				return
			}

			t, ok := s.s.topics[s.topic]
			if !ok {
				rerr = kerrors.WithKind(nil, ErrNotFound, fmt.Sprintf("Unknown topic: %s", s.topic))
				return
			}
			g, ok := t.groups[s.group]
			if !ok {
				rerr = kerrors.WithKind(nil, ErrNotFound, fmt.Sprintf("Unknown group: %s", s.group))
				return
			}
			if g.sub != s {
				s.rCond.Wait()
				continue
			}

			m, err := g.ring.Peek()
			if err != nil {
				if !errors.Is(err, ErrReadEmpty) {
					rerr = err
					return
				}
				s.rCond.Wait()
				continue
			}
			rMsg = m
			return
		}
	}()

	select {
	case <-ctx.Done():
		close(exiting)
		s.rCond.Broadcast()
		return nil, ctx.Err()
	case <-done:
		return rMsg, rerr
	}
}

func (s *chanSubscription) isAssigned(msg Msg) bool {
	if msg.Topic != s.topic {
		return false
	}

	s.s.mu.RLock()
	defer s.s.mu.RUnlock()

	if s.closed {
		return false
	}
	t, ok := s.s.topics[s.topic]
	if !ok {
		return false
	}
	g, ok := t.groups[s.group]
	if !ok {
		return false
	}
	return g.sub == s
}

func (s *chanSubscription) MsgUnassigned(msg Msg) <-chan struct{} {
	if msg.Topic != s.topic {
		ch := make(chan struct{})
		close(ch)
		return ch
	}

	s.s.mu.RLock()
	defer s.s.mu.RUnlock()

	if s.closed {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	t, ok := s.s.topics[s.topic]
	if !ok {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	g, ok := t.groups[s.group]
	if !ok {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	if g.sub != s {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return s.done
}

func (s *chanSubscription) Commit(ctx context.Context, msg Msg) error {
	return nil
}

func (s *chanSubscription) Close(ctx context.Context) error {
	return nil
}
