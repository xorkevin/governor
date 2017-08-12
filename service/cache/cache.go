package cache

import (
	"github.com/go-redis/redis"
	"github.com/hackform/governor"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
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
func New(c governor.Config, l *logrus.Logger) (Cache, error) {
	v := c.Conf()
	rconf := v.GetStringMapString("redis")

	cache := redis.NewClient(&redis.Options{
		Addr:     rconf["host"] + ":" + rconf["port"],
		Password: rconf["password"],
		DB:       v.GetInt("redis.dbname"),
	})

	if _, err := cache.Ping().Result(); err != nil {
		l.Errorf("error creating Cache: %s\n", err)
		return nil, err
	}

	l.Info("initialized cache")

	return &redisCache{
		cache: cache,
	}, nil
}

// Mount is a place to mount routes to satisfy the Service interface
func (c *redisCache) Mount(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
	l.Info("mounted cache")
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
func (c *redisCache) Setup(conf governor.Config, l *logrus.Logger, rsetup governor.ReqSetupPost) *governor.Error {
	return nil
}

// Cache returns the cache instance
func (c *redisCache) Cache() *redis.Client {
	return c.cache
}
