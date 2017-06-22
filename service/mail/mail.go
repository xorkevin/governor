package mail

import (
	"github.com/hackform/governor"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"gopkg.in/gomail.v2"
)

type (
	// Mail is a service wrapper around a mailer instance
	Mail struct {
		mailer *gomail.Dialer
	}
)

const (
	moduleID = "mail"
)

// New creates a new mailer service
func New(c governor.Config) (*Mail, error) {
	v := c.Conf()
	rconf := v.GetStringMapString("mail")

	dialer := gomail.NewDialer(rconf["host"], v.GetInt("mail.port"), rconf["username"], rconf["password"])

	return &Mail{
		mailer: dialer,
	}, nil
}

// Mount is a place to mount routes to satisfy the Service interface
func (m *Mail) Mount(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
	return nil
}

const (
	moduleIDHealth = moduleID + ".Health"
)

// Health is a health check for the service
func (m *Mail) Health() *governor.Error {
	return nil
}

// Mailer returns the mailer instance
func (m *Mail) Mailer() *gomail.Dialer {
	return m.mailer
}
