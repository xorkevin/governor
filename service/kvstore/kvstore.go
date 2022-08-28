package kvstore

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-redis/redis/v8"
	"xorkevin.dev/governor"
	"xorkevin.dev/kerrors"
)

const (
	kvpathSeparator = ":"
)

type (
	// Resulter returns the result of a string command in a multi after it has executed
	Resulter interface {
		Result() (string, error)
	}

	// IntResulter returns the result of an int command in a multi after it has executed
	IntResulter interface {
		Result() (int64, error)
	}

	// BoolResulter returns the result of a bool command in a multi after it has executed
	BoolResulter interface {
		Result() (bool, error)
	}

	// ErrResulter returns the err of a command in a multi after it has executed
	ErrResulter interface {
		Result() error
	}

	// Multi is a kvstore multi
	Multi interface {
		Get(ctx context.Context, key string) Resulter
		GetInt(ctx context.Context, key string) IntResulter
		Set(ctx context.Context, key, val string, seconds int64) ErrResulter
		Del(ctx context.Context, key ...string) ErrResulter
		Incr(ctx context.Context, key string, delta int64) IntResulter
		Expire(ctx context.Context, key string, seconds int64) BoolResulter
		Subkey(keypath ...string) string
		Subtree(prefix string) Multi
		Exec(ctx context.Context) error
	}

	// KVStore is a service wrapper around a kv store client
	KVStore interface {
		Get(ctx context.Context, key string) (string, error)
		GetInt(ctx context.Context, key string) (int64, error)
		Set(ctx context.Context, key, val string, seconds int64) error
		Del(ctx context.Context, key ...string) error
		Incr(ctx context.Context, key string, delta int64) (int64, error)
		Expire(ctx context.Context, key string, seconds int64) error
		Subkey(keypath ...string) string
		Multi(ctx context.Context) (Multi, error)
		Tx(ctx context.Context) (Multi, error)
		Subtree(prefix string) KVStore
	}

	// Service is a KVStore and governor.Service
	Service interface {
		governor.Service
		KVStore
	}

	getClientRes struct {
		client *redis.Client
		err    error
	}

	getOp struct {
		ctx context.Context
		res chan<- getClientRes
	}

	service struct {
		client     *redis.Client
		auth       secretAuth
		addr       string
		dbname     int
		config     governor.SecretReader
		logger     governor.Logger
		ops        chan getOp
		ready      *atomic.Bool
		hbfailed   int
		hbinterval int
		hbmaxfail  int
		done       <-chan struct{}
	}

	ctxKeyRootKV struct{}

	ctxKeyKVStore struct{}
)

// getCtxRootKV returns a root KVStore from the context
func getCtxRootKV(inj governor.Injector) KVStore {
	v := inj.Get(ctxKeyRootKV{})
	if v == nil {
		return nil
	}
	return v.(KVStore)
}

// setCtxRootKV sets a root KVStore in the context
func setCtxRootKV(inj governor.Injector, k KVStore) {
	inj.Set(ctxKeyRootKV{}, k)
}

// GetCtxKVStore returns a KVStore from the context
func GetCtxKVStore(inj governor.Injector) KVStore {
	v := inj.Get(ctxKeyKVStore{})
	if v == nil {
		return nil
	}
	return v.(KVStore)
}

// setCtxKVStore sets a KVStore in the context
func setCtxKVStore(inj governor.Injector, k KVStore) {
	inj.Set(ctxKeyKVStore{}, k)
}

// NewSubtreeInCtx creates a new kv subtree with a prefix and sets it in the context
func NewSubtreeInCtx(inj governor.Injector, prefix string) {
	kv := getCtxRootKV(inj)
	setCtxKVStore(inj, kv.Subtree(prefix))
}

// New creates a new cache service
func New() Service {
	return &service{
		ops:      make(chan getOp),
		ready:    &atomic.Bool{},
		hbfailed: 0,
	}
}

func (s *service) Register(name string, inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	setCtxRootKV(inj, s)

	r.SetDefault("auth", "")
	r.SetDefault("dbname", 0)
	r.SetDefault("host", "localhost")
	r.SetDefault("port", "6379")
	r.SetDefault("hbinterval", 5)
	r.SetDefault("hbmaxfail", 5)
}

