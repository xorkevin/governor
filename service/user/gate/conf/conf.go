package gateconf

import (
	"xorkevin.dev/governor"
)

// Conf loads in the default
func Conf(c *governor.Config) error {
	v := c.Conf()
	v.SetDefault("userauth.duration", "5m")
	v.SetDefault("userauth.refresh_duration", "168h")
	v.SetDefault("userauth.secret", "governor")
	v.SetDefault("userauth.issuer", "governor")
	return nil
}
