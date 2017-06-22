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
	Cache struct {
		cache *redis.Client
	}
)

const (
	moduleID = "redis"
)

// New creates a new cache service
func New(c governor.Config) (*Cache, error) {
	v := c.Conf()
	rconf := v.GetStringMapString("redis")

	cache := redis.NewClient(&redis.Options{
		Addr:     rconf["host"] + ":" + rconf["port"],
		Password: rconf["password"],
		DB:       v.GetInt("redis.dbname"),
	})

	if _, err := cache.Ping().Result(); err != nil {
		return nil, err
	}

	return &Cache{
		cache: cache,
	}, nil
}

// Mount is a place to mount routes to satisfy the Service interface
func (c *Cache) Mount(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
	return nil
}

const (
	moduleIDHealth = moduleID + ".Health"
)

// Health is a health check for the service
func (c *Cache) Health() *governor.Error {
	if _, err := c.cache.Ping().Result(); err != nil {
		return governor.NewError(moduleIDHealth, err.Error(), 0, http.StatusServiceUnavailable)
	}
	return nil
}

// Cache returns the cache instance
func (c *Cache) Cache() *redis.Client {
	return c.cache
}
