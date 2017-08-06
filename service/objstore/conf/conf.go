package objstoreconf

import (
	"github.com/hackform/governor"
)

// Conf loads in the defaults for the object store
func Conf(c *governor.Config) error {
	v := c.Conf()
	v.SetDefault("minio.key_id", "admin")
	v.SetDefault("minio.key_secret", "adminsecret")
	v.SetDefault("minio.host", "localhost")
	v.SetDefault("minio.port", "9000")
	v.SetDefault("minio.sslmode", false)
	return nil
}
