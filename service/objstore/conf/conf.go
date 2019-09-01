package objstoreconf

import (
	"xorkevin.dev/governor"
)

// Conf loads in the defaults for the object store
func Conf(c *governor.Config) error {
	v := c.Conf()
	return nil
}
