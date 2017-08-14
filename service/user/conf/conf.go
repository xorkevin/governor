package userconf

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/gate/conf"
)

// Conf loads in the default
func Conf(c *governor.Config) error {
	v := c.Conf()
	v.SetDefault("user.confirm_duration", "24h")
	v.SetDefault("user.password_reset_duration", "24h")
	return gateconf.Conf(c)
}
