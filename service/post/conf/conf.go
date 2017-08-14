package postconf

import (
	"github.com/hackform/governor"
)

// Conf loads in the defaults for the database
func Conf(c *governor.Config) error {
	v := c.Conf()
	v.SetDefault("post.archive_time", "2688h") // 4 months
	return nil
}
