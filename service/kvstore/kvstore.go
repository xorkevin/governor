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
	// KVStore is a service wrapper around a kv store client
	KVStore interface {
		Get(key string) (string, error)
		Set(key, val string, seconds int64) error
		Del(key ...string) error
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

func (s *service) Register(r governor.ConfigRegistrar) {
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
		l.Error("failed to ping kvstore", map[string]string{
			"error": err.Error(),
		})
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
	if err := s.client.Del(key...).Err(); err != nil {
		return governor.NewError("Failed to delete key", http.StatusInternalServerError, err)
	}
	return nil
}

type (
	tree struct {
		prefix string
		base   KVStore
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

func (t *tree) Subtree(prefix string) KVStore {
	return &tree{
		prefix: prefix,
		base:   t,
	}
}

func (s *service) Subtree(prefix string) KVStore {
	return &tree{
		prefix: prefix,
		base:   s,
	}
}
