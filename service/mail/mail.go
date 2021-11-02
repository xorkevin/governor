package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	_ "github.com/emersion/go-message/charset"
	emmail "github.com/emersion/go-message/mail"
	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/objstore"
	"xorkevin.dev/governor/service/template"
	"xorkevin.dev/governor/util/bytefmt"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/hunter2"
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

const (
	mailMsgKindTpl = "tpl"
	mailMsgKindRaw = "raw"
	mailMsgKindFwd = "fwd"
)

const (
	mediaTypeTextPlain = "text/plain"
	mediaTypeTextHTML  = "text/html"
	mediaTypeOctet     = "application/octet-stream"
)

type (
	// Mailer is a service wrapper around a mailer instance
	Mailer interface {
		Send(from Addr, to []Addr, tpl Tpl, emdata interface{}, encrypt bool) error
		SendStream(from Addr, to []Addr, subject string, size int64, body io.Reader, encrypt bool) error
		FwdStream(from string, to []string, size int64, body io.Reader, encrypt bool) error
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

	tplData struct {
		Tpl       Tpl    `json:"tpl"`
		Emdata    string `json:"emdata"`
		Encrypted bool   `json:"encrypted"`
	}

	rawData struct {
		Subject   string `json:"subject"`
		Path      string `json:"path"`
		Key       string `json:"key"`
		Tag       string `json:"tag"`
		Encrypted bool   `json:"encrypted"`
	}

	fwdData struct {
		Path      string `json:"path"`
		Key       string `json:"key"`
		Tag       string `json:"tag"`
		Encrypted bool   `json:"encrypted"`
	}

	// Addr is a mail address
	Addr struct {
		Address string `json:"address"`
		Name    string `json:"name"`
	}

	mailmsg struct {
		From    Addr    `json:"from"`
		To      []Addr  `json:"to"`
		Kind    string  `json:"kind"`
		TplData tplData `json:"tpl_data"`
		RawData rawData `json:"raw_data"`
		FwdData fwdData `json:"fwd_data"`
	}

	msgbuilder struct {
		headers  []string
		body     []byte
		htmlbody []byte
	}

	service struct {
		tpl               template.Template
		events            events.Events
		mailBucket        objstore.Bucket
		sendMailDir       objstore.Dir
		config            governor.SecretReader
		maildataDecrypter *hunter2.Decrypter
		maildataCipher    hunter2.Cipher
		logger            governor.Logger
		host              string
		addr              string
		msgiddomain       string
		fromAddress       string
		fromName          string
		streamsize        int64
		eventsize         int32
	}

	ctxKeyMailer struct{}
)

func TplLocal(name string) Tpl {
	return Tpl{
		Kind: template.KindLocal,
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
	obj := objstore.GetCtxBucket(inj)
	return New(tpl, ev, obj)
}

// New creates a new Mailer
func New(tpl template.Template, ev events.Events, obj objstore.Bucket) Service {
	return &service{
		tpl:         tpl,
		events:      ev,
		mailBucket:  obj,
		sendMailDir: obj.Subdir("sendmail"),
	}
}

func (s *service) Register(inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	setCtxMailer(inj, s)

	r.SetDefault("auth", "")
	r.SetDefault("host", "localhost")
	r.SetDefault("port", "587")
	r.SetDefault("msgiddomain", "")
	r.SetDefault("fromaddress", "")
	r.SetDefault("fromname", "")
	r.SetDefault("streamsize", "200M")
	r.SetDefault("eventsize", "2K")
}

type (
	secretMaildata struct {
		Keys []string `mapstructure:"secrets"`
	}
)

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, m governor.Router) error {
	s.logger = l
	l = s.logger.WithData(map[string]string{
		"phase": "init",
	})

	s.config = r

	s.host = r.GetStr("host")
	s.addr = fmt.Sprintf("%s:%s", r.GetStr("host"), r.GetStr("port"))
	s.msgiddomain = r.GetStr("msgiddomain")
	s.fromAddress = r.GetStr("fromaddress")
	s.fromName = r.GetStr("fromname")
	var err error
	s.streamsize, err = bytefmt.ToBytes(r.GetStr("streamsize"))
	if err != nil {
		return governor.ErrWithMsg(err, "Invalid stream size")
	}
	eventsize, err := bytefmt.ToBytes(r.GetStr("eventsize"))
	if err != nil {
		return governor.ErrWithMsg(err, "Invalid msg size")
	}
	s.eventsize = int32(eventsize)

	maildataSecrets := secretMaildata{}
	if err := r.GetSecret("mailkey", 0, &maildataSecrets); err != nil {
		return governor.ErrWithMsg(err, "Invalid mailkey secrets")
	}
	if len(maildataSecrets.Keys) == 0 {
		return governor.ErrWithKind(nil, governor.ErrInvalidConfig{}, "No otpkey present")
	}
	s.maildataDecrypter = hunter2.NewDecrypter()
	for n, i := range maildataSecrets.Keys {
		cipher, err := hunter2.CipherFromParams(i, hunter2.DefaultCipherAlgs)
		if err != nil {
			return governor.ErrWithKind(err, governor.ErrInvalidConfig{}, "Invalid cipher param")
		}
		if n == 0 {
			s.maildataCipher = cipher
		}
		s.maildataDecrypter.RegisterCipher(cipher)
	}

	l.Info("Initialize mail service", map[string]string{
		"smtp server addr":    s.addr,
		"msgid domain":        s.msgiddomain,
		"sender address":      s.fromAddress,
		"sender name":         s.fromName,
		"stream size (bytes)": r.GetStr("streamsize"),
		"event size (bytes)":  r.GetStr("eventsize"),
		"nummaildatakeys":     strconv.Itoa(len(maildataSecrets.Keys)),
	})
	return nil
}

func (s *service) Setup(req governor.ReqSetup) error {
	l := s.logger.WithData(map[string]string{
		"phase": "setup",
	})
	if err := s.mailBucket.Init(); err != nil {
		return governor.ErrWithMsg(err, "Failed to init mail bucket")
	}
	l.Info("Created mail bucket", nil)
	if err := s.events.InitStream(eventStream, []string{eventStreamChannels}, events.StreamOpts{
		Replicas:   1,
		MaxAge:     30 * 24 * time.Hour,
		MaxBytes:   s.streamsize,
		MaxMsgSize: s.eventsize,
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
	s.logger.Debug("Mail sent", map[string]string{
		"actiontype": "sendmail",
		"addr":       s.addr,
		"username":   secret.Username,
		"from":       from,
		"to":         strings.Join(to, ","),
	})
	return nil
}

type (
	// ErrMailEvent is returned when the msgqueue mail message is malformed
	ErrMailEvent struct{}
	// ErrInvalidMail is returned when the mail message is invalid
	ErrInvalidMail struct{}
	// ErrBuildMail is returned when failing to build an email message
	ErrBuildMail struct{}
)

func (e ErrMailEvent) Error() string {
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
		return governor.ErrWithKind(err, ErrMailEvent{}, "Failed to decode mail message")
	}

	var msg io.Reader

	var tag string
	var auth *hunter2.Poly1305Auth

	if emmsg.Kind == mailMsgKindFwd {
		data := emmsg.FwdData
		b1, _, err := s.sendMailDir.Get(data.Path)
		if err != nil {
			return governor.ErrWithKind(err, ErrMailEvent{}, "Failed to get mail body")
		}
		defer func() {
			if err := b1.Close(); err != nil {
				s.logger.Error("Failed to close mail body", map[string]string{
					"actiontype": "getmailbody",
					"error":      err.Error(),
				})
			}
		}()
		msg = b1
		if data.Encrypted {
			var err error
			data.Key, err = s.maildataDecrypter.Decrypt(data.Key)
			if err != nil {
				return governor.ErrWithKind(err, ErrMailEvent{}, "Failed to decrypt mail data key")
			}
			config, err := hunter2.ParseChaCha20Config(data.Key)
			if err != nil {
				return governor.ErrWithKind(err, ErrMailEvent{}, "Failed to parse mail data key")
			}
			stream, err := hunter2.NewChaCha20Stream(*config)
			if err != nil {
				return governor.ErrWithMsg(err, "Failed to create decryption stream")
			}
			auth, err = hunter2.NewPoly1305Auth(*config)
			if err != nil {
				return governor.ErrWithMsg(err, "Failed to create decryption auth")
			}
			tag = data.Tag
			msg = hunter2.NewDecStreamReader(stream, auth, msg)
		}
	} else {
		var subject, body, htmlbody io.Reader
		if emmsg.Kind == mailMsgKindRaw {
			data := emmsg.RawData
			b1, _, err := s.sendMailDir.Get(data.Path)
			if err != nil {
				return governor.ErrWithKind(err, ErrMailEvent{}, "Failed to get mail body")
			}
			defer func() {
				if err := b1.Close(); err != nil {
					s.logger.Error("Failed to close mail body", map[string]string{
						"actiontype": "getmailbody",
						"error":      err.Error(),
					})
				}
			}()
			body = b1
			if data.Encrypted {
				var err error
				data.Subject, err = s.maildataDecrypter.Decrypt(data.Subject)
				if err != nil {
					return governor.ErrWithKind(err, ErrMailEvent{}, "Failed to decrypt mail subject")
				}
				data.Key, err = s.maildataDecrypter.Decrypt(data.Key)
				if err != nil {
					return governor.ErrWithKind(err, ErrMailEvent{}, "Failed to decrypt mail data key")
				}
				config, err := hunter2.ParseChaCha20Config(data.Key)
				if err != nil {
					return governor.ErrWithKind(err, ErrMailEvent{}, "Failed to parse mail data key")
				}
				stream, err := hunter2.NewChaCha20Stream(*config)
				if err != nil {
					return governor.ErrWithMsg(err, "Failed to create decryption stream")
				}
				auth, err = hunter2.NewPoly1305Auth(*config)
				if err != nil {
					return governor.ErrWithMsg(err, "Failed to create decryption auth")
				}
				tag = data.Tag
				body = hunter2.NewDecStreamReader(stream, auth, body)
			}
			subject = strings.NewReader(data.Subject)
		} else if emmsg.Kind == mailMsgKindTpl {
			data := emmsg.TplData
			if data.Encrypted {
				var err error
				data.Emdata, err = s.maildataDecrypter.Decrypt(data.Emdata)
				if err != nil {
					return governor.ErrWithKind(err, ErrMailEvent{}, "Failed to decrypt mail data")
				}
			}
			emdata := map[string]string{}
			if err := json.Unmarshal([]byte(data.Emdata), &emdata); err != nil {
				return governor.ErrWithKind(err, ErrMailEvent{}, "Failed to decode mail data")
			}

			s1 := &bytes.Buffer{}
			b1 := &bytes.Buffer{}
			b2 := &bytes.Buffer{}
			if err := s.tpl.Execute(s1, data.Tpl.Kind, data.Tpl.Name+"_subject.txt", emdata); err != nil {
				return governor.ErrWithMsg(err, "Failed to execute mail subject template")
			}
			subject = s1
			if err := s.tpl.Execute(b1, data.Tpl.Kind, data.Tpl.Name+".txt", emdata); err != nil {
				return governor.ErrWithMsg(err, "Failed to execute mail body template")
			}
			body = b1
			if err := s.tpl.ExecuteHTML(b2, data.Tpl.Kind, data.Tpl.Name+".html", emdata); err != nil {
				if !errors.Is(err, template.ErrTemplateDNE{}) {
					s.logger.Error("Failed to execute mail html body template", map[string]string{
						"error":      err.Error(),
						"actiontype": "executehtmlbody",
						"bodytpl":    data.Tpl.Name + ".html",
					})
				}
				htmlbody = nil
			} else {
				htmlbody = b2
			}
		} else {
			return governor.ErrWithKind(nil, ErrMailEvent{}, "Invalid mail message kind")
		}

		b := &bytes.Buffer{}
		if err := msgToBytes(s.logger, s.msgiddomain, emmsg.From, emmsg.To, subject, body, htmlbody, b); err != nil {
			return err
		}
		msg = b
	}

	if auth != nil {
		if err := auth.WriteCount(); err != nil {
			return governor.ErrWithMsg(err, "Failed to write auth content length")
		}
		if err := auth.Auth(tag); err != nil {
			return governor.ErrWithKind(err, ErrMailEvent{}, "Mail body failed authentication")
		}
	}

	to := make([]string, 0, len(emmsg.To))
	for _, i := range emmsg.To {
		to = append(to, i.Address)
	}
	return s.handleSendMail(emmsg.From.Address, to, msg)
}

func msgToBytes(l governor.Logger, msgiddomain string, from Addr, to []Addr, subject, body, htmlbody io.Reader, dst io.Writer) error {
	h := emmail.Header{}
	u, err := uid.NewSnowflake(mailUIDRandSize)
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to generate mail msg id")
	}
	h.SetMessageID(fmt.Sprintf("%s@%s", u.Base32(), msgiddomain))
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

	if htmlbody == nil {
		h.SetContentType(mediaTypeTextPlain, map[string]string{"charset": "utf-8"})
		mw, err := emmail.CreateSingleInlineWriter(dst, h)
		if err != nil {
			return governor.ErrWithKind(err, ErrBuildMail{}, "Failed to create plain mail writer")
		}
		defer func() {
			if err := mw.Close(); err != nil {
				l.Error("Failed closing plain mail writer", map[string]string{
					"error":      err.Error(),
					"actiontype": "closeplainmailwriter",
				})
			}
		}()
		if _, err := io.Copy(mw, body); err != nil {
			return governor.ErrWithKind(err, ErrBuildMail{}, "Failed to write plaintext mail body")
		}
		return nil
	}

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

	if err := func() error {
		hh := emmail.InlineHeader{}
		hh.SetContentType(mediaTypeTextPlain, map[string]string{"charset": "utf-8"})
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

	if err := func() error {
		hh := emmail.InlineHeader{}
		hh.SetContentType(mediaTypeTextHTML, map[string]string{"charset": "utf-8"})
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
	return nil
}

// Send creates and sends a message given a template and data
func (s *service) Send(from Addr, to []Addr, tpl Tpl, emdata interface{}, encrypt bool) error {
	if len(to) == 0 {
		return governor.ErrWithKind(nil, ErrInvalidMail{}, "Email must have at least one recipient")
	}
	databytes, err := json.Marshal(emdata)
	if err != nil {
		return governor.ErrWithKind(err, ErrInvalidMail{}, "Failed to encode email data to JSON")
	}
	datastring := string(databytes)
	if encrypt {
		var err error
		datastring, err = s.maildataCipher.Encrypt(datastring)
		if err != nil {
			return governor.ErrWithMsg(err, "Failed to encrypt mail data")
		}
	}

	if from.Address == "" {
		from.Address = s.fromAddress
	}
	if from.Name == "" {
		from.Name = s.fromName
	}
	msg := mailmsg{
		From: from,
		To:   to,
		Kind: mailMsgKindTpl,
		TplData: tplData{
			Tpl:       tpl,
			Emdata:    datastring,
			Encrypted: encrypt,
		},
	}

	b, err := json.Marshal(msg)
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to encode mail event to json")
	}
	if err := s.events.StreamPublish(mailChannel, b); err != nil {
		return governor.ErrWithMsg(err, "Failed to publish mail event")
	}
	return nil
}

// SendStream creates and sends a message from a given body
func (s *service) SendStream(from Addr, to []Addr, subject string, size int64, body io.Reader, encrypt bool) error {
	if len(to) == 0 {
		return governor.ErrWithKind(nil, ErrInvalidMail{}, "Email must have at least one recipient")
	}

	u, err := uid.NewSnowflake(mailUIDRandSize)
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to generate mail body obj id")
	}
	path := u.Base32()

	contentType := mediaTypeTextPlain

	var key string
	var tag string
	var auth *hunter2.Poly1305Auth
	if encrypt {
		var err error
		subject, err = s.maildataCipher.Encrypt(subject)
		if err != nil {
			return governor.ErrWithMsg(err, "Failed to encrypt mail subject")
		}

		contentType = mediaTypeOctet
		config, err := hunter2.NewChaCha20Config()
		if err != nil {
			return governor.ErrWithMsg(err, "Failed to create mail data key")
		}
		key, err = s.maildataCipher.Encrypt(config.String())
		if err != nil {
			return governor.ErrWithMsg(err, "Failed to encrypt mail data key")
		}
		stream, err := hunter2.NewChaCha20Stream(*config)
		if err != nil {
			return governor.ErrWithMsg(err, "Failed to create encryption stream")
		}
		auth, err = hunter2.NewPoly1305Auth(*config)
		if err != nil {
			return governor.ErrWithMsg(err, "Failed to create encryption auth")
		}
		body = hunter2.NewEncStreamReader(stream, auth, body)
	}

	if err := s.sendMailDir.Put(path, contentType, size, nil, body); err != nil {
		return governor.ErrWithMsg(err, "Failed to save mail body")
	}
	if auth != nil {
		if err := auth.WriteCount(); err != nil {
			return governor.ErrWithMsg(err, "Failed to write auth content length")
		}
		tag = auth.String()
	}

	if from.Address == "" {
		from.Address = s.fromAddress
	}
	if from.Name == "" {
		from.Name = s.fromName
	}
	msg := mailmsg{
		From: from,
		To:   to,
		Kind: mailMsgKindRaw,
		RawData: rawData{
			Subject:   subject,
			Path:      path,
			Key:       key,
			Tag:       tag,
			Encrypted: encrypt,
		},
	}

	b, err := json.Marshal(msg)
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to encode mail event to json")
	}
	if err := s.events.StreamPublish(mailChannel, b); err != nil {
		return governor.ErrWithMsg(err, "Failed to publish mail event")
	}
	return nil
}

