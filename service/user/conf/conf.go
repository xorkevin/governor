package userconf

import (
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate/conf"
)

// Conf loads in the default
func Conf(c *governor.Config) error {
	v := c.Conf()
	v.SetDefault("user.confirm_duration", "1h")
	v.SetDefault("user.password_reset_duration", "1h")
	v.SetDefault("user.new_login_email", true)
	v.SetDefault("user.password_min_size", 8)
	return gateconf.Conf(c)
}
