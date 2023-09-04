package events

import "xorkevin.dev/kerrors"

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
