package kvstore

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/util/ksync"
	"xorkevin.dev/governor/util/lifecycle"
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
		Set(ctx context.Context, key, val string, duration time.Duration) ErrResulter
		SetNX(ctx context.Context, key, val string, duration time.Duration) BoolResulter
		Del(ctx context.Context, key ...string) ErrResulter
		Incr(ctx context.Context, key string, delta int64) IntResulter
		Expire(ctx context.Context, key string, duration time.Duration) BoolResulter
		Subkey(keypath ...string) string
		Subtree(prefix string) Multi
		Exec(ctx context.Context) error
	}

	// KVStore is a service wrapper around a kv store client
	KVStore interface {
		Ping(ctx context.Context) error
		Get(ctx context.Context, key string) (string, error)
		GetInt(ctx context.Context, key string) (int64, error)
		Set(ctx context.Context, key, val string, duration time.Duration) error
		SetNX(ctx context.Context, key, val string, duration time.Duration) (bool, error)
		Del(ctx context.Context, key ...string) error
		Incr(ctx context.Context, key string, delta int64) (int64, error)
		Expire(ctx context.Context, key string, duration time.Duration) error
		Subkey(keypath ...string) string
		Multi(ctx context.Context) (Multi, error)
		Subtree(prefix string) KVStore
	}

	kvstoreClient struct {
		client *redis.Client
		auth   redisauth
	}

	Service struct {
		lc        *lifecycle.Lifecycle[kvstoreClient]
		addr      string
		dbname    int
		config    governor.SecretReader
		log       *klog.LevelLogger
		hbfailed  int
		hbmaxfail int
		wg        *ksync.WaitGroup
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
		hbfailed: 0,
		wg:       ksync.NewWaitGroup(),
	}
}

func (s *Service) Register(inj governor.Injector, r governor.ConfigRegistrar) {
	setCtxRootKV(inj, s)

	r.SetDefault("auth", "")
	r.SetDefault("dbname", 0)
	r.SetDefault("host", "localhost")
	r.SetDefault("port", "6379")
	r.SetDefault("hbinterval", "5s")
	r.SetDefault("hbmaxfail", 5)
}

var (
	// ErrConn is returned on a kvstore connection error
	ErrConn errConn
	// ErrClient is returned for unknown client errors
	ErrClient errClient
	// ErrNotFound is returned when a key is not found
	ErrNotFound errNotFound
	// ErrVal is returned for invalid value errors
	ErrVal errVal
)

type (
	errConn     struct{}
	errClient   struct{}
	errNotFound struct{}
	errVal      struct{}
)

func (e errConn) Error() string {
	return "KVStore connection error"
}

func (e errClient) Error() string {
	return "KVStore client error"
}

func (e errNotFound) Error() string {
	return "Key not found"
}

func (e errVal) Error() string {
	return "Invalid value"
}

func (s *Service) Init(ctx context.Context, r governor.ConfigReader, log klog.Logger, m governor.Router) error {
	s.log = klog.NewLevelLogger(log)
	s.config = r

	s.addr = fmt.Sprintf("%s:%s", r.GetStr("host"), r.GetStr("port"))
	s.dbname = r.GetInt("dbname")
	hbinterval, err := r.GetDuration("hbinterval")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse hbinterval")
	}
	s.hbmaxfail = r.GetInt("hbmaxfail")

	s.log.Info(ctx, "Loaded config",
		klog.AString("addr", s.addr),
		klog.AString("dbname", strconv.Itoa(s.dbname)),
		klog.AString("hbinterval", hbinterval.String()),
		klog.AInt("hbmaxfail", s.hbmaxfail),
	)

	ctx = klog.CtxWithAttrs(ctx, klog.AString("gov.phase", "run"))

	s.lc = lifecycle.New(
		ctx,
		s.handleGetClient,
		s.closeClient,
		s.handlePing,
		hbinterval,
	)
	go s.lc.Heartbeat(ctx, s.wg)

	return nil
}

