package dbconf

import (
	"xorkevin.dev/governor"
)

// Conf loads in the defaults for the database
func Conf(c *governor.Config) error {
	v := c.Conf()
	v.SetDefault("postgres.user", "postgres")
	v.SetDefault("postgres.password", "admin")
	v.SetDefault("postgres.dbname", "governor")
	v.SetDefault("postgres.host", "localhost")
	v.SetDefault("postgres.port", "5432")
	v.SetDefault("postgres.sslmode", "disable")
	return nil
}
