package kvstore

import (
	"context"
	"fmt"
	"github.com/go-redis/redis"
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
		Tx() Tx
		Subtree(prefix string) KVStore
	}

	Service interface {
		governor.Service
		KVStore
	}

	service struct {
		client *redis.Client
		logger governor.Logger
	}
)

// New creates a new cache service
func New() Service {
	return &service{}
}

func (s *service) Register(r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	r.SetDefault("password", "admin")
	r.SetDefault("dbname", 0)
	r.SetDefault("host", "localhost")
	r.SetDefault("port", "6379")
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, g *echo.Group) error {
	s.logger = l
	l = s.logger.WithData(map[string]string{
		"phase": "init",
	})

	client := redis.NewClient(&redis.Options{
		Addr:     r.GetStr("host") + ":" + r.GetStr("port"),
		Password: r.GetStr("password"),
		DB:       r.GetInt("dbname"),
	})
	s.client = client

	if _, err := client.Ping().Result(); err != nil {
		return governor.NewError("Failed to ping kvstore", http.StatusInternalServerError, err)
	}

	l.Info(fmt.Sprintf("established connection to %s:%s", r.GetStr("host"), r.GetStr("port")), nil)
	return nil
}

func (s *service) Setup(req governor.ReqSetup) error {
	return nil
}

func (s *service) Start(ctx context.Context) error {
	return nil
}

func (s *service) Stop(ctx context.Context) {
}

func (s *service) Health() error {
	if _, err := s.client.Ping().Result(); err != nil {
		return governor.NewError("Failed to connect to kvstore", http.StatusInternalServerError, err)
	}
	return nil
}

func (s *service) Get(key string) (string, error) {
	val, err := s.client.Get(key).Result()
	if err != nil {
		if err == redis.Nil {
			return "", governor.NewError("Key not found", http.StatusNotFound, err)
		}
		return "", governor.NewError("Failed to get key", http.StatusInternalServerError, err)
	}
	return val, nil
}

func (s *service) Set(key, val string, seconds int64) error {
	if err := s.client.Set(key, val, time.Duration(seconds)*time.Second).Err(); err != nil {
		return governor.NewError("Failed to set key", http.StatusInternalServerError, err)
	}
	return nil
}

func (s *service) Del(key ...string) error {
	if len(key) == 0 {
		return nil
	}

	if err := s.client.Del(key...).Err(); err != nil {
		return governor.NewError("Failed to delete key", http.StatusInternalServerError, err)
	}
	return nil
}

func (s *service) Tx() Tx {
	return &baseTransaction{
		base: s.client.TxPipeline(),
	}
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

func (t *tree) Tx() Tx {
	return t.base.Tx().Subtree(t.prefix)
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