type (
	// ErrConn is returned on a kvstore connection error
	ErrConn struct{}
	// ErrClient is returned for unknown client errors
	ErrClient struct{}
	// ErrNotFound is returned when a key is not found
	ErrNotFound struct{}
	// ErrVal is returned for invalid value errors
	ErrVal struct{}
)

func (e ErrConn) Error() string {
	return "KVStore connection error"
}

func (e ErrClient) Error() string {
	return "KVStore client error"
}

func (e ErrNotFound) Error() string {
	return "Key not found"
}

func (e ErrVal) Error() string {
	return "Invalid value"
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, m governor.Router) error {
	s.logger = l
	l = s.logger.WithData(map[string]string{
		"phase": "init",
	})

	s.config = r

	s.addr = fmt.Sprintf("%s:%s", r.GetStr("host"), r.GetStr("port"))
	s.dbname = r.GetInt("dbname")
	s.hbinterval = r.GetInt("hbinterval")
	s.hbmaxfail = r.GetInt("hbmaxfail")

	l.Info("Loaded config", map[string]string{
		"addr":       s.addr,
		"dbname":     strconv.Itoa(s.dbname),
		"hbinterval": strconv.Itoa(s.hbinterval),
		"hbmaxfail":  strconv.Itoa(s.hbmaxfail),
	})

	done := make(chan struct{})
	go s.execute(ctx, done)
	s.done = done

	return nil
}

func (s *service) execute(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(time.Duration(s.hbinterval) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.closeClient()
			return
		case <-ticker.C:
			s.handlePing(ctx)
		case op := <-s.ops:
			client, err := s.handleGetClient(ctx)
			select {
			case <-op.ctx.Done():
			case op.res <- getClientRes{
				client: client,
				err:    err,
			}:
				close(op.res)
			}
		}
	}
}

func (s *service) handlePing(ctx context.Context) {
	if s.client != nil {
		_, err := s.client.Ping(ctx).Result()
		if err == nil {
			s.ready.Store(true)
			s.hbfailed = 0
			return
		}
		s.hbfailed++
		if s.hbfailed < s.hbmaxfail {
			s.logger.Warn("Failed to ping kvstore", map[string]string{
				"error":      err.Error(),
				"actiontype": "kvstore_ping",
				"address":    s.addr,
				"dbname":     strconv.Itoa(s.dbname),
			})
			return
		}
		s.logger.Error("Failed max pings to kvstore", map[string]string{
			"error":      err.Error(),
			"actiontype": "kvstore_ping",
			"address":    s.addr,
			"dbname":     strconv.Itoa(s.dbname),
		})
		s.ready.Store(false)
		s.hbfailed = 0
		s.auth = secretAuth{}
		s.config.InvalidateSecret("auth")
	}
	if _, err := s.handleGetClient(ctx); err != nil {
		s.logger.Error("Failed to create kvstore client", map[string]string{
			"error":      err.Error(),
			"actiontype": "kvstore_create_client",
		})
	}
}

type (
	secretAuth struct {
		Password string `mapstructure:"password"`
	}
)

func (s *service) handleGetClient(ctx context.Context) (*redis.Client, error) {
	var secret secretAuth
	if err := s.config.GetSecret(ctx, "auth", 0, &secret); err != nil {
		return nil, kerrors.WithMsg(err, "Invalid secret")
	}
	if secret.Password == "" {
		return nil, kerrors.WithKind(nil, governor.ErrInvalidConfig{}, "Invalid secret")
	}
	if secret == s.auth {
		return s.client, nil
	}

	s.closeClient()

	client := redis.NewClient(&redis.Options{
		Addr:     s.addr,
		Password: secret.Password,
		DB:       s.dbname,
	})
	if _, err := client.Ping(ctx).Result(); err != nil {
		s.config.InvalidateSecret("auth")
		return nil, kerrors.WithKind(err, ErrConn{}, "Failed to ping kvstore")
	}

	s.client = client
	s.auth = secret
	s.ready.Store(true)
	s.hbfailed = 0
	s.logger.Info(fmt.Sprintf("Established connection to %s dbname %d", s.addr, s.dbname), nil)
	return s.client, nil
}

