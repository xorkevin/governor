package mail

import (
	"crypto/tls"
	"github.com/hackform/governor"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"gopkg.in/gomail.v2"
	"net/http"
	"time"
)

type (
	// Mail is a service wrapper around a mailer instance
	Mail interface {
		governor.Service
		Send(to, subject, body string) *governor.Error
	}

	goMail struct {
		host        string
		port        int
		username    string
		password    string
		insecure    bool
		bufferSize  int
		workerSize  int
		fromAddress string
		msgc        chan *gomail.Message
	}
)

const (
	moduleID = "mail"
)

// New creates a new mailer service
func New(c governor.Config, l *logrus.Logger) Mail {
	v := c.Conf()
	rconf := v.GetStringMapString("mail")

	l.Info("initialized mail service")

	return &goMail{
		host:        rconf["host"],
		port:        v.GetInt("mail.port"),
		username:    rconf["username"],
		password:    rconf["password"],
		insecure:    v.GetBool("mail.insecure"),
		bufferSize:  v.GetInt("mail.buffer_size"),
		workerSize:  v.GetInt("mail.worker_size"),
		fromAddress: rconf["from_address"],
		msgc:        make(chan *gomail.Message),
	}
}

func (m *goMail) dialer() *gomail.Dialer {
	d := gomail.NewDialer(m.host, m.port, m.username, m.password)

	if m.insecure {
		d.TLSConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	return d
}

func (m *goMail) mailWorker(l *logrus.Logger, ch <-chan *gomail.Message) {
	d := m.dialer()

	var s gomail.SendCloser
	var err error
	open := false
	for {
		select {
		case m, ok := <-ch:
			if !ok {
				return
			}
			if !open {
				if s, err = d.Dial(); err != nil {
					panic(err)
				}
				open = true
			}
			if err := gomail.Send(s, m); err != nil {
				l.Error(err)
			}
		// Close the connection to the SMTP server if no email was sent in
		// the last 30 seconds.
		case <-time.After(30 * time.Second):
			if open {
				if err := s.Close(); err != nil {
					panic(err)
				}
				open = false
			}
		}
	}
}

// Mount is a place to mount routes to satisfy the Service interface
func (m *goMail) Mount(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
	l.Info("mounted mail service")
	return nil
}

// Health is a health check for the service
func (m *goMail) Health() *governor.Error {
	return nil
}

// Setup is run on service setup
func (m *goMail) Setup(conf governor.Config, l *logrus.Logger, rsetup governor.ReqSetupPost) *governor.Error {
	return nil
}

const (
	moduleIDSend = moduleID + ".send"
)

// Send creates and sends a new message
func (m *goMail) Send(to, subject, body string) *governor.Error {
	msg := gomail.NewMessage()
	msg.SetHeader("From", m.fromAddress)
	msg.SetHeader("To", to)
	msg.SetHeader("Subject", subject)
	msg.SetBody("text/html", body)

	//if err := m.mailer.DialAndSend(msg); err != nil {
	//	return governor.NewError(moduleIDSend, err.Error(), 0, http.StatusInternalServerError)
	//}

	return nil
}
