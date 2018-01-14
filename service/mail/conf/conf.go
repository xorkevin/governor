package mailconf

import (
	"github.com/hackform/governor"
)

// Conf loads in the defaults for the mailer
func Conf(c *governor.Config) error {
	v := c.Conf()
	v.SetDefault("mail.username", "")
	v.SetDefault("mail.password", "")
	v.SetDefault("mail.host", "localhost")
	v.SetDefault("mail.port", "587")
	v.SetDefault("mail.insecure", false)
	v.SetDefault("mail.buffer_size", 1024)
	v.SetDefault("mail.worker_size", 2)
	v.SetDefault("mail.conn_msg_cap", 0)
	v.SetDefault("mail.from_address", "")
	return nil
}
