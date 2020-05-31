package kvstore

import (
	"context"
	"fmt"
	"github.com/go-redis/redis/v7"
	"github.com/labstack/echo/v4"
	"net/http"
	"time"
	"xorkevin.dev/governor"
)

type (
	Resulter interface {
		Result() (string, error)
	}

	Tx interface {
		Get(key string) Resulter
		Set(key, val string, seconds int64)
		Del(key ...string)
		Subtree(prefix string) Tx
		Exec() error
	}

	// KVStore is a service wrapper around a kv store client
	KVStore interface {
		Get(key string) (string, error)
		Set(key, val string, seconds int64) error
		Del(key ...string) error
		Tx() (Tx, error)
		Subtree(prefix string) KVStore
	}

	Service interface {
		governor.Service
		KVStore
	}

	getClientRes struct {
		client *redis.Client
		err    error
	}

	getOp struct {
		res chan<- getClientRes
	}

	pingOp struct {
		res chan<- error
	}

	service struct {
		client *redis.Client
		auth   string
		addr   string
		dbname int
		config governor.SecretReader
		logger governor.Logger
		ops    chan getOp
		pings  chan pingOp
		done   <-chan struct{}
	}
)

// New creates a new cache service
func New() Service {
	return &service{
		ops:   make(chan getOp),
		pings: make(chan pingOp),
	}
}

func (s *service) Register(r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	r.SetDefault("auth", "")
	r.SetDefault("dbname", 0)
	r.SetDefault("host", "localhost")
	r.SetDefault("port", "6379")
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, g *echo.Group) error {
	s.logger = l
	l = s.logger.WithData(map[string]string{
		"phase": "init",
	})

	s.config = r

	conf := r.GetStrMap("")
	s.addr = fmt.Sprintf("%s:%s", conf["host"], conf["port"])
	s.dbname = r.GetInt("dbname")

	done := make(chan struct{})
	go s.execute(ctx, done)
	s.done = done

	if _, err := s.getClient(); err != nil {
		return err
	}
	return nil
}

func (s *service) execute(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	for {
		select {
		case <-ctx.Done():
			s.closeClient()
			return
		case op := <-s.pings:
			op.res <- s.handlePing()
			close(op.res)
		case op := <-s.ops:
			client, err := s.handleGetClient()
			op.res <- getClientRes{
				client: client,
				err:    err,
			}
			close(op.res)
		}
	}
}

func (s *service) handlePing() error {
	if s.client == nil {
		return governor.NewError("No kvstore connection", http.StatusInternalServerError, nil)
	}
	if _, err := s.client.Ping().Result(); err != nil {
		s.config.InvalidateSecret("auth")
		return governor.NewError("Failed to ping kvstore", http.StatusInternalServerError, err)
	}
	return nil
}

func (s *service) handleGetClient() (*redis.Client, error) {
	authsecret, err := s.config.GetSecret("auth")
	if err != nil {
		return nil, err
	}
	auth := authsecret["password"].(string)
	if auth == s.auth {
		return s.client, nil
	}

	s.closeClient()

	client := redis.NewClient(&redis.Options{
		Addr:     s.addr,
		Password: auth,
		DB:       s.dbname,
	})
	if _, err := client.Ping().Result(); err != nil {
		s.config.InvalidateSecret("auth")
		return nil, governor.NewError("Failed to ping kvstore", http.StatusInternalServerError, err)
	}

	s.client = client
	s.auth = auth
	s.logger.Info(fmt.Sprintf("established connection to %s", s.addr), nil)
	return s.client, nil
}

func (s *service) closeClient() {
	if s.client == nil {
		return
	}
	if err := s.client.Close(); err != nil {
		s.logger.Error("failed to close kvstore connection", map[string]string{
			"error":      err.Error(),
			"actiontype": "closekverr",
			"address":    s.addr,
			"dbname":     fmt.Sprintf("%d", s.dbname),
		})
	} else {
		s.logger.Info("closed kvstore connection", map[string]string{
			"actiontype": "closekvok",
			"address":    s.addr,
			"dbname":     fmt.Sprintf("%d", s.dbname),
		})
	}
	s.client = nil
	s.auth = ""
}

func (s *service) Setup(req governor.ReqSetup) error {
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
		l.Warn("failed to stop", nil)
	}
}

