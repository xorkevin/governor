package cacheconf

import (
	"github.com/hackform/governor"
)

// Conf loads in the defaults for the cache
func Conf(c *governor.Config) error {
	v := c.Conf()
	v.SetDefault("redis.password", "admin")
	v.SetDefault("redis.dbname", 0)
	v.SetDefault("redis.host", "localhost")
	v.SetDefault("redis.port", "6379")
	return nil
}
