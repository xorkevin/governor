package mail

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/gob"
	"encoding/json"
	gomail "github.com/go-mail/mail"
	"github.com/labstack/echo"
	"net/http"
	"strconv"
	"sync"
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

	service struct {
		tpl         template.Template
		queue       msgqueue.Msgqueue
		logger      governor.Logger
		host        string
		port        int
		username    string
		password    string
		insecure    bool
		workerSize  int
		connMsgCap  int
		connTimeout int
		fromAddress string
		fromName    string
		msgc        chan *gomail.Message
		done        <-chan struct{}
	}

	mailmsg struct {
		To         string
		Subjecttpl string
		Bodytpl    string
		Emdata     string
	}
)

// New creates a new mailer service
func New(tpl template.Template, queue msgqueue.Msgqueue) Mail {
	return &service{
		tpl:   tpl,
		queue: queue,
	}
}

func (s *service) Register(r governor.ConfigRegistrar) {
	r.SetDefault("host", "localhost")
	r.SetDefault("port", "587")
	r.SetDefault("username", "")
	r.SetDefault("password", "")
	r.SetDefault("insecure", false)
	r.SetDefault("workersize", 2)
	r.SetDefault("connmsgcap", 0)
	r.SetDefault("conntimeout", 4)
	r.SetDefault("fromaddress", "")
	r.SetDefault("fromname", "")
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, g *echo.Group) error {
	s.logger = l
	conf := r.GetStrMap("")

	s.host = conf["host"]
	s.port = r.GetInt("port")
	s.username = conf["username"]
	s.password = conf["password"]
	s.insecure = r.GetBool("insecure")
	s.workerSize = r.GetInt("workersize")
	s.connMsgCap = r.GetInt("connmsgcap")
	s.connTimeout = r.GetInt("conntimeout")
	s.fromAddress = conf["fromaddress"]
	s.fromName = conf["fromname"]
	s.msgc = make(chan *gomail.Message)

	s.logger.Info("mail: initialize service options", map[string]string{
		"smtp server host": conf["host"],
		"smtp server port": conf["port"],
		"worker size":      strconv.Itoa(r.GetInt("workersize")),
		"conn msg cap":     strconv.Itoa(r.GetInt("connmsgcap")),
		"conn timeout (s)": strconv.Itoa(r.GetInt("conntimeout")),
		"sender address":   conf["fromaddress"],
		"sender name":      conf["fromname"],
	})
	return nil
}

func (s *service) Setup(req governor.ReqSetup) error {
	return nil
}

func (s *service) Start(ctx context.Context) error {
	s.logger.Info("mail: starting mail workers", map[string]string{
		"count": strconv.Itoa(s.workerSize),
	})
	wg := &sync.WaitGroup{}
	for i := 0; i < s.workerSize; i++ {
		wg.Add(1)
		go s.mailWorker(strconv.Itoa(i), wg)
	}
	if _, err := s.queue.SubscribeQueue(govmailqueueid, govmailqueueworker, s.mailSubscriber); err != nil {
		s.logger.Error("mail: failed to subscribe to mail queue", map[string]string{
			"error": err.Error(),
		})
		return governor.NewError("Failed to subscribe to queue", http.StatusInternalServerError, err)
	}
	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		close(s.msgc)
		wg.Wait()
		done <- struct{}{}
	}()
	s.done = done

	s.logger.Info("mail: subscribed to mail queue", nil)
	return nil
}

func (s *service) Stop(ctx context.Context) {
	select {
	case <-s.done:
		return
	case <-ctx.Done():
		s.logger.Warn("mail: failed to stop", nil)
	}
}

func (s *service) Health() error {
	return nil
}

