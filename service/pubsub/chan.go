package pubsub

import (
	"context"
	"sync"

	"xorkevin.dev/governor/util/ringbuf"
	"xorkevin.dev/kerrors"
)

var _ Pubsub = (*MuxChan)(nil)

type (
	MuxChan struct {
		topics map[string]*chanTopic
		mu     sync.RWMutex
	}

	chanTopic struct {
		individuals map[*chanSubscription]struct{}
		groups      map[string]*chanGroup
	}

	chanGroup struct {
		sub  *chanSubscription
		subs map[*chanSubscription]struct{}
	}

	chanSubscription struct {
		s      *MuxChan
		topic  string
		group  string
		rCond  *sync.Cond
		ring   *ringbuf.Ring[Msg]
		closed bool
		done   chan struct{}
	}
)

func NewMuxChan() *MuxChan {
	return &MuxChan{
		topics: map[string]*chanTopic{},
	}
}

func (s *MuxChan) Subscribe(ctx context.Context, topic, group string) (Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.topics[topic]
	if !ok {
		t = &chanTopic{
			individuals: map[*chanSubscription]struct{}{},
			groups:      map[string]*chanGroup{},
		}
		s.topics[topic] = t
	}
	sub := &chanSubscription{
		s:      s,
		topic:  topic,
		group:  group,
		rCond:  sync.NewCond(s.mu.RLocker()),
		ring:   ringbuf.New[Msg](),
		closed: false,
		done:   make(chan struct{}),
	}
	if group == "" {
		t.individuals[sub] = struct{}{}
	} else {
		g, ok := t.groups[group]
		if !ok {
			g = &chanGroup{
				sub:  sub,
				subs: map[*chanSubscription]struct{}{},
			}
			t.groups[group] = g
		}
		g.subs[sub] = struct{}{}
		if g.sub == nil {
			g.sub = sub
		}
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
	if sub.group == "" {
		delete(t.individuals, sub)
		if len(t.individuals) == 0 && len(t.groups) == 0 {
			delete(s.topics, sub.topic)
		}
	} else {
		g, ok := t.groups[sub.group]
		if !ok {
			return
		}
		delete(g.subs, sub)
		if g.sub == sub {
			g.sub = nil
			for k := range g.subs {
				g.sub = k
				g.sub.rCond.Broadcast()
				break
			}
			if g.sub == nil {
				delete(t.groups, sub.group)
				if len(t.individuals) == 0 && len(t.groups) == 0 {
					delete(s.topics, sub.topic)
				}
			}
		}
	}
}

func (s *MuxChan) Publish(ctx context.Context, topic string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.topics[topic]
	if !ok {
		return nil
	}
	m := Msg{
		Subject: topic,
		Data:    data,
	}
	for i := range t.individuals {
		i.ring.Write(m)
		i.rCond.Broadcast()
	}
	for _, g := range t.groups {
		if g.sub != nil {
			g.sub.ring.Write(m)
			g.sub.rCond.Broadcast()
		}
	}
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

			m, ok := s.ring.Read()
			if !ok {
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

func (s *chanSubscription) Close(ctx context.Context) error {
	select {
	case <-s.done:
		return nil
	default:
	}

	s.s.unsubscribe(ctx, s)
	return nil
}
