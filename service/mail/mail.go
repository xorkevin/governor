package mail

import (
	"bytes"
	"crypto/tls"
	"encoding/gob"
	"encoding/json"
	gomail "github.com/go-mail/mail"
	"github.com/labstack/echo"
	"net/http"
	"strconv"
	"time"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/msgqueue"
	"xorkevin.dev/governor/service/template"
)

const (
	govmailqueueid     = "gov-mail"
	govmailqueueworker = "gov-mail-worker"
)

type (
	// Mail is a service wrapper around a mailer instance
	Mail interface {
		governor.Service
		Send(to, subjecttpl, bodytpl string, emdata interface{}) error
	}

	goMail struct {
		logger      governor.Logger
		tpl         template.Template
		queue       msgqueue.Msgqueue
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

	mailmsg struct {
		To         string
		Subjecttpl string
		Bodytpl    string
		Emdata     string
	}
)

// New creates a new mailer service
func New(c governor.Config, l governor.Logger, tpl template.Template, queue msgqueue.Msgqueue) (Mail, error) {
	v := c.Conf()
	rconf := v.GetStringMapString("mail")

	l.Info("initialize mail service", map[string]string{
		"smtp server host": rconf["host"],
		"smtp server port": rconf["port"],
		"buffer_size":      strconv.Itoa(v.GetInt("mail.buffer_size")),
		"worker_size":      strconv.Itoa(v.GetInt("mail.worker_size")),
		"conn_msg_cap":     strconv.Itoa(v.GetInt("mail.conn_msg_cap")),
		"sender name":      rconf["from_name"],
		"sender address":   rconf["from_address"],
	})

	gm := &goMail{
		logger:      l,
		tpl:         tpl,
		queue:       queue,
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

	if err := gm.startWorkers(); err != nil {
		return nil, err
	}

	return gm, nil
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

func (m *goMail) mailWorker() {
	cap := m.connMsgCap
	d := m.dialer()
	var sender gomail.SendCloser
	mailSent := 0

	for {
		select {
		case msg, ok := <-m.msgc:
			if !ok {
				return
			}
			if sender == nil || mailSent >= cap && cap > 0 {
				if s, err := d.Dial(); err != nil {
					m.logger.Error("mail: mailWorker: fail dial smtp server", map[string]string{
						"err": err.Error(),
					})
				} else {
					sender = s
					mailSent = 0
				}
			}
			if sender != nil {
				if err := gomail.Send(sender, msg); err != nil {
					m.logger.Error("mail: mailWorker: fail send smtp server", map[string]string{
						"err": err.Error(),
					})
				}
				mailSent++
			}

		case <-time.After(30 * time.Second):
			if sender != nil {
				if err := sender.Close(); err != nil {
					m.logger.Error("mail: mailWorker: fail close smtp client", map[string]string{
						"err": err.Error(),
					})
				}
				sender = nil
			}
		}
	}
}

func (m *goMail) mailSubscriber(msgdata []byte) {
	emmsg := mailmsg{}
	b := bytes.NewBuffer(msgdata)
	if err := gob.NewDecoder(b).Decode(&emmsg); err != nil {
		m.logger.Error("mail: mailSubscriber: fail decode mailmsg", map[string]string{
			"err": err.Error(),
		})
		return
	}

	emdata := map[string]string{}
	b1 := bytes.NewBufferString(emmsg.Emdata)
	if err := json.NewDecoder(b1).Decode(&emdata); err != nil {
		m.logger.Error("mail: mailSubscriber: fail decode emdata", map[string]string{
			"err": err.Error(),
		})
		return
	}
	if err := m.enqueue(emmsg.To, emmsg.Subjecttpl, emmsg.Bodytpl, emdata); err != nil {
		m.logger.Error("mail: mailSubscriber: fail enqueue mail", map[string]string{
			"err": err.Error(),
		})
		return
	}
}

func (m *goMail) startWorkers() error {
	for i := 0; i < m.workerSize; i++ {
		go m.mailWorker()
	}
	if _, err := m.queue.SubscribeQueue(govmailqueueid, govmailqueueworker, m.mailSubscriber); err != nil {
		return governor.NewError("Failed to subscribe to queue", http.StatusInternalServerError, err)
	}
	return nil
}

func (m *goMail) enqueue(to, subjecttpl, bodytpl string, emdata interface{}) error {
	body, err := m.tpl.ExecuteHTML(bodytpl, emdata)
	if err != nil {
		return governor.NewError("Failed to execute mail body template", http.StatusInternalServerError, err)
	}
	subject, err := m.tpl.ExecuteHTML(subjecttpl, emdata)
	if err != nil {
		return governor.NewError("Failed to execute mail subject template", http.StatusInternalServerError, err)
	}

	msg := gomail.NewMessage()
	if len(m.fromName) > 0 {
		msg.SetAddressHeader("From", m.fromAddress, m.fromName)
	} else {
		msg.SetHeader("From", m.fromAddress)
	}
	msg.SetHeader("To", to)
	msg.SetHeader("Subject", subject)
	msg.SetBody("text/html", body)

	select {
	case m.msgc <- msg:
		return nil
	case <-time.After(30 * time.Second):
		return governor.NewError("Email service experiencing load", http.StatusInternalServerError, nil)
	}
}

// Mount is a place to mount routes to satisfy the Service interface
func (m *goMail) Mount(conf governor.Config, l governor.Logger, r *echo.Group) error {
	l.Info("mount mail service", nil)
	return nil
}

// Health is a health check for the service
func (m *goMail) Health() error {
	return nil
}

// Setup is run on service setup
func (m *goMail) Setup(conf governor.Config, l governor.Logger, rsetup governor.ReqSetupPost) error {
	return nil
}

// Send creates and enqueues a new message to be sent
func (m *goMail) Send(to, subjecttpl, bodytpl string, emdata interface{}) error {
	datastring := bytes.Buffer{}
	if err := json.NewEncoder(&datastring).Encode(emdata); err != nil {
		return governor.NewError("Failed to encode email data to JSON", http.StatusInternalServerError, err)
	}

	b := bytes.Buffer{}
	if err := gob.NewEncoder(&b).Encode(mailmsg{
		To:         to,
		Subjecttpl: subjecttpl,
		Bodytpl:    bodytpl,
		Emdata:     datastring.String(),
	}); err != nil {
		return governor.NewError("Failed to encode email to gob", http.StatusInternalServerError, err)
	}
	if err := m.queue.Publish(govmailqueueid, b.Bytes()); err != nil {
		return governor.NewError("Failed to push gob to message queue", http.StatusInternalServerError, err)
	}
	return nil
}
