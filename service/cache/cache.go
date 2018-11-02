package cache

import (
	"fmt"
	"github.com/go-redis/redis"
	"github.com/hackform/governor"
	"github.com/labstack/echo"
	"net/http"
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

const (
	moduleID = "redis"
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
		l.Error(err.Error(), moduleID, "fail create cache", 0, nil)
		return nil, err
	}

	l.Info(fmt.Sprintf("cache: connected to %s:%s", rconf["host"], rconf["port"]), moduleID, "establish cache connection", 0, nil)
	l.Info("initialized cache", moduleID, "initialize cache service", 0, nil)

	return &redisCache{
		cache: cache,
	}, nil
}

// Mount is a place to mount routes to satisfy the Service interface
func (c *redisCache) Mount(conf governor.Config, l governor.Logger, r *echo.Group) error {
	l.Info("mounted cache", moduleID, "mount cache service", 0, nil)
	return nil
}

const (
	moduleIDHealth = moduleID + ".health"
)

// Health is a health check for the service
func (c *redisCache) Health() *governor.Error {
	if _, err := c.cache.Ping().Result(); err != nil {
		return governor.NewError(moduleIDHealth, err.Error(), 0, http.StatusServiceUnavailable)
	}
	return nil
}

// Setup is run on service setup
func (c *redisCache) Setup(conf governor.Config, l governor.Logger, rsetup governor.ReqSetupPost) *governor.Error {
	return nil
}

// Cache returns the cache instance
func (c *redisCache) Cache() *redis.Client {
	return c.cache
}
