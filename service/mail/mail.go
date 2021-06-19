package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/template"
	"xorkevin.dev/governor/util/bytefmt"
)

const (
	eventStream         = "DEV_XORKEVIN_GOV_MAIL"
	eventStreamChannels = eventStream + ".*"
	mailChannel         = eventStream + ".mail"
	mailWorker          = eventStream + "_WORKER"
)

type (
	// Mailer is a service wrapper around a mailer instance
	Mailer interface {
		Send(from, fromname string, to []string, tpl string, emdata interface{}) error
	}

	// Service is a Mailer and governor.Service
	Service interface {
		governor.Service
		Mailer
	}

	mailmsg struct {
		From        string   `json:"from"`
		FromName    string   `json:"fromname"`
		To          []string `json:"to"`
		Subjecttpl  string   `json:"subjecttpl"`
		Bodytpl     string   `json:"bodytpl"`
		HTMLBodytpl string   `json:"htmlbodytpl"`
		Emdata      string   `json:"emdata"`
	}

	msgbuilder struct {
		headers  []string
		body     []byte
		htmlbody []byte
	}

	service struct {
		tpl         template.Template
		events      events.Events
		config      governor.SecretReader
		logger      governor.Logger
		host        string
		addr        string
		fromAddress string
		fromName    string
		insecure    bool
		streamsize  int64
		msgsize     int32
	}

	ctxKeyMailer struct{}
)

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
	r.SetDefault("fromaddress", "")
	r.SetDefault("fromname", "")
	r.SetDefault("insecure", false)
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
	s.fromAddress = r.GetStr("fromaddress")
	s.fromName = r.GetStr("fromname")
	s.insecure = r.GetBool("insecure")
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

type plainAuth struct {
	identity, username, password string
	host                         string
}

func newPlainAuth(identity, username, password, host string) smtp.Auth {
	return &plainAuth{identity, username, password, host}

}

func (a *plainAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if server.Name != a.host {
		return "", nil, fmt.Errorf("Wrong host name: expected %s, have %s", a.host, server.Name)
	}
	resp := []byte(a.identity + "\x00" + a.username + "\x00" + a.password)
	return "PLAIN", resp, nil
}

func (a *plainAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		return nil, errors.New("Unexpected server challenge")
	}
	return nil, nil
}

func (s *service) handleSendMail(from string, to []string, msg []byte) error {
	authsecret, err := s.config.GetSecret("auth")
	if err != nil {
		return err
	}

	username, ok := authsecret["username"].(string)
	if !ok {
		return governor.ErrWithKind(nil, governor.ErrInvalidConfig{}, "Invalid secret")
	}
	password, ok := authsecret["password"].(string)
	if !ok {
		return governor.ErrWithKind(nil, governor.ErrInvalidConfig{}, "Invalid secret")
	}
	var smtpauth smtp.Auth
	if s.insecure {
		smtpauth = newPlainAuth("", username, password, s.host)
	} else {
		smtpauth = smtp.PlainAuth("", username, password, s.host)
	}
	if err := smtp.SendMail(s.addr, smtpauth, from, to, msg); err != nil {
		return err
	}
	s.logger.Debug("mail sent", map[string]string{
		"actiontype": "sendmail",
		"addr":       s.addr,
		"username":   username,
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

	subject, err := s.tpl.Execute(emmsg.Subjecttpl, emdata)
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to execute mail subject template")
	}
	body, err := s.tpl.Execute(emmsg.Bodytpl, emdata)
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to execute mail body template")
	}
	htmlbody, err := s.tpl.ExecuteHTML(emmsg.HTMLBodytpl, emdata)
	if err != nil {
		s.logger.Error("failed to execute mail html body template", map[string]string{
			"error":      err.Error(),
			"actiontype": "executehtmlbody",
			"bodytpl":    emmsg.HTMLBodytpl,
		})
		htmlbody = nil
	}

	msg, err := msgToBytes(string(subject), emmsg.From, emmsg.FromName, emmsg.To, body, htmlbody)
	if err != nil {
		return err
	}

	return s.handleSendMail(emmsg.From, emmsg.To, msg)
}

func msgToBytes(subject string, from, fromname string, to []string, body []byte, htmlbody []byte) ([]byte, error) {
	msg := newMsgBuilder()
	msg.addHeader("Mime-Version", "1.0")
	msg.addHeader("Date", time.Now().Round(0).Format(time.RFC1123Z))
	msg.addHeader("Subject", mime.QEncoding.Encode("utf-8", subject))
	if fromname == "" {
		msg.addHeader("From", from)
	} else {
		msg.addAddrHeader("From", mime.QEncoding.Encode("utf-8", fromname), from)
	}
	msg.addHeader("To", strings.Join(to, ",\r\n\t"))
	if body != nil {
		msg.addBody(body)
	}
	if htmlbody != nil {
		msg.addHTMLBody(htmlbody)
	}
	buf, err := msg.build()
	if err != nil {
		return nil, governor.ErrWithKind(err, ErrBuildMail{}, "Failed to construct email")
	}
	return buf.Bytes(), nil
}

