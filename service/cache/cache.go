package cache

import (
	"fmt"
	"github.com/go-redis/redis"
	"github.com/labstack/echo"
	"net/http"
	"xorkevin.dev/governor"
)

type (
	// Cache is a service wrapper around a redis instance
	Cache interface {
		governor.Service
		Cache() *redis.Client
	}

	redisCache struct {
		cache *redis.Client
	}
)

// New creates a new cache service
func New(c governor.Config, l governor.Logger) (Cache, error) {
	v := c.Conf()
	rconf := v.GetStringMapString("redis")

	cache := redis.NewClient(&redis.Options{
		Addr:     rconf["host"] + ":" + rconf["port"],
		Password: rconf["password"],
		DB:       v.GetInt("redis.dbname"),
	})

	if _, err := cache.Ping().Result(); err != nil {
		l.Error("cache: fail create cache", map[string]string{
			"err": err.Error(),
		})
		return nil, err
	}

	l.Info(fmt.Sprintf("cache: establish connection to %s:%s", rconf["host"], rconf["port"]), nil)
	l.Info("initialize cache service", nil)

	return &redisCache{
		cache: cache,
	}, nil
}

// Mount is a place to mount routes to satisfy the Service interface
func (c *redisCache) Mount(conf governor.Config, l governor.Logger, r *echo.Group) error {
	l.Info("mount cache service", nil)
	return nil
}

// Health is a health check for the service
func (c *redisCache) Health() error {
	if _, err := c.cache.Ping().Result(); err != nil {
		return governor.NewError("Failed to connect to cache", http.StatusServiceUnavailable, err)
	}
	return nil
}

// Setup is run on service setup
func (c *redisCache) Setup(conf governor.Config, l governor.Logger, rsetup governor.ReqSetupPost) error {
	return nil
}

// Cache returns the cache instance
func (c *redisCache) Cache() *redis.Client {
	return c.cache
}
