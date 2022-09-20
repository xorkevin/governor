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
	"xorkevin.dev/klog"
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
		Ping(ctx context.Context) error
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

	getClientRes struct {
		client *redis.Client
		err    error
	}

	getOp struct {
		ctx context.Context
		res chan<- getClientRes
	}

	Service struct {
		client     *redis.Client
		aclient    *atomic.Pointer[redis.Client]
		auth       secretAuth
		addr       string
		dbname     int
		config     governor.SecretReader
		log        *klog.LevelLogger
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
func New() *Service {
	return &Service{
		aclient:  &atomic.Pointer[redis.Client]{},
		ops:      make(chan getOp),
		ready:    &atomic.Bool{},
		hbfailed: 0,
	}
}

func (s *Service) Register(name string, inj governor.Injector, r governor.ConfigRegistrar) {
	setCtxRootKV(inj, s)

	r.SetDefault("auth", "")
	r.SetDefault("dbname", 0)
	r.SetDefault("host", "localhost")
	r.SetDefault("port", "6379")
	r.SetDefault("hbinterval", 5)
	r.SetDefault("hbmaxfail", 5)
}

type (
	// ErrorConn is returned on a kvstore connection error
	ErrorConn struct{}
	// ErrorClient is returned for unknown client errors
	ErrorClient struct{}
	// ErrorNotFound is returned when a key is not found
	ErrorNotFound struct{}
	// ErrorVal is returned for invalid value errors
	ErrorVal struct{}
)

func (e ErrorConn) Error() string {
	return "KVStore connection error"
}

func (e ErrorClient) Error() string {
	return "KVStore client error"
}

func (e ErrorNotFound) Error() string {
	return "Key not found"
}

func (e ErrorVal) Error() string {
	return "Invalid value"
}

func (s *Service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, log klog.Logger, m governor.Router) error {
	s.log = klog.NewLevelLogger(log)
	s.config = r

	s.addr = fmt.Sprintf("%s:%s", r.GetStr("host"), r.GetStr("port"))
	s.dbname = r.GetInt("dbname")
	s.hbinterval = r.GetInt("hbinterval")
	s.hbmaxfail = r.GetInt("hbmaxfail")

	s.log.Info(ctx, "Loaded config", klog.Fields{
		"kv.addr":       s.addr,
		"kv.dbname":     strconv.Itoa(s.dbname),
		"kv.hbinterval": s.hbinterval,
		"kv.hbmaxfail":  s.hbmaxfail,
	})

	ctx = klog.WithFields(ctx, klog.Fields{
		"gov.service.phase": "run",
	})

	done := make(chan struct{})
	go s.execute(ctx, done)
	s.done = done

	return nil
}

func (s *Service) execute(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(time.Duration(s.hbinterval) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.closeClient(klog.ExtendCtx(context.Background(), ctx, nil))
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

func (s *Service) handlePing(ctx context.Context) {
	var err error
	// Check client auth expiry, and reinit client if about to be expired
	if _, err = s.handleGetClient(ctx); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to create kvstore client"), nil)
	}
	// Regardless of whether we were able to successfully retrieve a client, if
	// there is a client then ping the store. This allows vault to be temporarily
	// unavailable without disrupting the client connections.
	if s.client != nil {
		_, err = s.client.Ping(ctx).Result()
		if err == nil {
			s.ready.Store(true)
			s.hbfailed = 0
			return
		}
	}
	s.hbfailed++
	if s.hbfailed < s.hbmaxfail {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to ping kvstore"), klog.Fields{
			"kv.addr":   s.addr,
			"kv.dbname": strconv.Itoa(s.dbname),
		})
		return
	}
	s.log.Err(ctx, kerrors.WithMsg(err, "Failed max pings to kvstore"), klog.Fields{
		"kv.addr":   s.addr,
		"kv.dbname": strconv.Itoa(s.dbname),
	})
	s.aclient.Store(nil)
	s.ready.Store(false)
	s.hbfailed = 0
	s.auth = secretAuth{}
	s.config.InvalidateSecret("auth")
}

type (
	secretAuth struct {
		Password string `mapstructure:"password"`
	}
)

func (s *Service) handleGetClient(ctx context.Context) (*redis.Client, error) {
	var secret secretAuth
	if err := s.config.GetSecret(ctx, "auth", 0, &secret); err != nil {
		return nil, kerrors.WithMsg(err, "Invalid secret")
	}
	if secret.Password == "" {
		return nil, kerrors.WithKind(nil, governor.ErrorInvalidConfig{}, "Empty auth")
	}
	if secret == s.auth {
		return s.client, nil
	}

	s.closeClient(klog.ExtendCtx(context.Background(), ctx, nil))

	client := redis.NewClient(&redis.Options{
		Addr:     s.addr,
		Password: secret.Password,
		DB:       s.dbname,
	})
	if _, err := client.Ping(ctx).Result(); err != nil {
		s.config.InvalidateSecret("auth")
		return nil, kerrors.WithKind(err, ErrorConn{}, "Failed to ping kvstore")
	}

	s.client = client
	s.aclient.Store(s.client)
	s.auth = secret
	s.ready.Store(true)
	s.hbfailed = 0
	s.log.Info(ctx, "Established connection to kvstore", klog.Fields{
		"kv.addr":   s.addr,
		"kv.dbname": strconv.Itoa(s.dbname),
	})
	return s.client, nil
}

func (s *Service) closeClient(ctx context.Context) {
	if s.client == nil {
		return
	}
	s.aclient.Store(nil)
	if err := s.client.Close(); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to close kvstore connection"), klog.Fields{
			"kv.addr":   s.addr,
			"kv.dbname": strconv.Itoa(s.dbname),
		})
	} else {
		s.log.Info(ctx, "Closed kvstore connection", klog.Fields{
			"kv.addr":   s.addr,
			"kv.dbname": strconv.Itoa(s.dbname),
		})
	}
	s.client = nil
	s.auth = secretAuth{}
}