// FwdStream forwards an rfc5322 message
func (s *service) FwdStream(from string, to []string, size int64, body io.Reader, encrypt bool) error {
	if len(to) == 0 {
		return governor.ErrWithKind(nil, ErrInvalidMail{}, "Email must have at least one recipient")
	}

	u, err := uid.NewSnowflake(mailUIDRandSize)
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to generate mail body obj id")
	}
	path := u.Base32()

	contentType := mediaTypeTextPlain

	var key string
	var tag string
	var auth *hunter2.Poly1305Auth
	if encrypt {
		contentType = mediaTypeOctet
		config, err := hunter2.NewChaCha20Config()
		if err != nil {
			return governor.ErrWithMsg(err, "Failed to create mail data key")
		}
		key, err = s.maildataCipher.Encrypt(config.String())
		if err != nil {
			return governor.ErrWithMsg(err, "Failed to encrypt mail data key")
		}
		stream, err := hunter2.NewChaCha20Stream(*config)
		if err != nil {
			return governor.ErrWithMsg(err, "Failed to create encryption stream")
		}
		auth, err = hunter2.NewPoly1305Auth(*config)
		if err != nil {
			return governor.ErrWithMsg(err, "Failed to create encryption auth")
		}
		body = hunter2.NewEncStreamReader(stream, auth, body)
	}

	if err := s.sendMailDir.Put(path, contentType, size, nil, body); err != nil {
		return governor.ErrWithMsg(err, "Failed to save mail body")
	}
	if auth != nil {
		if err := auth.WriteCount(); err != nil {
			return governor.ErrWithMsg(err, "Failed to write auth content length")
		}
		tag = auth.String()
	}

	if from == "" {
		from = s.fromAddress
	}
	toAddrs := make([]Addr, 0, len(to))
	for _, i := range to {
		toAddrs = append(toAddrs, Addr{
			Address: i,
		})
	}
	msg := mailmsg{
		From: Addr{
			Address: from,
		},
		To:   toAddrs,
		Kind: mailMsgKindFwd,
		FwdData: fwdData{
			Path:      path,
			Key:       key,
			Tag:       tag,
			Encrypted: encrypt,
		},
	}

	b, err := json.Marshal(msg)
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to encode mail event to json")
	}
	if err := s.events.StreamPublish(mailChannel, b); err != nil {
		return governor.ErrWithMsg(err, "Failed to publish mail event")
	}
	return nil
}
