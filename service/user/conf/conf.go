package userconf

import (
	"github.com/hackform/governor"
)

// Conf loads in the default
func Conf(c *governor.Config) error {
	v := c.Conf()
	v.SetDefault("userauth.duration", "5m")
	v.SetDefault("userauth.refresh_duration", "168h")
	v.SetDefault("userauth.secret", "governor")
	v.SetDefault("userauth.issuer", "governor")
	v.SetDefault("user.confirm_duration", "24h")
	v.SetDefault("user.password_reset_duration", "24h")
	return nil
}