func (s *Service) handlePing(ctx context.Context, m *lifecycle.Manager[kvstoreClient]) {
	// Check client auth expiry, and reinit client if about to be expired
	client, err := m.Construct(ctx)
	if err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to create kvstore client"))
	}
	// Regardless of whether we were able to successfully retrieve a client, if
	// there is a client then ping the store. This allows vault to be temporarily
	// unavailable without disrupting the client connections.
	var username string
	if client != nil {
		_, err = client.client.Ping(ctx).Result()
		if err == nil {
			s.hbfailed = 0
			return
		}
		username = client.auth.Username
	}
	s.hbfailed++
	if s.hbfailed < s.hbmaxfail {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to ping kvstore"),
			klog.AString("addr", s.addr),
			klog.AString("username", username),
			klog.AString("dbname", strconv.Itoa(s.dbname)),
		)
		return
	}
	s.log.Err(ctx, kerrors.WithMsg(err, "Failed max pings to kvstore"),
		klog.AString("addr", s.addr),
		klog.AString("username", username),
		klog.AString("dbname", strconv.Itoa(s.dbname)),
	)

	s.hbfailed = 0
	// first invalidate cached secret in order to ensure that construct client
	// will use refreshed auth
	s.config.InvalidateSecret("auth")
	// must stop the client in order to invalidate cached client, and force wait
	// on newly constructed client
	m.Stop(ctx)
}

type (
	redisauth struct {
		Username string `mapstructure:"username"`
		Password string `mapstructure:"password"`
	}
)

func (s *Service) handleGetClient(ctx context.Context, m *lifecycle.Manager[kvstoreClient]) (*kvstoreClient, error) {
	var auth redisauth
	{
		client := m.Load(ctx)
		if err := s.config.GetSecret(ctx, "auth", 0, &auth); err != nil {
			return client, kerrors.WithMsg(err, "Invalid secret")
		}
		if auth.Username == "" {
			return client, kerrors.WithKind(nil, governor.ErrInvalidConfig, "Empty auth")
		}
		if client != nil && auth == client.auth {
			return client, nil
		}
	}

	kvClient := redis.NewClient(&redis.Options{
		Addr:     s.addr,
		Username: auth.Username,
		Password: auth.Password,
		DB:       s.dbname,
	})
	if _, err := kvClient.Ping(ctx).Result(); err != nil {
		if err := kvClient.Close(); err != nil {
			s.log.Err(ctx, kerrors.WithKind(err, ErrConn, "Failed to close db after failed initial ping"),
				klog.AString("addr", s.addr),
				klog.AString("username", auth.Username),
				klog.AString("dbname", strconv.Itoa(s.dbname)),
			)
		}
		s.config.InvalidateSecret("auth")
		return nil, kerrors.WithKind(err, ErrConn, "Failed to ping kvstore")
	}

	m.Stop(ctx)

	s.log.Info(ctx, "Established connection to kvstore",
		klog.AString("addr", s.addr),
		klog.AString("username", auth.Username),
		klog.AString("dbname", strconv.Itoa(s.dbname)),
	)

	client := &kvstoreClient{
		client: kvClient,
		auth:   auth,
	}
	m.Store(client)

	return client, nil
}

func (s *Service) closeClient(ctx context.Context, client *kvstoreClient) {
	if client != nil {
		if err := client.client.Close(); err != nil {
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to close kvstore connection"),
				klog.AString("addr", s.addr),
				klog.AString("username", client.auth.Username),
				klog.AString("dbname", strconv.Itoa(s.dbname)),
			)
		} else {
			s.log.Info(ctx, "Closed kvstore connection",
				klog.AString("addr", s.addr),
				klog.AString("username", client.auth.Username),
				klog.AString("dbname", strconv.Itoa(s.dbname)),
			)
		}
	}
}

func (s *Service) Start(ctx context.Context) error {
	return nil
}

func (s *Service) Stop(ctx context.Context) {
	if err := s.wg.Wait(ctx); err != nil {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to stop"))
	}
}

func (s *Service) Setup(ctx context.Context, req governor.ReqSetup) error {
	return nil
}

func (s *Service) Health(ctx context.Context) error {
	if s.lc.Load(ctx) == nil {
		return kerrors.WithKind(nil, ErrConn, "KVStore service not ready")
	}
	return nil
}

func (s *Service) getClient(ctx context.Context) (*redis.Client, error) {
	if client := s.lc.Load(ctx); client != nil {
		return client.client, nil
	}

	client, err := s.lc.Construct(ctx)
	if err != nil {
		// explicitly return nil in order to prevent usage of any cached client
		return nil, err
	}
	return client.client, nil
}

func (s *Service) Ping(ctx context.Context) error {
	client, err := s.getClient(ctx)
	if err != nil {
		return err
	}
	if _, err := client.Ping(ctx).Result(); err != nil {
		return kerrors.WithKind(err, ErrClient, "Failed to ping kvstore")
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
			return "", kerrors.WithKind(err, ErrNotFound, "Key not found")
		}
		return "", kerrors.WithKind(err, ErrClient, "Failed to get key")
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
			return 0, kerrors.WithKind(err, ErrNotFound, "Key not found")
		}
		return 0, kerrors.WithKind(err, ErrClient, "Failed to get key")
	}
	num, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, kerrors.WithKind(err, ErrVal, "Invalid int value")
	}
	return num, nil
}