func (s *service) mailWorker(id string, wg *sync.WaitGroup) {
	defer (func() {
		wg.Done()
		s.logger.Info("mail: mail worker stopped", map[string]string{
			"mailworkerid": id,
		})
	})()
	s.logger.Info("mail: mail worker started", map[string]string{
		"mailworkerid": id,
	})
	d := gomail.NewDialer(s.host, s.port, s.username, s.password)
	if s.insecure {
		d.TLSConfig = &tls.Config{
			ServerName:         s.host,
			InsecureSkipVerify: true,
		}
	}
	var sender gomail.SendCloser
	mailSent := 0

	for {
		select {
		case msg, ok := <-s.msgc:
			if !ok {
				if sender != nil {
					if err := sender.Close(); err != nil {
						s.logger.Error("mail: mailWorker: fail close smtp client", map[string]string{
							"error": err.Error(),
						})
					}
					sender = nil
					s.logger.Error("mail: mailWorker: close smtp client", nil)
					return
				}
				return
			}
			if sender == nil || mailSent >= s.connMsgCap && s.connMsgCap > 0 {
				if send, err := d.Dial(); err != nil {
					s.logger.Error("mail: mailWorker: fail dial smtp server", map[string]string{
						"error": err.Error(),
					})
				} else {
					sender = send
					mailSent = 0
				}
			}
			if sender != nil {
				if err := gomail.Send(sender, msg); err != nil {
					s.logger.Error("mail: mailWorker: fail send smtp server", map[string]string{
						"error": err.Error(),
					})
				}
				mailSent++
			}

		case <-time.After(time.Duration(s.connTimeout) * time.Second):
			if sender != nil {
				if err := sender.Close(); err != nil {
					s.logger.Error("mail: mailWorker: fail close smtp client", map[string]string{
						"error": err.Error(),
					})
				}
				sender = nil
			}
		}
	}
}

func (s *service) mailSubscriber(msgdata []byte) {
	emmsg := mailmsg{}
	b := bytes.NewBuffer(msgdata)
	if err := gob.NewDecoder(b).Decode(&emmsg); err != nil {
		s.logger.Error("mail: mailSubscriber: fail decode mailmsg", map[string]string{
			"error": err.Error(),
		})
		return
	}

	emdata := map[string]string{}
	b1 := bytes.NewBufferString(emmsg.Emdata)
	if err := json.NewDecoder(b1).Decode(&emdata); err != nil {
		s.logger.Error("mail: mailSubscriber: fail decode emdata", map[string]string{
			"error": err.Error(),
		})
		return
	}
	if err := s.enqueue(emmsg.To, emmsg.Subjecttpl, emmsg.Bodytpl, emdata); err != nil {
		s.logger.Error("mail: mailSubscriber: fail enqueue mail", map[string]string{
			"error": err.Error(),
		})
		return
	}
}

func (s *service) enqueue(to, subjecttpl, bodytpl string, emdata interface{}) error {
	body, err := s.tpl.ExecuteHTML(bodytpl, emdata)
	if err != nil {
		return governor.NewError("Failed to execute mail body template", http.StatusInternalServerError, err)
	}
	subject, err := s.tpl.ExecuteHTML(subjecttpl, emdata)
	if err != nil {
		return governor.NewError("Failed to execute mail subject template", http.StatusInternalServerError, err)
	}

	msg := gomail.NewMessage()
	if len(s.fromName) > 0 {
		msg.SetAddressHeader("From", s.fromAddress, s.fromName)
	} else {
		msg.SetHeader("From", s.fromAddress)
	}
	msg.SetHeader("To", to)
	msg.SetHeader("Subject", subject)
	msg.SetBody("text/html", body)

	select {
	case s.msgc <- msg:
		return nil
	case <-time.After(time.Duration(s.connTimeout) * time.Second):
		return governor.NewError("Email service experiencing load", http.StatusInternalServerError, nil)
	}
}

// Send creates and enqueues a new message to be sent
func (s *service) Send(to, subjecttpl, bodytpl string, emdata interface{}) error {
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
	if err := s.queue.Publish(govmailqueueid, b.Bytes()); err != nil {
		return governor.NewError("Failed to push gob to message queue", http.StatusInternalServerError, err)
	}
	return nil
}
