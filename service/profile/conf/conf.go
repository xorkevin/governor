package profileconf

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/conf"
)

// Conf loads in the default
func Conf(c *governor.Config) error {
	return userconf.Conf(c)
}
