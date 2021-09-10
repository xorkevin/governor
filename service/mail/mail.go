package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	_ "github.com/emersion/go-message/charset"
	emmail "github.com/emersion/go-message/mail"
	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/template"
	"xorkevin.dev/governor/util/bytefmt"
	"xorkevin.dev/governor/util/uid"
)

const (
	eventStream         = "DEV_XORKEVIN_GOV_MAIL"
	eventStreamChannels = eventStream + ".*"
	mailChannel         = eventStream + ".mail"
	mailWorker          = eventStream + "_WORKER"
)

const (
	mailUIDRandSize = 16
)

type (
	// Mailer is a service wrapper around a mailer instance
	Mailer interface {
		Send(from Addr, to []Addr, tpl Tpl, emdata interface{}) error
	}

	// Service is a Mailer and governor.Service
	Service interface {
		governor.Service
		Mailer
	}

	// Tpl points to a mail template
	Tpl struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
	}

	// Addr is a mail address
	Addr struct {
		Address string `json:"address"`
		Name    string `json:"name"`
	}

	mailmsg struct {
		From   Addr   `json:"from"`
		To     []Addr `json:"to"`
		Tpl    Tpl    `json:"tpl"`
		Emdata string `json:"emdata"`
	}

	msgbuilder struct {
		headers  []string
		body     []byte
		htmlbody []byte
	}

	service struct {
		tpl          template.Template
		events       events.Events
		config       governor.SecretReader
		logger       governor.Logger
		host         string
		addr         string
		senderdomain string
		fromAddress  string
		fromName     string
		streamsize   int64
		msgsize      int32
	}

	ctxKeyMailer struct{}
)

const (
	TplKindLocal = "local"
)

func TplLocal(name string) Tpl {
	return Tpl{
		Kind: TplKindLocal,
		Name: name,
	}
}

// GetCtxMailer returns a Mailer service from the context
func GetCtxMailer(inj governor.Injector) Mailer {
	v := inj.Get(ctxKeyMailer{})
	if v == nil {
		return nil
	}
	return v.(Mailer)
}

// setCtxMailer sets a Mailer service in the context
func setCtxMailer(inj governor.Injector, m Mailer) {
	inj.Set(ctxKeyMailer{}, m)
}

// NewCtx creates a new Mailer service from a context
func NewCtx(inj governor.Injector) Service {
	tpl := template.GetCtxTemplate(inj)
	ev := events.GetCtxEvents(inj)
	return New(tpl, ev)
}

// New creates a new Mailer
func New(tpl template.Template, ev events.Events) Service {
	return &service{
		tpl:    tpl,
		events: ev,
	}
}

func (s *service) Register(inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	setCtxMailer(inj, s)

	r.SetDefault("auth", "")
	r.SetDefault("host", "localhost")
	r.SetDefault("port", "587")
	r.SetDefault("senderdomain", "")
	r.SetDefault("fromaddress", "")
	r.SetDefault("fromname", "")
	r.SetDefault("streamsize", "200M")
	r.SetDefault("msgsize", "2K")
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, m governor.Router) error {
	s.logger = l
	l = s.logger.WithData(map[string]string{
		"phase": "init",
	})

	s.config = r

	s.host = r.GetStr("host")
	s.addr = fmt.Sprintf("%s:%s", r.GetStr("host"), r.GetStr("port"))
	s.senderdomain = r.GetStr("senderdomain")
	s.fromAddress = r.GetStr("fromaddress")
	s.fromName = r.GetStr("fromname")
	var err error
	s.streamsize, err = bytefmt.ToBytes(r.GetStr("streamsize"))
	if err != nil {
		return governor.ErrWithMsg(err, "Invalid stream size")
	}
	msgsize, err := bytefmt.ToBytes(r.GetStr("msgsize"))
	if err != nil {
		return governor.ErrWithMsg(err, "Invalid msg size")
	}
	s.msgsize = int32(msgsize)

	l.Info("initialize mail options", map[string]string{
		"smtp server addr":    s.addr,
		"sender domain":       s.senderdomain,
		"sender address":      s.fromAddress,
		"sender name":         s.fromName,
		"stream size (bytes)": r.GetStr("streamsize"),
		"msg size (bytes)":    r.GetStr("msgsize"),
	})
	return nil
}