func (s *service) closeClient() {
	if s.client == nil {
		return
	}
	if err := s.client.Close(); err != nil {
		s.logger.Error("Failed to close kvstore connection", map[string]string{
			"error":      err.Error(),
			"actiontype": "kvstore_close",
			"address":    s.addr,
			"dbname":     strconv.Itoa(s.dbname),
		})
	} else {
		s.logger.Info("Closed kvstore connection", map[string]string{
			"actiontype": "kvstore_close_ok",
			"address":    s.addr,
			"dbname":     strconv.Itoa(s.dbname),
		})
	}
	s.client = nil
	s.auth = secretAuth{}
}

func (s *service) Setup(req governor.ReqSetup) error {
	return nil
}

func (s *service) PostSetup(req governor.ReqSetup) error {
	return nil
}

func (s *service) Start(ctx context.Context) error {
	return nil
}

func (s *service) Stop(ctx context.Context) {
	l := s.logger.WithData(map[string]string{
		"phase": "stop",
	})
	select {
	case <-s.done:
		return
	case <-ctx.Done():
		l.Warn("Failed to stop", map[string]string{
			"error":      ctx.Err().Error(),
			"actiontype": "kvstore_stop",
		})
	}
}

func (s *service) Health() error {
	if !s.ready.Load() {
		return kerrors.WithKind(nil, ErrConn{}, "KVStore service not ready")
	}
	return nil
}

func (s *service) getClient(ctx context.Context) (*redis.Client, error) {
	res := make(chan getClientRes)
	op := getOp{
		ctx: ctx,
		res: res,
	}
	select {
	case <-s.done:
		return nil, kerrors.WithMsg(nil, "KVStore service shutdown")
	case <-ctx.Done():
		return nil, kerrors.WithMsg(ctx.Err(), "Context cancelled")
	case s.ops <- op:
		select {
		case <-ctx.Done():
			return nil, kerrors.WithMsg(ctx.Err(), "Context cancelled")
		case v := <-res:
			return v.client, v.err
		}
	}
}

func (s *service) Get(ctx context.Context, key string) (string, error) {
	client, err := s.getClient(ctx)
	if err != nil {
		return "", err
	}
	val, err := client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", kerrors.WithKind(err, ErrNotFound{}, "Key not found")
		}
		return "", kerrors.WithKind(err, ErrClient{}, "Failed to get key")
	}
	return val, nil
}

func (s *service) GetInt(ctx context.Context, key string) (int64, error) {
	client, err := s.getClient(ctx)
	if err != nil {
		return 0, err
	}
	val, err := client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, kerrors.WithKind(err, ErrNotFound{}, "Key not found")
		}
		return 0, kerrors.WithKind(err, ErrClient{}, "Failed to get key")
	}
	num, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, kerrors.WithKind(err, ErrVal{}, "Invalid int value")
	}
	return num, nil
}

func (s *service) Set(ctx context.Context, key, val string, seconds int64) error {
	client, err := s.getClient(ctx)
	if err != nil {
		return err
	}
	if err := client.Set(ctx, key, val, time.Duration(seconds)*time.Second).Err(); err != nil {
		return kerrors.WithKind(err, ErrClient{}, "Failed to set key")
	}
	return nil
}

func (s *service) Del(ctx context.Context, key ...string) error {
	if len(key) == 0 {
		return nil
	}

	client, err := s.getClient(ctx)
	if err != nil {
		return err
	}

	if err := client.Del(ctx, key...).Err(); err != nil {
		return kerrors.WithKind(err, ErrClient{}, "Failed to delete key")
	}
	return nil
}

func (s *service) Incr(ctx context.Context, key string, delta int64) (int64, error) {
	client, err := s.getClient(ctx)
	if err != nil {
		return 0, err
	}
	val, err := client.IncrBy(ctx, key, delta).Result()
	if err != nil {
		return 0, kerrors.WithKind(err, ErrClient{}, "Failed to incr key")
	}
	return val, nil
}

func (s *service) Expire(ctx context.Context, key string, seconds int64) error {
	client, err := s.getClient(ctx)
	if err != nil {
		return err
	}
	if err := client.Expire(ctx, key, time.Duration(seconds)*time.Second).Err(); err != nil {
		return kerrors.WithKind(err, ErrClient{}, "Failed to set expire key")
	}
	return nil
}

