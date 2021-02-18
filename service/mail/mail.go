package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/http"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/msgqueue"
	"xorkevin.dev/governor/service/template"
)

const (
	govmailchannelid = "gov.mail"
	govmailworker    = "gov.mail.worker"
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

	mailOp struct {
		from string
		to   []string
		msg  []byte
		res  chan<- error
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
		queue       msgqueue.Msgqueue
		config      governor.SecretReader
		logger      governor.Logger
		host        string
		addr        string
		fromAddress string
		fromName    string
		insecure    bool
		outbox      chan mailOp
		done        <-chan struct{}
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
	queue := msgqueue.GetCtxMsgqueue(inj)
	return New(tpl, queue)
}

// New creates a new Mailer
func New(tpl template.Template, queue msgqueue.Msgqueue) Service {
	return &service{
		tpl:    tpl,
		queue:  queue,
		outbox: make(chan mailOp),
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

	done := make(chan struct{})
	go s.execute(ctx, done)
	s.done = done

	l.Info("initialize mail options", map[string]string{
		"smtp server addr": s.addr,
		"sender address":   s.fromAddress,
		"sender name":      s.fromName,
	})
	return nil
}

func (s *service) Setup(req governor.ReqSetup) error {
	return nil
}

func (s *service) Start(ctx context.Context) error {
	l := s.logger.WithData(map[string]string{
		"phase": "start",
	})

	if _, err := s.queue.Subscribe(govmailchannelid, govmailworker, 15*time.Second, 2, s.mailSubscriber); err != nil {
		return governor.NewError("Failed to subscribe to mail queue", http.StatusInternalServerError, err)
	}
	l.Info("subscribed to mail queue", nil)
	return nil
}

func (s *service) Stop(ctx context.Context) {
	l := s.logger.WithData(map[string]string{
		"phase": "stop",
	})
	select {
	case <-s.done:
		return
	case <-ctx.Done():
		l.Warn("failed to stop", nil)
	}
}

func (s *service) Health() error {
	return nil
}

func (s *service) execute(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	for {
		select {
		case <-ctx.Done():
			return
		case op := <-s.outbox:
			op.res <- s.handleSendMail(op.from, op.to, op.msg)
			close(op.res)
		}
	}
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
		return "", nil, governor.NewError(fmt.Sprintf("Wrong host name: expected %s, have %s", a.host, server.Name), http.StatusInternalServerError, nil)
	}
	resp := []byte(a.identity + "\x00" + a.username + "\x00" + a.password)
	return "PLAIN", resp, nil
}

func (a *plainAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		return nil, governor.NewError("Unexpected server challenge", http.StatusInternalServerError, nil)
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
		return governor.NewError("Invalid secret", http.StatusInternalServerError, nil)
	}
	password, ok := authsecret["password"].(string)
	if !ok {
		return governor.NewError("Invalid secret", http.StatusInternalServerError, nil)
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

func (s *service) mailSubscriber(msgdata []byte) error {
	emmsg := &mailmsg{}
	if err := json.Unmarshal(msgdata, emmsg); err != nil {
		return governor.NewError("Failed to decode mail message", http.StatusInternalServerError, err)
	}
	emdata := map[string]string{}
	if err := json.Unmarshal([]byte(emmsg.Emdata), &emdata); err != nil {
		return governor.NewError("Failed to decode mail data", http.StatusInternalServerError, err)
	}

	subject, err := s.tpl.Execute(emmsg.Subjecttpl, emdata)
	if err != nil {
		return governor.NewError("Failed to execute mail subject template", http.StatusInternalServerError, err)
	}
	body, err := s.tpl.Execute(emmsg.Bodytpl, emdata)
	if err != nil {
		return governor.NewError("Failed to execute mail body template", http.StatusInternalServerError, err)
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

	res := make(chan error)
	op := mailOp{
		from: emmsg.From,
		to:   emmsg.To,
		msg:  msg,
		res:  res,
	}
	select {
	case <-s.done:
		return governor.NewError("Mail service shutdown", http.StatusInternalServerError, err)
	case s.outbox <- op:
		return <-res
	}
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
		return nil, governor.NewError("Failed to write mail", http.StatusInternalServerError, err)
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
		return governor.NewError("Email must have at least one recipient", http.StatusBadRequest, nil)
	}
	datastring, err := json.Marshal(emdata)
	if err != nil {
		return governor.NewError("Failed to encode email data to JSON", http.StatusInternalServerError, err)
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
		return governor.NewError("Failed to encode email to json", http.StatusInternalServerError, err)
	}
	if err := s.queue.Publish(govmailchannelid, b); err != nil {
		return governor.NewError("Failed to publish new email to message queue", http.StatusInternalServerError, err)
	}
	return nil
}