func (s *service) Setup(req governor.ReqSetup) error {
	l := s.logger.WithData(map[string]string{
		"phase": "setup",
	})
	if err := s.events.InitStream(eventStream, []string{eventStreamChannels}, events.StreamOpts{
		Replicas:   1,
		MaxAge:     30 * 24 * time.Hour,
		MaxBytes:   s.streamsize,
		MaxMsgSize: s.msgsize,
	}); err != nil {
		return governor.ErrWithMsg(err, "Failed to init mail stream")
	}
	l.Info("Created mail stream", nil)
	return nil
}

func (s *service) PostSetup(req governor.ReqSetup) error {
	return nil
}

func (s *service) Start(ctx context.Context) error {
	l := s.logger.WithData(map[string]string{
		"phase": "start",
	})

	if _, err := s.events.StreamSubscribe(eventStream, mailChannel, mailWorker, s.mailSubscriber, events.StreamConsumerOpts{
		AckWait:     30 * time.Second,
		MaxDeliver:  30,
		MaxPending:  1024,
		MaxRequests: 32,
	}); err != nil {
		return governor.ErrWithMsg(err, "Failed to subscribe to mail queue")
	}
	l.Info("Subscribed to mail queue", nil)
	return nil
}

func (s *service) Stop(ctx context.Context) {
}

func (s *service) Health() error {
	return nil
}

type (
	secretAuth struct {
		Username string `mapstructure:"username"`
		Password string `mapstructure:"password"`
	}
)

func (s *service) handleSendMail(from string, to []string, msg io.Reader) error {
	var secret secretAuth
	if err := s.config.GetSecret("auth", 0, &secret); err != nil {
		return governor.ErrWithMsg(err, "Invalid secret")
	}

	auth := sasl.NewPlainClient("", secret.Username, secret.Password)
	if err := smtp.SendMail(s.addr, auth, from, to, msg); err != nil {
		return err
	}
	s.logger.Debug("mail sent", map[string]string{
		"actiontype": "sendmail",
		"addr":       s.addr,
		"username":   secret.Username,
		"from":       from,
		"to":         strings.Join(to, ","),
	})
	return nil
}

type (
	// ErrMailMsg is returned when the msgqueue mail message is malformed
	ErrMailMsg struct{}
	// ErrInvalidMail is returned when the mail message is invalid
	ErrInvalidMail struct{}
	// ErrBuildMail is returned when failing to build an email message
	ErrBuildMail struct{}
)

func (e ErrMailMsg) Error() string {
	return "Malformed mail message"
}

func (e ErrInvalidMail) Error() string {
	return "Invalid mail"
}

func (e ErrBuildMail) Error() string {
	return "Error building email"
}

func (s *service) mailSubscriber(pinger events.Pinger, msgdata []byte) error {
	emmsg := &mailmsg{}
	if err := json.Unmarshal(msgdata, emmsg); err != nil {
		return governor.ErrWithKind(err, ErrMailMsg{}, "Failed to decode mail message")
	}
	emdata := map[string]string{}
	if err := json.Unmarshal([]byte(emmsg.Emdata), &emdata); err != nil {
		return governor.ErrWithKind(err, ErrMailMsg{}, "Failed to decode mail data")
	}

	subject := &bytes.Buffer{}
	body := &bytes.Buffer{}
	htmlbody := &bytes.Buffer{}
	switch emmsg.Tpl.Kind {
	case TplKindLocal:
		if err := s.tpl.Execute(subject, emmsg.Tpl.Name+"_subject.txt", emdata); err != nil {
			return governor.ErrWithMsg(err, "Failed to execute mail subject template")
		}
		if err := s.tpl.Execute(body, emmsg.Tpl.Name+".txt", emdata); err != nil {
			return governor.ErrWithMsg(err, "Failed to execute mail body template")
		}
		if err := s.tpl.ExecuteHTML(htmlbody, emmsg.Tpl.Name+".html", emdata); err != nil {
			s.logger.Error("Failed to execute mail html body template", map[string]string{
				"error":      err.Error(),
				"actiontype": "executehtmlbody",
				"bodytpl":    emmsg.Tpl.Name + ".html",
			})
			htmlbody = nil
		}
	default:
		return governor.ErrWithKind(nil, ErrMailMsg{}, "Invalid mail message kind")
	}

	b := &bytes.Buffer{}
	if err := msgToBytes(s.logger, s.senderdomain, emmsg.From, emmsg.To, subject, body, htmlbody, b); err != nil {
		return err
	}

	to := make([]string, 0, len(emmsg.To))
	for _, i := range emmsg.To {
		to = append(to, i.Address)
	}
	return s.handleSendMail(emmsg.From.Address, to, b)
}

