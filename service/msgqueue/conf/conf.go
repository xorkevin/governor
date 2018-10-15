package msgqueueconf

import (
	"github.com/hackform/governor"
)

// Conf loads in the defaults for the msgqueue
func Conf(c *governor.Config) error {
	v := c.Conf()
	v.SetDefault("nats.host", "localhost")
	v.SetDefault("nats.port", "4222")
	v.SetDefault("nats.cluster", "nss")
	v.SetDefault("nats.clientid", "governor")
	return nil
}