func (s *service) Subkey(keypath ...string) string {
	if len(keypath) == 0 {
		return ""
	}
	return strings.Join(keypath, kvpathSeparator)
}

func (s *service) Multi(ctx context.Context) (Multi, error) {
	client, err := s.getClient(ctx)
	if err != nil {
		return nil, err
	}
	return &baseMulti{
		base: client.Pipeline(),
	}, nil
}

func (s *service) Tx(ctx context.Context) (Multi, error) {
	client, err := s.getClient(ctx)
	if err != nil {
		return nil, err
	}
	return &baseMulti{
		base: client.TxPipeline(),
	}, nil
}

func (s *service) Subtree(prefix string) KVStore {
	return &tree{
		prefix: prefix,
		base:   s,
	}
}

type (
	baseMulti struct {
		base redis.Pipeliner
	}

	multi struct {
		prefix string
		base   *baseMulti
	}
)

func (t *baseMulti) Get(ctx context.Context, key string) Resulter {
	return &resulter{
		res: t.base.Get(ctx, key),
	}
}

func (t *baseMulti) GetInt(ctx context.Context, key string) IntResulter {
	return &intResulter{
		res: t.base.Get(ctx, key),
	}
}

func (t *baseMulti) Set(ctx context.Context, key, val string, seconds int64) ErrResulter {
	return &statusCmdErrResulter{
		res: t.base.Set(ctx, key, val, time.Duration(seconds)*time.Second),
	}
}

func (t *baseMulti) Del(ctx context.Context, key ...string) ErrResulter {
	return &intCmdErrResulter{
		res: t.base.Del(ctx, key...),
	}
}

func (t *baseMulti) Incr(ctx context.Context, key string, delta int64) IntResulter {
	return &intCmdResulter{
		res: t.base.IncrBy(ctx, key, delta),
	}
}

func (t *baseMulti) Expire(ctx context.Context, key string, seconds int64) BoolResulter {
	return &boolCmdResulter{
		res: t.base.Expire(ctx, key, time.Duration(seconds)*time.Second),
	}
}

func (t *baseMulti) Subkey(keypath ...string) string {
	if len(keypath) == 0 {
		return ""
	}
	return strings.Join(keypath, kvpathSeparator)
}

func (t *baseMulti) Subtree(prefix string) Multi {
	return &multi{
		prefix: prefix,
		base:   t,
	}
}

func (t *baseMulti) Exec(ctx context.Context) error {
	if _, err := t.base.Exec(ctx); err != nil {
		if !errors.Is(err, redis.Nil) {
			return kerrors.WithKind(err, ErrNotFound{}, "Failed to execute multi")
		}
	}
	return nil
}

func (t *multi) Get(ctx context.Context, key string) Resulter {
	return t.base.Get(ctx, t.prefix+kvpathSeparator+key)
}

func (t *multi) GetInt(ctx context.Context, key string) IntResulter {
	return t.base.GetInt(ctx, t.prefix+kvpathSeparator+key)
}

func (t *multi) Set(ctx context.Context, key, val string, seconds int64) ErrResulter {
	return t.base.Set(ctx, t.prefix+kvpathSeparator+key, val, seconds)
}

func (t *multi) Del(ctx context.Context, key ...string) ErrResulter {
	args := make([]string, 0, len(key))
	for _, i := range key {
		args = append(args, t.prefix+kvpathSeparator+i)
	}
	return t.base.Del(ctx, args...)
}

func (t *multi) Incr(ctx context.Context, key string, delta int64) IntResulter {
	return t.base.Incr(ctx, t.prefix+kvpathSeparator+key, delta)
}

func (t *multi) Expire(ctx context.Context, key string, seconds int64) BoolResulter {
	return t.base.Expire(ctx, t.prefix+kvpathSeparator+key, seconds)
}

func (t *multi) Exec(ctx context.Context) error {
	return t.base.Exec(ctx)
}

func (t *multi) Subkey(keypath ...string) string {
	if len(keypath) == 0 {
		return ""
	}
	return strings.Join(keypath, kvpathSeparator)
}

func (t *multi) Subtree(prefix string) Multi {
	return &multi{
		prefix: t.prefix + kvpathSeparator + prefix,
		base:   t.base,
	}
}

type (
	tree struct {
		prefix string
		base   *service
	}
)