func (s *Service) Set(ctx context.Context, key, val string, duration time.Duration) error {
	client, err := s.getClient(ctx)
	if err != nil {
		return err
	}
	if err := client.Set(ctx, key, val, duration).Err(); err != nil {
		return kerrors.WithKind(err, ErrClient, "Failed to set key")
	}
	return nil
}

func (s *Service) SetNX(ctx context.Context, key, val string, duration time.Duration) (bool, error) {
	client, err := s.getClient(ctx)
	if err != nil {
		return false, err
	}
	ok, err := client.SetNX(ctx, key, val, duration).Result()
	if err != nil {
		return false, kerrors.WithKind(err, ErrClient, "Failed to setnx key")
	}
	return ok, nil
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
		return kerrors.WithKind(err, ErrClient, "Failed to delete key")
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
		return 0, kerrors.WithKind(err, ErrClient, "Failed to incr key")
	}
	return val, nil
}

func (s *Service) Expire(ctx context.Context, key string, duration time.Duration) error {
	client, err := s.getClient(ctx)
	if err != nil {
		return err
	}
	if err := client.Expire(ctx, key, duration).Err(); err != nil {
		return kerrors.WithKind(err, ErrClient, "Failed to set expire key")
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

func (t *baseMulti) Set(ctx context.Context, key, val string, duration time.Duration) ErrResulter {
	return &statusCmdErrResulter{
		res: t.base.Set(ctx, key, val, duration),
	}
}

func (t *baseMulti) SetNX(ctx context.Context, key, val string, duration time.Duration) BoolResulter {
	return &boolCmdResulter{
		res: t.base.SetNX(ctx, key, val, duration),
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

func (t *baseMulti) Expire(ctx context.Context, key string, duration time.Duration) BoolResulter {
	return &boolCmdResulter{
		res: t.base.Expire(ctx, key, duration),
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
			return kerrors.WithKind(err, ErrNotFound, "Failed to execute multi")
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

func (t *multi) Set(ctx context.Context, key, val string, duration time.Duration) ErrResulter {
	return t.base.Set(ctx, t.prefix+kvpathSeparator+key, val, duration)
}

func (t *multi) SetNX(ctx context.Context, key, val string, duration time.Duration) BoolResulter {
	return t.base.SetNX(ctx, t.prefix+kvpathSeparator+key, val, duration)
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

func (t *multi) Expire(ctx context.Context, key string, duration time.Duration) BoolResulter {
	return t.base.Expire(ctx, t.prefix+kvpathSeparator+key, duration)
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

func (t *tree) Set(ctx context.Context, key, val string, duration time.Duration) error {
	return t.base.Set(ctx, t.prefix+kvpathSeparator+key, val, duration)
}

func (t *tree) SetNX(ctx context.Context, key, val string, duration time.Duration) (bool, error) {
	return t.base.SetNX(ctx, t.prefix+kvpathSeparator+key, val, duration)
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

func (t *tree) Expire(ctx context.Context, key string, duration time.Duration) error {
	return t.base.Expire(ctx, t.prefix+kvpathSeparator+key, duration)
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
			return "", kerrors.WithKind(err, ErrNotFound, "Key not found")
		}
		return "", kerrors.WithKind(err, ErrClient, "Failed to get key")
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
			return 0, kerrors.WithKind(err, ErrNotFound, "Key not found")
		}
		return 0, kerrors.WithKind(err, ErrClient, "Failed to get key")
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
			return 0, kerrors.WithKind(err, ErrNotFound, "Key not found")
		}
		return 0, kerrors.WithKind(err, ErrClient, "Failed to get key")
	}
	num, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, kerrors.WithKind(err, ErrVal, "Invalid int value")
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
			return false, kerrors.WithKind(err, ErrNotFound, "Key not found")
		}
		return false, kerrors.WithKind(err, ErrClient, "Failed to get key")
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
			return kerrors.WithKind(err, ErrNotFound, "Key not found")
		}
		return kerrors.WithKind(err, ErrClient, "Failed to get key")
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
			return kerrors.WithKind(err, ErrNotFound, "Key not found")
		}
		return kerrors.WithKind(err, ErrClient, "Failed to get key")
	}
	return nil
}