func msgToBytes(l governor.Logger, senderdomain string, from Addr, to []Addr, subject, body, htmlbody io.Reader, dst io.Writer) error {
	h := emmail.Header{}
	u, err := uid.NewSnowflake(mailUIDRandSize)
	if err != nil {
		return governor.ErrWithKind(err, ErrBuildMail{}, "Failed to generate mail msg id")
	}
	h.SetMessageID(fmt.Sprintf("%s@%s", u.Base32(), senderdomain))
	h.SetDate(time.Now().Round(0).In(time.UTC))
	subj := strings.Builder{}
	if _, err := io.Copy(&subj, subject); err != nil {
		return governor.ErrWithKind(err, ErrBuildMail{}, "Failed to write mail subject")
	}
	h.SetSubject(subj.String())
	h.SetAddressList("From", []*emmail.Address{
		{
			Address: from.Address,
			Name:    from.Name,
		},
	})
	emto := make([]*emmail.Address, 0, len(to))
	for _, i := range to {
		emto = append(emto, &emmail.Address{
			Address: i.Address,
			Name:    i.Name,
		})
	}
	h.SetAddressList("To", emto)

	mw, err := emmail.CreateWriter(dst, h)
	if err != nil {
		return governor.ErrWithKind(err, ErrBuildMail{}, "Failed to create mail writer")
	}
	defer func() {
		if err := mw.Close(); err != nil {
			l.Error("Failed closing mail writer", map[string]string{
				"error":      err.Error(),
				"actiontype": "closemailwriter",
			})
		}
	}()

	bw, err := mw.CreateInline()
	if err != nil {
		return governor.ErrWithKind(err, ErrBuildMail{}, "Failed to create mail body writer")
	}
	defer func() {
		if err := bw.Close(); err != nil {
			l.Error("Failed closing mail body writer", map[string]string{
				"error":      err.Error(),
				"actiontype": "closemailbodywriter",
			})
		}
	}()

	if body != nil {
		if err := func() error {
			hh := emmail.InlineHeader{}
			hh.SetContentType("text/plain", map[string]string{"charset": "utf-8"})
			pw, err := bw.CreatePart(hh)
			if err != nil {
				return governor.ErrWithKind(err, ErrBuildMail{}, "Failed to create mail body plaintext writer")
			}
			defer func() {
				if err := pw.Close(); err != nil {
					l.Error("Failed closing mail body plaintext writer", map[string]string{
						"error":      err.Error(),
						"actiontype": "closemailbodyplaintextwriter",
					})
				}
			}()
			if _, err := io.Copy(pw, body); err != nil {
				return governor.ErrWithKind(err, ErrBuildMail{}, "Failed to write plaintext mail body")
			}
			return nil
		}(); err != nil {
			return err
		}
	}

	if htmlbody != nil {
		if err := func() error {
			hh := emmail.InlineHeader{}
			hh.SetContentType("text/html", map[string]string{"charset": "utf-8"})
			pw, err := bw.CreatePart(hh)
			if err != nil {
				return governor.ErrWithKind(err, ErrBuildMail{}, "Failed to create mail body html writer")
			}
			defer func() {
				if err := pw.Close(); err != nil {
					l.Error("Failed closing mail body plaintext writer", map[string]string{
						"error":      err.Error(),
						"actiontype": "closemailbodyhtmlwriter",
					})
				}
			}()
			if _, err := io.Copy(pw, htmlbody); err != nil {
				return governor.ErrWithKind(err, ErrBuildMail{}, "Failed to write html mail body")
			}
			return nil
		}(); err != nil {
			return err
		}
	}
	return nil
}

// Send creates and enqueues a new message to be sent
func (s *service) Send(from Addr, to []Addr, tpl Tpl, emdata interface{}) error {
	if len(to) == 0 {
		return governor.ErrWithKind(nil, ErrInvalidMail{}, "Email must have at least one recipient")
	}
	datastring, err := json.Marshal(emdata)
	if err != nil {
		return governor.ErrWithKind(err, ErrMailMsg{}, "Failed to encode email data to JSON")
	}

	if from.Address == "" {
		from.Address = s.fromAddress
	}
	if from.Name == "" {
		from.Name = s.fromName
	}
	msg := mailmsg{
		From:   from,
		To:     to,
		Tpl:    tpl,
		Emdata: string(datastring),
	}

	b, err := json.Marshal(msg)
	if err != nil {
		return governor.ErrWithKind(err, ErrMailMsg{}, "Failed to encode email to json")
	}
	if err := s.events.StreamPublish(mailChannel, b); err != nil {
		return governor.ErrWithMsg(err, "Failed to publish new email to message queue")
	}
	return nil
}
