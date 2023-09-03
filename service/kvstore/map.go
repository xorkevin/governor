package kvstore

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"xorkevin.dev/kerrors"
)

type (
	Map struct {
		store map[string]mapEntry
		mu    sync.Mutex
	}

	mapEntry struct {
		val    string
		expire time.Time
	}
)

func NewMap() *Map {
	return &Map{
		store: map[string]mapEntry{},
	}
}

func (s *Map) Ping(ctx context.Context) error {
	return nil
}

func (s *Map) Get(ctx context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	v, ok := s.store[key]
	if !ok || v.expire.Before(time.Now()) {
		delete(s.store, key)
		return "", kerrors.WithKind(nil, ErrNotFound, "Key not found")
	}
	return v.val, nil
}

func (s *Map) GetInt(ctx context.Context, key string) (int64, error) {
	v, err := s.Get(ctx, key)
	if err != nil {
		return 0, err
	}
	num, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, kerrors.WithKind(err, ErrVal, "Invalid int value")
	}
	return num, nil
}

func (s *Map) Set(ctx context.Context, key, val string, duration time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if duration <= 0 {
		delete(s.store, key)
		return nil
	}
	s.store[key] = mapEntry{
		val:    val,
		expire: time.Now().Add(duration),
	}
	return nil
}

func (s *Map) SetNX(ctx context.Context, key, val string, duration time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if v, ok := s.store[key]; ok && v.expire.After(time.Now()) {
		return false, nil
	}
	if duration <= 0 {
		delete(s.store, key)
		return true, nil
	}
	s.store[key] = mapEntry{
		val:    val,
		expire: time.Now().Add(duration),
	}
	return true, nil
}

func (s *Map) Del(ctx context.Context, key ...string) error {
	if len(key) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, i := range key {
		delete(s.store, i)
	}
	return nil
}

func (s *Map) Incr(ctx context.Context, key string, delta int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	v, ok := s.store[key]
	if !ok || v.expire.Before(time.Now()) {
		v = mapEntry{
			val: "0",
			// incr does not add an expiration, so set a value far in the future
			expire: time.Now().Add(24 * time.Hour),
		}
	}
	num, err := strconv.ParseInt(v.val, 10, 64)
	if err != nil {
		return 0, kerrors.WithKind(err, ErrVal, "Invalid int value")
	}
	num++
	v.val = strconv.FormatInt(num, 10)
	s.store[key] = v
	return num, nil
}

func (s *Map) Expire(ctx context.Context, key string, duration time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if duration <= 0 {
		delete(s.store, key)
		return nil
	}
	v, ok := s.store[key]
	if !ok || v.expire.Before(time.Now()) {
		delete(s.store, key)
		return nil
	}
	v.expire = time.Now().Add(duration)
	s.store[key] = v
	return nil
}

func (s *Map) Subkey(keypath ...string) string {
	if len(keypath) == 0 {
		return ""
	}
	return strings.Join(keypath, kvpathSeparator)
}

func (s *Map) Multi(ctx context.Context) (Multi, error) {
	return &mapMulti{
		m: s,
	}, nil
}

func (s *Map) Subtree(prefix string) KVStore {
	return &tree{
		prefix: prefix,
		base:   s,
	}
}

type (
	mapMulti struct {
		m   *Map
		ops []deferredOp
	}

	deferredOp interface {
		run()
	}

	mapMultiResult struct {
		op  func()
		v   string
		err error
	}

	mapMultiIntResult struct {
		op  func()
		v   int64
		err error
	}

	mapMultiStatusResult struct {
		op  func()
		err error
	}

	mapMultiBoolResult struct {
		op  func()
		v   bool
		err error
	}

	errIncomplete struct{}
)

func (e errIncomplete) Error() string {
	return "Incomplete"
}

func (r *mapMultiResult) Result() (string, error) {
	return r.v, r.err
}

func (r *mapMultiResult) run() {
	r.op()
}

func (r *mapMultiIntResult) Result() (int64, error) {
	return r.v, r.err
}

func (r *mapMultiIntResult) run() {
	r.op()
}

func (r *mapMultiStatusResult) Result() error {
	return r.err
}

func (r *mapMultiStatusResult) run() {
	r.op()
}

func (r *mapMultiBoolResult) Result() (bool, error) {
	return r.v, r.err
}

func (r *mapMultiBoolResult) run() {
	r.op()
}

func (t *mapMulti) Get(ctx context.Context, key string) Resulter {
	r := &mapMultiResult{
		err: errIncomplete{},
	}
	r.op = func() {
		r.v, r.err = t.m.Get(ctx, key)
	}
	t.ops = append(t.ops, r)
	return r
}

func (t *mapMulti) GetInt(ctx context.Context, key string) IntResulter {
	r := &mapMultiIntResult{
		err: errIncomplete{},
	}
	r.op = func() {
		r.v, r.err = t.m.GetInt(ctx, key)
	}
	t.ops = append(t.ops, r)
	return r
}

func (t *mapMulti) Set(ctx context.Context, key, val string, duration time.Duration) ErrResulter {
	r := &mapMultiStatusResult{
		err: errIncomplete{},
	}
	r.op = func() {
		r.err = t.m.Set(ctx, key, val, duration)
	}
	t.ops = append(t.ops, r)
	return r
}

func (t *mapMulti) SetNX(ctx context.Context, key, val string, duration time.Duration) BoolResulter {
	r := &mapMultiBoolResult{
		err: errIncomplete{},
	}
	r.op = func() {
		r.v, r.err = t.m.SetNX(ctx, key, val, duration)
	}
	t.ops = append(t.ops, r)
	return r
}

func (t *mapMulti) Del(ctx context.Context, key ...string) ErrResulter {
	r := &mapMultiStatusResult{
		err: errIncomplete{},
	}
	r.op = func() {
		r.err = t.m.Del(ctx, key...)
	}
	t.ops = append(t.ops, r)
	return r
}

func (t *mapMulti) Incr(ctx context.Context, key string, delta int64) IntResulter {
	r := &mapMultiIntResult{
		err: errIncomplete{},
	}
	r.op = func() {
		r.v, r.err = t.m.Incr(ctx, key, delta)
	}
	t.ops = append(t.ops, r)
	return r
}

func (t *mapMulti) Expire(ctx context.Context, key string, duration time.Duration) ErrResulter {
	r := &mapMultiStatusResult{
		err: errIncomplete{},
	}
	r.op = func() {
		r.err = t.m.Expire(ctx, key, duration)
	}
	t.ops = append(t.ops, r)
	return r
}

func (t *mapMulti) Subkey(keypath ...string) string {
	if len(keypath) == 0 {
		return ""
	}
	return strings.Join(keypath, kvpathSeparator)
}

func (t *mapMulti) Subtree(prefix string) Multi {
	return &multi{
		prefix: prefix,
		base:   t,
	}
}

func (t *mapMulti) Exec(ctx context.Context) error {
	for _, i := range t.ops {
		i.run()
	}
	return nil
}
