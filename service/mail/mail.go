package mail

import (
	"crypto/tls"
	gomail "github.com/go-mail/mail"
	"github.com/hackform/governor"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
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
		connMsgCap  int
		fromAddress string
		fromName    string
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

	l.Infof("mail: smtp server: %s:%s", rconf["host"], rconf["port"])
	l.Infof("mail: buffer_size: %d", v.GetInt("mail.buffer_size"))
	l.Infof("mail: worker_size: %d", v.GetInt("mail.worker_size"))
	l.Infof("mail: conn_msg_cap: %d", v.GetInt("mail.conn_msg_cap"))
	l.Infof("mail: from: %s <%s>", rconf["from_name"], rconf["from_address"])
	l.Info("initialized mail service")

	gm := &goMail{
		host:        rconf["host"],
		port:        v.GetInt("mail.port"),
		username:    rconf["username"],
		password:    rconf["password"],
		insecure:    v.GetBool("mail.insecure"),
		bufferSize:  v.GetInt("mail.buffer_size"),
		workerSize:  v.GetInt("mail.worker_size"),
		connMsgCap:  v.GetInt("mail.conn_msg_cap"),
		fromAddress: rconf["from_address"],
		fromName:    rconf["from_name"],
		msgc:        make(chan *gomail.Message, v.GetInt("mail.buffer_size")),
	}

	gm.startWorkers(l)

	return gm
}

func (m *goMail) dialer() *gomail.Dialer {
	d := gomail.NewDialer(m.host, m.port, m.username, m.password)

	if m.insecure {
		d.TLSConfig = &tls.Config{
			ServerName:         m.host,
			InsecureSkipVerify: true,
		}
	}

	return d
}

func (m *goMail) mailWorker(l *logrus.Logger) {
	cap := m.connMsgCap
	d := m.dialer()
	var sender gomail.SendCloser
	mailSent := 0

	for {
		select {
		case m, ok := <-m.msgc:
			if !ok {
				return
			}
			if sender == nil || mailSent >= cap && cap > 0 {
				if s, err := d.Dial(); err == nil {
					sender = s
					mailSent = 0
				} else {
					l.Errorf("Failed to dial smtp server: %s", err)
				}
			}
			if sender != nil {
				if err := gomail.Send(sender, m); err != nil {
					l.Error(err)
				}
				mailSent++
			}

		case <-time.After(30 * time.Second):
			if sender != nil {
				if err := sender.Close(); err != nil {
					l.Error(err)
				}
				sender = nil
			}
		}
	}
}

func (m *goMail) startWorkers(l *logrus.Logger) {
	for i := 0; i < m.workerSize; i++ {
		go m.mailWorker(l)
	}
}

const (
	moduleIDenqueue = moduleID + ".enqueue"
)

func (m *goMail) enqueue(msg *gomail.Message) *governor.Error {
	select {
	case m.msgc <- msg:
	default:
		return governor.NewError(moduleIDenqueue, "email service experiencing load", 0, http.StatusInternalServerError)
	}

	return nil
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
	moduleIDSend = moduleID + ".Send"
)

// Send creates and enqueues a new message to be sent
func (m *goMail) Send(to, subject, body string) *governor.Error {
	msg := gomail.NewMessage()
	if len(m.fromName) > 0 {
		msg.SetAddressHeader("From", m.fromAddress, m.fromName)
	} else {
		msg.SetHeader("From", m.fromAddress)
	}
	msg.SetHeader("To", to)
	msg.SetHeader("Subject", subject)
	msg.SetBody("text/html", body)

	return m.enqueue(msg)
}