func (s *service) Health() error {
	res := make(chan error)
	op := pingOp{
		res: res,
	}
	select {
	case <-s.done:
		return governor.NewError("KVStore service shutdown", http.StatusInternalServerError, nil)
	case s.pings <- op:
		return <-res
	}
}

func (s *service) getClient() (*redis.Client, error) {
	res := make(chan getClientRes)
	op := getOp{
		res: res,
	}
	select {
	case <-s.done:
		return nil, governor.NewError("KVStore service shutdown", http.StatusInternalServerError, nil)
	case s.ops <- op:
		v := <-res
		return v.client, v.err
	}
}

func (s *service) Get(key string) (string, error) {
	client, err := s.getClient()
	if err != nil {
		return "", err
	}
	val, err := client.Get(key).Result()
	if err != nil {
		if err == redis.Nil {
			return "", governor.NewError("Key not found", http.StatusNotFound, err)
		}
		return "", governor.NewError("Failed to get key", http.StatusInternalServerError, err)
	}
	return val, nil
}

func (s *service) Set(key, val string, seconds int64) error {
	client, err := s.getClient()
	if err != nil {
		return err
	}
	if err := client.Set(key, val, time.Duration(seconds)*time.Second).Err(); err != nil {
		return governor.NewError("Failed to set key", http.StatusInternalServerError, err)
	}
	return nil
}

func (s *service) Del(key ...string) error {
	if len(key) == 0 {
		return nil
	}

	client, err := s.getClient()
	if err != nil {
		return err
	}

	if err := client.Del(key...).Err(); err != nil {
		return governor.NewError("Failed to delete key", http.StatusInternalServerError, err)
	}
	return nil
}

func (s *service) Tx() (Tx, error) {
	client, err := s.getClient()
	if err != nil {
		return nil, err
	}
	return &baseTransaction{
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
	baseTransaction struct {
		base redis.Pipeliner
	}

	transaction struct {
		prefix string
		base   *baseTransaction
	}
)

func (t *baseTransaction) Get(key string) Resulter {
	return &resulter{
		res: t.base.Get(key),
	}
}

func (t *baseTransaction) Set(key, val string, seconds int64) {
	t.base.Set(key, val, time.Duration(seconds)*time.Second)
}

func (t *baseTransaction) Del(key ...string) {
	t.base.Del(key...)
}

func (t *baseTransaction) Subtree(prefix string) Tx {
	return &transaction{
		prefix: prefix,
		base:   t,
	}
}

func (t *baseTransaction) Exec() error {
	if _, err := t.base.Exec(); err != nil {
		if err != redis.Nil {
			return governor.NewError("Failed to execute transaction", http.StatusInternalServerError, err)
		}
	}
	return nil
}

func (t *transaction) Get(key string) Resulter {
	return t.base.Get(t.prefix + ":" + key)
}

func (t *transaction) Set(key, val string, seconds int64) {
	t.base.Set(t.prefix+":"+key, val, seconds)
}

func (t *transaction) Del(key ...string) {
	args := make([]string, 0, len(key))
	for _, i := range key {
		args = append(args, t.prefix+":"+i)
	}
	t.base.Del(args...)
}

func (t *transaction) Exec() error {
	return t.base.Exec()
}

func (t *transaction) Subtree(prefix string) Tx {
	return &transaction{
		prefix: t.prefix + ":" + prefix,
		base:   t.base,
	}
}

type (
	tree struct {
		prefix string
		base   *service
	}
)

func (t *tree) Get(key string) (string, error) {
	return t.base.Get(t.prefix + ":" + key)
}

func (t *tree) Set(key, val string, seconds int64) error {
	return t.base.Set(t.prefix+":"+key, val, seconds)
}

func (t *tree) Del(key ...string) error {
	args := make([]string, 0, len(key))
	for _, i := range key {
		args = append(args, t.prefix+":"+i)
	}
	return t.base.Del(args...)
}

func (t *tree) Tx() (Tx, error) {
	tx, err := t.base.Tx()
	if err != nil {
		return nil, err
	}
	return tx.Subtree(t.prefix), nil
}

func (t *tree) Subtree(prefix string) KVStore {
	return &tree{
		prefix: t.prefix + ":" + prefix,
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
		if err == redis.Nil {
			return "", governor.NewError("Key not found", http.StatusNotFound, err)
		}
		return "", governor.NewError("Failed to get key", http.StatusInternalServerError, err)
	}
	return val, nil
}
