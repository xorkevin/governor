package mail

import (
	"crypto/tls"
	"github.com/hackform/governor"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"gopkg.in/gomail.v2"
	"net/http"
)

type (
	// Mail is a service wrapper around a mailer instance
	Mail struct {
		mailer      *gomail.Dialer
		fromAddress string
	}
)

const (
	moduleID = "mail"
)

// New creates a new mailer service
func New(c governor.Config, l *logrus.Logger) *Mail {
	v := c.Conf()
	rconf := v.GetStringMapString("mail")

	m := gomail.NewDialer(rconf["host"], v.GetInt("mail.port"), rconf["username"], rconf["password"])

	if v.GetBool("mail.insecure") {
		m.TLSConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	l.Info("initialized mail service")

	return &Mail{
		mailer:      m,
		fromAddress: rconf["from_address"],
	}
}

// Mount is a place to mount routes to satisfy the Service interface
func (m *Mail) Mount(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
	l.Info("mounted mail service")
	return nil
}

// Health is a health check for the service
func (m *Mail) Health() *governor.Error {
	return nil
}

const (
	moduleIDSend = moduleID + ".send"
)

// Send creates and sends a new message
func (m *Mail) Send(to, subject, body string) *governor.Error {
	msg := gomail.NewMessage()
	msg.SetHeader("From", m.fromAddress)
	msg.SetHeader("To", to)
	msg.SetHeader("Subject", subject)
	msg.SetBody("text/html", body)

	if err := m.mailer.DialAndSend(msg); err != nil {
		return governor.NewError(moduleIDSend, err.Error(), 0, http.StatusInternalServerError)
	}

	return nil
}
