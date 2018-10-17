package mail

import (
	"bytes"
	"crypto/tls"
	"encoding/gob"
	"encoding/json"
	gomail "github.com/go-mail/mail"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/msgqueue"
	"github.com/hackform/governor/service/template"
	"github.com/labstack/echo"
	"github.com/nats-io/go-nats-streaming"
	"github.com/sirupsen/logrus"
	"net/http"
	"time"
)

const (
	govmailqueueid      = "governor-mail-queue"
	govmailqueueworker  = "governor-mail-queue-worker"
	govmailqueuedurable = "governor-mail-queue-durable"
)

type (
	// Mail is a service wrapper around a mailer instance
	Mail interface {
		governor.Service
		Send(to, subjecttpl, bodytpl string, emdata interface{}) *governor.Error
	}

	goMail struct {
		logger      *logrus.Logger
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

const (
	moduleID = "mail"
)

// New creates a new mailer service
func New(c governor.Config, l *logrus.Logger, tpl template.Template, queue msgqueue.Msgqueue) (Mail, error) {
	v := c.Conf()
	rconf := v.GetStringMapString("mail")

	l.Infof("mail: smtp server: %s:%s", rconf["host"], rconf["port"])
	l.Infof("mail: buffer_size: %d", v.GetInt("mail.buffer_size"))
	l.Infof("mail: worker_size: %d", v.GetInt("mail.worker_size"))
	l.Infof("mail: conn_msg_cap: %d", v.GetInt("mail.conn_msg_cap"))
	l.Infof("mail: from: %s <%s>", rconf["from_name"], rconf["from_address"])
	l.Info("initialized mail service")

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

const (
	moduleIDmailWorker = moduleID + ".mailWorker"
)

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
				if s, err := d.Dial(); err == nil {
					sender = s
					mailSent = 0
				} else {
					m.logger.Error(governor.NewError(moduleIDmailWorker, "Failed to start dial smtp server: "+err.Error(), 0, http.StatusInternalServerError))
				}
			}
			if sender != nil {
				if err := gomail.Send(sender, msg); err != nil {
					m.logger.Error(governor.NewError(moduleIDmailWorker, "Failed to send to smtp server: "+err.Error(), 0, http.StatusInternalServerError))
				}
				mailSent++
			}

		case <-time.After(30 * time.Second):
			if sender != nil {
				if err := sender.Close(); err != nil {
					m.logger.Error(governor.NewError(moduleIDmailWorker, "Failed to close smtp client: "+err.Error(), 0, http.StatusInternalServerError))
				}
				sender = nil
			}
		}
	}
}

const (
	moduleIDmailSubscriber = moduleID + ".mailSubscriber"
)

func (m *goMail) mailSubscriber(msg *stan.Msg) {
	emmsg := mailmsg{}
	b := bytes.NewBuffer(msg.Data)
	if err := gob.NewDecoder(b).Decode(&emmsg); err != nil {
		m.logger.Error(governor.NewError(moduleIDmailSubscriber, "Failed to decode mailmsg: "+err.Error(), 0, http.StatusInternalServerError))
		return
	}

	emdata := map[string]string{}
	b1 := bytes.NewBufferString(emmsg.Emdata)
	if err := json.NewDecoder(b1).Decode(&emdata); err != nil {
		m.logger.Error(governor.NewError(moduleIDmailSubscriber, "Failed to decode emdata: "+err.Error(), 0, http.StatusInternalServerError))
		return
	}
	if err := m.enqueue(emmsg.To, emmsg.Subjecttpl, emmsg.Bodytpl, emdata); err != nil {
		m.logger.Error(err)
		return
	}
}

const (
	moduleIDstartWorkers = moduleID + ".startWorkers"
)

func (m *goMail) startWorkers() *governor.Error {
	for i := 0; i < m.workerSize; i++ {
		go m.mailWorker()
	}
	_, err := m.queue.Queue().QueueSubscribe(govmailqueueid, govmailqueueworker, m.mailSubscriber, stan.DurableName(govmailqueuedurable))
	if err != nil {
		return governor.NewError(moduleIDstartWorkers, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

const (
	moduleIDenqueue = moduleID + ".enqueue"
)

func (m *goMail) enqueue(to, subjecttpl, bodytpl string, emdata interface{}) *governor.Error {
	body, err := m.tpl.ExecuteHTML(bodytpl, emdata)
	if err != nil {
		err.AddTrace(moduleIDenqueue)
		return err
	}
	subject, err := m.tpl.ExecuteHTML(subjecttpl, emdata)
	if err != nil {
		err.AddTrace(moduleIDenqueue)
		return err
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
		return governor.NewError(moduleIDenqueue, "email service experiencing load", 0, http.StatusInternalServerError)
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
	moduleIDSend = moduleID + ".Send"
)

// Send creates and enqueues a new message to be sent
func (m *goMail) Send(to, subjecttpl, bodytpl string, emdata interface{}) *governor.Error {
	datastring := bytes.Buffer{}
	if err := json.NewEncoder(&datastring).Encode(emdata); err != nil {
		return governor.NewError(moduleIDSend, err.Error(), 0, http.StatusInternalServerError)
	}

	b := bytes.Buffer{}
	if err := gob.NewEncoder(&b).Encode(mailmsg{
		To:         to,
		Subjecttpl: subjecttpl,
		Bodytpl:    bodytpl,
		Emdata:     datastring.String(),
	}); err != nil {
		return governor.NewError(moduleIDSend, err.Error(), 0, http.StatusInternalServerError)
	}
	if err := m.queue.Queue().Publish(govmailqueueid, b.Bytes()); err != nil {
		return governor.NewError(moduleIDSend, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}
