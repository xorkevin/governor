package objstoreconf

import (
	"github.com/hackform/governor"
)

// Conf loads in the defaults for the object store
func Conf(c *governor.Config) error {
	v := c.Conf()
	v.SetDefault("minio.keyID", "")
	v.SetDefault("minio.keySecret", "")
	v.SetDefault("minio.host", "localhost:9000")
	v.SetDefault("minio.sslmode", false)
	return nil
}
