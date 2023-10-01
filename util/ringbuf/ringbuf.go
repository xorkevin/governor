package ringbuf

type (
	Ring[T any] struct {
		buf []T
		r   int
		w   int
	}
)

func New[T any]() *Ring[T] {
	return &Ring[T]{
		buf: make([]T, 2),
		r:   0,
		w:   0,
	}
}

func (b *Ring[T]) resize() {
	next := make([]T, len(b.buf)*2)
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

func (b *Ring[T]) Write(m T) {
	next := (b.w + 1) % len(b.buf)
	if next == b.r {
		b.resize()
		b.Write(m)
		return
	}
	b.buf[b.w] = m
	b.w = next
}

func (b *Ring[T]) Read() (*T, bool) {
	if b.r == b.w {
		return nil, false
	}
	next := (b.r + 1) % len(b.buf)
	m := b.buf[b.r]
	b.r = next
	return &m, true
}

func (b *Ring[T]) Peek() (*T, bool) {
	if b.r == b.w {
		return nil, false
	}
	m := b.buf[b.r]
	return &m, true
}