func (t *tree) Get(ctx context.Context, key string) (string, error) {
	return t.base.Get(ctx, t.prefix+kvpathSeparator+key)
}

func (t *tree) GetInt(ctx context.Context, key string) (int64, error) {
	return t.base.GetInt(ctx, t.prefix+kvpathSeparator+key)
}

func (t *tree) Set(ctx context.Context, key, val string, seconds int64) error {
	return t.base.Set(ctx, t.prefix+kvpathSeparator+key, val, seconds)
}

func (t *tree) Del(ctx context.Context, key ...string) error {
	args := make([]string, 0, len(key))
	for _, i := range key {
		args = append(args, t.prefix+kvpathSeparator+i)
	}
	return t.base.Del(ctx, args...)
}

func (t *tree) Incr(ctx context.Context, key string, delta int64) (int64, error) {
	return t.base.Incr(ctx, t.prefix+kvpathSeparator+key, delta)
}

func (t *tree) Expire(ctx context.Context, key string, seconds int64) error {
	return t.base.Expire(ctx, t.prefix+kvpathSeparator+key, seconds)
}

func (t *tree) Subkey(keypath ...string) string {
	if len(keypath) == 0 {
		return ""
	}
	return strings.Join(keypath, kvpathSeparator)
}

func (t *tree) Multi(ctx context.Context) (Multi, error) {
	tx, err := t.base.Multi(ctx)
	if err != nil {
		return nil, err
	}
	return tx.Subtree(t.prefix), nil
}

func (t *tree) Tx(ctx context.Context) (Multi, error) {
	tx, err := t.base.Tx(ctx)
	if err != nil {
		return nil, err
	}
	return tx.Subtree(t.prefix), nil
}

func (t *tree) Subtree(prefix string) KVStore {
	return &tree{
		prefix: t.prefix + kvpathSeparator + prefix,
		base:   t.base,
	}
}

type (
	resulter struct {
		res *redis.StringCmd
	}
)

func (r *resulter) Result() (string, error) {
	val, err := r.res.Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", kerrors.WithKind(err, ErrNotFound{}, "Key not found")
		}
		return "", kerrors.WithKind(err, ErrClient{}, "Failed to get key")
	}
	return val, nil
}

type (
	intCmdResulter struct {
		res *redis.IntCmd
	}
)

func (r *intCmdResulter) Result() (int64, error) {
	val, err := r.res.Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, kerrors.WithKind(err, ErrNotFound{}, "Key not found")
		}
		return 0, kerrors.WithKind(err, ErrClient{}, "Failed to get key")
	}
	return val, nil
}

type (
	intResulter struct {
		res *redis.StringCmd
	}
)

func (r *intResulter) Result() (int64, error) {
	val, err := r.res.Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, kerrors.WithKind(err, ErrNotFound{}, "Key not found")
		}
		return 0, kerrors.WithKind(err, ErrClient{}, "Failed to get key")
	}
	num, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, kerrors.WithKind(err, ErrVal{}, "Invalid int value")
	}
	return num, nil
}

type (
	boolCmdResulter struct {
		res *redis.BoolCmd
	}
)

func (r *boolCmdResulter) Result() (bool, error) {
	val, err := r.res.Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return false, kerrors.WithKind(err, ErrNotFound{}, "Key not found")
		}
		return false, kerrors.WithKind(err, ErrClient{}, "Failed to get key")
	}
	return val, nil
}

type (
	statusCmdErrResulter struct {
		res *redis.StatusCmd
	}
)

func (r *statusCmdErrResulter) Result() error {
	if err := r.res.Err(); err != nil {
		if errors.Is(err, redis.Nil) {
			return kerrors.WithKind(err, ErrNotFound{}, "Key not found")
		}
		return kerrors.WithKind(err, ErrClient{}, "Failed to get key")
	}
	return nil
}

type (
	intCmdErrResulter struct {
		res *redis.IntCmd
	}
)

func (r *intCmdErrResulter) Result() error {
	if err := r.res.Err(); err != nil {
		if errors.Is(err, redis.Nil) {
			return kerrors.WithKind(err, ErrNotFound{}, "Key not found")
		}
		return kerrors.WithKind(err, ErrClient{}, "Failed to get key")
	}
	return nil
}