func newMsgBuilder() *msgbuilder {
	return &msgbuilder{
		headers: []string{},
		body:    nil,
	}
}

func (b *msgbuilder) addHeader(key, val string) {
	b.headers = append(b.headers, fmt.Sprintf("%s: %s", key, val))
}

func (b *msgbuilder) addAddrHeader(key, name, addr string) {
	b.addHeader(key, fmt.Sprintf("%s <%s>", name, addr))
}

func (b *msgbuilder) addBody(body []byte) {
	b.body = body
}

func (b *msgbuilder) addHTMLBody(body []byte) {
	b.htmlbody = body
}

func (b *msgbuilder) writeHeaders(buf io.StringWriter) {
	for _, h := range b.headers {
		buf.WriteString(h)
		buf.WriteString("\r\n")
	}
}

func (b *msgbuilder) writePart(w io.Writer, data []byte) error {
	qw := quotedprintable.NewWriter(w)
	defer qw.Close()
	if _, err := qw.Write(data); err != nil {
		return err
	}
	return nil
}

func (b *msgbuilder) writeBody(w *multipart.Writer) error {
	defer w.Close()
	if len(b.body) != 0 {
		header := textproto.MIMEHeader{
			"Content-Type":              {mime.FormatMediaType("text/plain", map[string]string{"charset": "utf-8"})},
			"Content-Transfer-Encoding": {"quoted-printable"},
		}
		w, err := w.CreatePart(header)
		if err != nil {
			return err
		}
		if err := b.writePart(w, b.body); err != nil {
			return err
		}
	}
	if len(b.htmlbody) != 0 {
		header := textproto.MIMEHeader{
			"Content-Type":              {mime.FormatMediaType("text/html", map[string]string{"charset": "utf-8"})},
			"Content-Transfer-Encoding": {"quoted-printable"},
		}
		w, err := w.CreatePart(header)
		if err != nil {
			return err
		}
		if err := b.writePart(w, b.htmlbody); err != nil {
			return err
		}
	}
	return nil
}

func genBoundary() string {
	return multipart.NewWriter(&bytes.Buffer{}).Boundary()
}

func createPart(m *multipart.Writer, contenttype string) (*multipart.Writer, error) {
	boundary := genBoundary()

	header := textproto.MIMEHeader{
		"Content-Type": {mime.FormatMediaType(contenttype, map[string]string{"boundary": boundary})},
	}
	part, err := m.CreatePart(header)
	if err != nil {
		return nil, err
	}
	w := multipart.NewWriter(part)
	if err := w.SetBoundary(boundary); err != nil {
		return nil, err
	}
	return w, nil
}

func (b *msgbuilder) build() (*bytes.Buffer, error) {
	buf := &bytes.Buffer{}
	b.writeHeaders(buf)

	m := multipart.NewWriter(buf)
	defer m.Close()
	fmt.Fprintf(buf, "Content-Type: %s\r\n\r\n", mime.FormatMediaType("multipart/mixed", map[string]string{"boundary": m.Boundary(), "charset": "utf-8"}))

	part, err := createPart(m, "multipart/alternative")
	if err != nil {
		return nil, err
	}
	if err := b.writeBody(part); err != nil {
		return nil, err
	}

	return buf, nil
}

// Send creates and enqueues a new message to be sent
func (s *service) Send(from, fromname string, to []string, tpl string, emdata interface{}) error {
	if len(to) == 0 {
		return governor.ErrWithKind(nil, ErrInvalidMail{}, "Email must have at least one recipient")
	}
	datastring, err := json.Marshal(emdata)
	if err != nil {
		return governor.ErrWithKind(err, ErrMailMsg{}, "Failed to encode email data to JSON")
	}

	msg := mailmsg{
		From:        from,
		FromName:    fromname,
		To:          to,
		Subjecttpl:  tpl + "_subject.txt",
		Bodytpl:     tpl + ".txt",
		HTMLBodytpl: tpl + ".html",
		Emdata:      string(datastring),
	}
	if msg.From == "" {
		msg.From = s.fromAddress
	}
	if msg.FromName == "" {
		msg.FromName = s.fromName
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