func (s *Service) Start(ctx context.Context) error {
	return nil
}

func (s *Service) Stop(ctx context.Context) {
	select {
	case <-s.done:
		return
	case <-ctx.Done():
		s.log.WarnErr(ctx, kerrors.WithMsg(ctx.Err(), "Failed to stop"), nil)
	}
}

func (s *Service) Setup(ctx context.Context, req governor.ReqSetup) error {
	return nil
}

func (s *Service) Health(ctx context.Context) error {
	if !s.ready.Load() {
		return kerrors.WithKind(nil, ErrorConn{}, "KVStore service not ready")
	}
	return nil
}

func (s *Service) getClient(ctx context.Context) (*redis.Client, error) {
	if client := s.aclient.Load(); client != nil {
		return client, nil
	}

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

func (s *Service) Ping(ctx context.Context) error {
	client, err := s.getClient(ctx)
	if err != nil {
		return err
	}
	if _, err := client.Ping(ctx).Result(); err != nil {
		return kerrors.WithKind(err, ErrorClient{}, "Failed to ping kvstore")
	}
	return nil
}

func (s *Service) Get(ctx context.Context, key string) (string, error) {
	client, err := s.getClient(ctx)
	if err != nil {
		return "", err
	}
	val, err := client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", kerrors.WithKind(err, ErrorNotFound{}, "Key not found")
		}
		return "", kerrors.WithKind(err, ErrorClient{}, "Failed to get key")
	}
	return val, nil
}

func (s *Service) GetInt(ctx context.Context, key string) (int64, error) {
	client, err := s.getClient(ctx)
	if err != nil {
		return 0, err
	}
	val, err := client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, kerrors.WithKind(err, ErrorNotFound{}, "Key not found")
		}
		return 0, kerrors.WithKind(err, ErrorClient{}, "Failed to get key")
	}
	num, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, kerrors.WithKind(err, ErrorVal{}, "Invalid int value")
	}
	return num, nil
}

func (s *Service) Set(ctx context.Context, key, val string, seconds int64) error {
	client, err := s.getClient(ctx)
	if err != nil {
		return err
	}
	if err := client.Set(ctx, key, val, time.Duration(seconds)*time.Second).Err(); err != nil {
		return kerrors.WithKind(err, ErrorClient{}, "Failed to set key")
	}
	return nil
}

func (s *Service) Del(ctx context.Context, key ...string) error {
	if len(key) == 0 {
		return nil
	}

	client, err := s.getClient(ctx)
	if err != nil {
		return err
	}

	if err := client.Del(ctx, key...).Err(); err != nil {
		return kerrors.WithKind(err, ErrorClient{}, "Failed to delete key")
	}
	return nil
}

func (s *Service) Incr(ctx context.Context, key string, delta int64) (int64, error) {
	client, err := s.getClient(ctx)
	if err != nil {
		return 0, err
	}
	val, err := client.IncrBy(ctx, key, delta).Result()
	if err != nil {
		return 0, kerrors.WithKind(err, ErrorClient{}, "Failed to incr key")
	}
	return val, nil
}

func (s *Service) Expire(ctx context.Context, key string, seconds int64) error {
	client, err := s.getClient(ctx)
	if err != nil {
		return err
	}
	if err := client.Expire(ctx, key, time.Duration(seconds)*time.Second).Err(); err != nil {
		return kerrors.WithKind(err, ErrorClient{}, "Failed to set expire key")
	}
	return nil
}

func (s *Service) Subkey(keypath ...string) string {
	if len(keypath) == 0 {
		return ""
	}
	return strings.Join(keypath, kvpathSeparator)
}

func (s *Service) Multi(ctx context.Context) (Multi, error) {
	client, err := s.getClient(ctx)
	if err != nil {
		return nil, err
	}
	return &baseMulti{
		base: client.Pipeline(),
	}, nil
}

func (s *Service) Tx(ctx context.Context) (Multi, error) {
	client, err := s.getClient(ctx)
	if err != nil {
		return nil, err
	}
	return &baseMulti{
		base: client.TxPipeline(),
	}, nil
}

func (s *Service) Subtree(prefix string) KVStore {
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
			return kerrors.WithKind(err, ErrorNotFound{}, "Failed to execute multi")
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
		base   *Service
	}
)

func (t *tree) Ping(ctx context.Context) error {
	return t.base.Ping(ctx)
}

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
			return "", kerrors.WithKind(err, ErrorNotFound{}, "Key not found")
		}
		return "", kerrors.WithKind(err, ErrorClient{}, "Failed to get key")
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
			return 0, kerrors.WithKind(err, ErrorNotFound{}, "Key not found")
		}
		return 0, kerrors.WithKind(err, ErrorClient{}, "Failed to get key")
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
			return 0, kerrors.WithKind(err, ErrorNotFound{}, "Key not found")
		}
		return 0, kerrors.WithKind(err, ErrorClient{}, "Failed to get key")
	}
	num, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, kerrors.WithKind(err, ErrorVal{}, "Invalid int value")
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
			return false, kerrors.WithKind(err, ErrorNotFound{}, "Key not found")
		}
		return false, kerrors.WithKind(err, ErrorClient{}, "Failed to get key")
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
			return kerrors.WithKind(err, ErrorNotFound{}, "Key not found")
		}
		return kerrors.WithKind(err, ErrorClient{}, "Failed to get key")
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
			return kerrors.WithKind(err, ErrorNotFound{}, "Key not found")
		}
		return kerrors.WithKind(err, ErrorClient{}, "Failed to get key")
	}
	return nil
}
