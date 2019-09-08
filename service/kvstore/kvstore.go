package kvstore

import (
	"context"
	"fmt"
	"github.com/go-redis/redis"
	"github.com/labstack/echo"
	"net/http"
	"xorkevin.dev/governor"
)

type (
	// KVStore is a service wrapper around a kv store client
	KVStore interface {
		KVStore() *redis.Client
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
	conf := r.GetStrMap("")

	client := redis.NewClient(&redis.Options{
		Addr:     conf["host"] + ":" + conf["port"],
		Password: conf["password"],
		DB:       r.GetInt("dbname"),
	})
	s.client = client

	if _, err := client.Ping().Result(); err != nil {
		s.logger.Error("kvstore: failed to ping kvstore", map[string]string{
			"error": err.Error(),
		})
		return governor.NewError("Failed to ping kvstore", http.StatusInternalServerError, err)
	}

	s.logger.Info(fmt.Sprintf("kvstore: established connection to %s:%s", conf["host"], conf["port"]), nil)
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

// KVStore returns the client instance
func (s *service) KVStore() *redis.Client {
	return s.client
}
