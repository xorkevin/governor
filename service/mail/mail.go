package mail

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync/atomic"
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
	"xorkevin.dev/governor/util/kjson"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/hunter2"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

const (
	mailUIDRandSize = 8
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
		FwdStream(ctx context.Context, retpath string, to []string, size int64, body io.Reader, encrypt bool) error
		SendStream(ctx context.Context, retpath string, from Addr, to []Addr, subject string, size int64, body io.Reader, encrypt bool) error
		SendTpl(ctx context.Context, retpath string, from Addr, to []Addr, tpl Tpl, emdata interface{}, encrypt bool) error
	}

	// Tpl points to a mail template
	Tpl struct {
		Kind template.Kind `json:"kind"`
		Name string        `json:"name"`
	}

	tplData struct {
		MsgID     string `json:"msgid"`
		Tpl       Tpl    `json:"tpl"`
		Emdata    string `json:"emdata"`
		Encrypted bool   `json:"encrypted"`
	}

	rawData struct {
		MsgID     string `json:"msgid"`
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
		ReturnPath string  `json:"retpath"`
		From       Addr    `json:"from"`
		To         []Addr  `json:"to"`
		Kind       string  `json:"kind"`
		TplData    tplData `json:"tpl_data"`
		RawData    rawData `json:"raw_data"`
		FwdData    fwdData `json:"fwd_data"`
	}

	msgbuilder struct {
		headers  []string
		body     []byte
		htmlbody []byte
	}

	mailgcmsg struct {
		MsgPath string `json:"msgpath"`
	}

	maildataCipher struct {
		cipher    hunter2.Cipher
		decrypter *hunter2.Decrypter
	}

	getCipherRes struct {
		cipher *maildataCipher
		err    error
	}

	getOp struct {
		ctx context.Context
		res chan<- getCipherRes
	}

	getAuthRes struct {
		auth secretAuth
		err  error
	}

	getAuthOp struct {
		ctx context.Context
		res chan<- getAuthRes
	}

	Service struct {
		tpl             template.Template
		events          events.Events
		mailBucket      objstore.Bucket
		sendMailDir     objstore.Dir
		maildataCipher  *maildataCipher
		amaildataCipher *atomic.Pointer[maildataCipher]
		config          governor.SecretReader
		log             *klog.LevelLogger
		streamns        string
		opts            svcOpts
		host            string
		addr            string
		auth            secretAuth
		aauth           *atomic.Pointer[secretAuth]
		msgiddomain     string
		returnpath      string
		fromAddress     string
		fromName        string
		streamsize      int64
		eventsize       int32
		ops             chan getOp
		authops         chan getAuthOp
		ready           *atomic.Bool
		hbfailed        int
		hbinterval      time.Duration
		hbmaxfail       int
		done            <-chan struct{}
		authrefresh     time.Duration
	}

	ctxKeyMailer struct{}

	svcOpts struct {
		StreamName  string
		MailChannel string
		GCChannel   string
	}
)

// TplLocal is a local template source
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
func NewCtx(inj governor.Injector) *Service {
	tpl := template.GetCtxTemplate(inj)
	ev := events.GetCtxEvents(inj)
	obj := objstore.GetCtxBucket(inj)
	return New(tpl, ev, obj)
}

// New creates a new Mailer
func New(tpl template.Template, ev events.Events, obj objstore.Bucket) *Service {
	return &Service{
		tpl:             tpl,
		events:          ev,
		mailBucket:      obj,
		sendMailDir:     obj.Subdir("sendmail"),
		amaildataCipher: &atomic.Pointer[maildataCipher]{},
		aauth:           &atomic.Pointer[secretAuth]{},
		ops:             make(chan getOp),
		authops:         make(chan getAuthOp),
		ready:           &atomic.Bool{},
		hbfailed:        0,
	}
}

func (s *Service) Register(name string, inj governor.Injector, r governor.ConfigRegistrar) {
	setCtxMailer(inj, s)
	streamname := strings.ToUpper(name)
	s.streamns = streamname
	s.opts = svcOpts{
		StreamName:  streamname,
		MailChannel: streamname + ".mail",
		GCChannel:   streamname + ".gc",
	}

	r.SetDefault("auth", "")
	r.SetDefault("host", "localhost")
	r.SetDefault("port", "587")
	r.SetDefault("msgiddomain", "")
	r.SetDefault("fromaddress", "")
	r.SetDefault("fromname", "")
	r.SetDefault("streamsize", "200M")
	r.SetDefault("eventsize", "16K")
	r.SetDefault("hbinterval", "5s")
	r.SetDefault("hbmaxfail", 6)
	r.SetDefault("authrefresh", "1m")
}

type (
	secretMaildata struct {
		Keys []string `mapstructure:"secrets"`
	}
)

func (s *Service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, log klog.Logger, m governor.Router) error {
	s.log = klog.NewLevelLogger(log)
	s.config = r

	s.host = r.GetStr("host")
	s.addr = fmt.Sprintf("%s:%s", r.GetStr("host"), r.GetStr("port"))
	s.msgiddomain = r.GetStr("msgiddomain")
	s.returnpath = r.GetStr("returnpath")
	s.fromAddress = r.GetStr("fromaddress")
	s.fromName = r.GetStr("fromname")
	var err error
	s.streamsize, err = bytefmt.ToBytes(r.GetStr("streamsize"))
	if err != nil {
		return kerrors.WithMsg(err, "Invalid stream size")
	}
	eventsize, err := bytefmt.ToBytes(r.GetStr("eventsize"))
	if err != nil {
		return kerrors.WithMsg(err, "Invalid msg size")
	}
	s.eventsize = int32(eventsize)
	s.hbinterval, err = r.GetDuration("hbinterval")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse hbinterval")
	}
	s.hbmaxfail = r.GetInt("hbmaxfail")
	s.authrefresh, err = r.GetDuration("authrefresh")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse authrefresh")
	}

	s.log.Info(ctx, "Initialize mail service", klog.Fields{
		"mail.smtp.addr":      s.addr,
		"mail.msgiddomain":    s.msgiddomain,
		"mail.returnpath":     s.returnpath,
		"mail.sender.address": s.fromAddress,
		"mail.sender.name":    s.fromName,
		"mail.streamsize":     r.GetStr("streamsize"),
		"mail.eventsize":      r.GetStr("eventsize"),
		"mail.hbinterval":     s.hbinterval.String(),
		"mail.hbmaxfail":      s.hbmaxfail,
		"mail.authrefresh":    s.authrefresh.String(),
	})

	ctx = klog.WithFields(ctx, klog.Fields{
		"gov.service.phase": "run",
	})

	done := make(chan struct{})
	go s.execute(ctx, done)
	s.done = done

	return nil
}

func (s *Service) execute(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(s.hbinterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.handlePing(ctx)
		case op := <-s.ops:
			cipher, err := s.handleGetCipher(ctx)
			select {
			case <-op.ctx.Done():
			case op.res <- getCipherRes{
				cipher: cipher,
				err:    err,
			}:
				close(op.res)
			}
		case op := <-s.authops:
			auth, err := s.handleGetAuth(ctx)
			select {
			case <-op.ctx.Done():
			case op.res <- getAuthRes{
				auth: auth,
				err:  err,
			}:
				close(op.res)
			}
		}
	}
}

func (s *Service) handlePing(ctx context.Context) {
	if err := s.refreshAuth(ctx); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to refresh mail auth"), nil)
	}

	err := s.refreshSecrets(ctx)
	if err == nil {
		s.ready.Store(true)
		s.hbfailed = 0
		return
	}
	s.hbfailed++
	if s.hbfailed < s.hbmaxfail {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to refresh mail keys"), nil)
		return
	}
	s.log.Err(ctx, kerrors.WithMsg(err, "Failed max refresh attempts"), nil)
	s.ready.Store(false)
	s.hbfailed = 0
}

func (s *Service) refreshAuth(ctx context.Context) error {
	var auth secretAuth
	if err := s.config.GetSecret(ctx, "auth", s.authrefresh, &auth); err != nil {
		return kerrors.WithMsg(err, "Invalid mail auth")
	}
	if auth.Username == "" {
		return kerrors.WithKind(nil, governor.ErrorInvalidConfig{}, "Empty mail auth")
	}
	if auth != s.auth {
		s.auth = auth
		s.aauth.Store(&auth)
		s.log.Info(ctx, "Refreshed smtp auth", klog.Fields{
			"mail.smtp.username": auth.Username,
		})
	}
	return nil
}

func (s *Service) handleGetAuth(ctx context.Context) (secretAuth, error) {
	if s.auth.Username == "" {
		if err := s.refreshAuth(ctx); err != nil {
			return secretAuth{}, err
		}
	}
	return s.auth, nil
}

func (s *Service) getAuth(ctx context.Context) (secretAuth, error) {
	if auth := s.aauth.Load(); auth != nil {
		return *auth, nil
	}

	res := make(chan getAuthRes)
	op := getAuthOp{
		ctx: ctx,
		res: res,
	}
	select {
	case <-s.done:
		return secretAuth{}, kerrors.WithMsg(nil, "Mail service shutdown")
	case <-ctx.Done():
		return secretAuth{}, kerrors.WithMsg(ctx.Err(), "Context cancelled")
	case s.authops <- op:
		select {
		case <-ctx.Done():
			return secretAuth{}, kerrors.WithMsg(ctx.Err(), "Context cancelled")
		case v := <-res:
			return v.auth, v.err
		}
	}
}

func (s *Service) refreshSecrets(ctx context.Context) error {
	var maildataSecrets secretMaildata
	if err := s.config.GetSecret(ctx, "mailkey", s.authrefresh, &maildataSecrets); err != nil {
		return kerrors.WithKind(err, governor.ErrorInvalidConfig{}, "Invalid mailkey secrets")
	}
	if len(maildataSecrets.Keys) == 0 {
		return kerrors.WithKind(nil, governor.ErrorInvalidConfig{}, "No mailkey present")
	}
	decrypter := hunter2.NewDecrypter()
	var cipher hunter2.Cipher
	for n, i := range maildataSecrets.Keys {
		c, err := hunter2.CipherFromParams(i, hunter2.DefaultCipherAlgs)
		if err != nil {
			return kerrors.WithKind(err, governor.ErrorInvalidConfig{}, "Invalid cipher param")
		}
		if n == 0 {
			if s.maildataCipher != nil && s.maildataCipher.cipher.ID() == c.ID() {
				// first, newest cipher matches current cipher, therefore no change in ciphers
				return nil
			}
			cipher = c
		}
		decrypter.RegisterCipher(c)
	}
	s.maildataCipher = &maildataCipher{
		cipher:    cipher,
		decrypter: decrypter,
	}
	s.amaildataCipher.Store(s.maildataCipher)
	s.log.Info(ctx, "Refreshed mailkey with new keys", klog.Fields{
		"mail.datakey.kid":  s.maildataCipher.cipher.ID(),
		"mail.datakeys.num": strconv.Itoa(decrypter.Size()),
	})
	return nil
}

func (s *Service) handleGetCipher(ctx context.Context) (*maildataCipher, error) {
	if s.maildataCipher == nil {
		if err := s.refreshSecrets(ctx); err != nil {
			return nil, err
		}
		s.ready.Store(true)
	}
	return s.maildataCipher, nil
}

func (s *Service) getCipher(ctx context.Context) (*maildataCipher, error) {
	if cipher := s.amaildataCipher.Load(); cipher != nil {
		return cipher, nil
	}

	res := make(chan getCipherRes)
	op := getOp{
		ctx: ctx,
		res: res,
	}
	select {
	case <-s.done:
		return nil, kerrors.WithMsg(nil, "Mail service shutdown")
	case <-ctx.Done():
		return nil, kerrors.WithMsg(ctx.Err(), "Context cancelled")
	case s.ops <- op:
		select {
		case <-ctx.Done():
			return nil, kerrors.WithMsg(ctx.Err(), "Context cancelled")
		case v := <-res:
			return v.cipher, v.err
		}
	}
}

func (s *Service) Start(ctx context.Context) error {
	if _, err := s.events.StreamSubscribe(s.opts.StreamName, s.opts.MailChannel, s.streamns+"_WORKER", s.mailSubscriber, events.StreamConsumerOpts{
		AckWait:    30 * time.Second,
		MaxDeliver: 30,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to mail queue")
	}
	s.log.Info(ctx, "Subscribed to mail queue", nil)

	if _, err := s.events.StreamSubscribe(s.opts.StreamName, s.opts.GCChannel, s.streamns+"_WORKER_GC", s.gcSubscriber, events.StreamConsumerOpts{
		AckWait:    15 * time.Second,
		MaxDeliver: 30,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to mail gc queue")
	}
	s.log.Info(ctx, "Subscribed to mail gc queue", nil)

	return nil
}

func (s *Service) Stop(ctx context.Context) {
	select {
	case <-s.done:
		return
	case <-ctx.Done():
		s.log.WarnErr(ctx, kerrors.WithMsg(ctx.Err(), "Failed to stop"), nil)
	}
}

func (s *Service) Setup(ctx context.Context, req governor.ReqSetup) error {
	if err := s.mailBucket.Init(ctx); err != nil {
		return kerrors.WithMsg(err, "Failed to init mail bucket")
	}
	s.log.Info(ctx, "Created mail bucket", nil)
	if err := s.events.InitStream(ctx, s.opts.StreamName, []string{s.opts.StreamName + ".>"}, events.StreamOpts{
		Replicas:   1,
		MaxAge:     30 * 24 * time.Hour,
		MaxBytes:   s.streamsize,
		MaxMsgSize: s.eventsize,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to init mail stream")
	}
	s.log.Info(ctx, "Created mail stream", nil)
	return nil
}

func (s *Service) Health(ctx context.Context) error {
	if !s.ready.Load() {
		return kerrors.WithKind(nil, governor.ErrorInvalidConfig{}, "Mail service not ready")
	}
	return nil
}

type (
	secretAuth struct {
		Username string `mapstructure:"username"`
		Password string `mapstructure:"password"`
	}
)

func (s *Service) handleSendMail(ctx context.Context, from string, to []string, msg io.Reader) error {
	secret, err := s.getAuth(ctx)
	if err != nil {
		return err
	}

	auth := sasl.NewPlainClient("", secret.Username, secret.Password)
	if err := smtp.SendMail(s.addr, auth, from, to, msg); err != nil {
		return kerrors.WithKind(err, ErrorSendMail{}, "Failed to send mail")
	}
	s.log.Info(ctx, "Mail sent", klog.Fields{
		"mail.smtp.addr":     s.addr,
		"mail.smtp.username": secret.Username,
	})
	s.log.DebugF(ctx, func() (string, klog.Fields) {
		return "Mail sent", klog.Fields{
			"mail.smtp.addr":     s.addr,
			"mail.smtp.username": secret.Username,
			"mail.from":          from,
			"mail.to":            to,
		}
	})
	return nil
}

type (
	// ErrorMailEvent is returned when the msgqueue mail message is malformed
	ErrorMailEvent struct{}
	// ErrorInvalidMail is returned when the mail message is invalid
	ErrorInvalidMail struct{}
	// ErrorBuildMail is returned when failing to build an email message
	ErrorBuildMail struct{}
	// ErrorSendMail is returned when failing to send an email message
	ErrorSendMail struct{}
)

func (e ErrorMailEvent) Error() string {
	return "Malformed mail message"
}

func (e ErrorInvalidMail) Error() string {
	return "Invalid mail"
}

func (e ErrorBuildMail) Error() string {
	return "Error building email"
}

func (e ErrorSendMail) Error() string {
	return "Error sending email"
}

func (s *Service) mailSubscriber(ctx context.Context, pinger events.Pinger, topic string, msgdata []byte) error {
	var emmsg mailmsg
	if err := kjson.Unmarshal(msgdata, &emmsg); err != nil {
		return kerrors.WithKind(err, ErrorMailEvent{}, "Failed to decode mail message")
	}

	var msg io.Reader

	var tag string
	var auth *hunter2.Poly1305Auth

	msgpath := ""

	if emmsg.Kind == mailMsgKindFwd {
		data := emmsg.FwdData
		ctx = klog.WithFields(ctx, klog.Fields{
			"mail.msg.kind":      "fwd",
			"mail.msg.data.path": data.Path,
		})
		s.log.Info(ctx, "Received mail msg to send", nil)
		b1, _, err := s.sendMailDir.Get(ctx, data.Path)
		if err != nil {
			if errors.Is(err, objstore.ErrorNotFound{}) {
				s.log.Err(ctx, kerrors.WithMsg(err, "Mail body content not found"), nil)
				return nil
			}
			return kerrors.WithKind(err, ErrorMailEvent{}, "Failed to get mail body")
		}
		defer func() {
			if err := b1.Close(); err != nil {
				s.log.Err(ctx, kerrors.WithMsg(err, "Failed to close mail body"), nil)
			}
		}()
		msgpath = data.Path
		msg = b1
		if data.Encrypted {
			cipher, err := s.getCipher(ctx)
			if err != nil {
				return err
			}
			data.Key, err = cipher.decrypter.Decrypt(data.Key)
			if err != nil {
				return kerrors.WithKind(err, ErrorMailEvent{}, "Failed to decrypt mail data key")
			}
			config, err := hunter2.ParseChaCha20Config(data.Key)
			if err != nil {
				return kerrors.WithKind(err, ErrorMailEvent{}, "Failed to parse mail data key")
			}
			stream, err := hunter2.NewChaCha20Stream(*config)
			if err != nil {
				return kerrors.WithMsg(err, "Failed to create decryption stream")
			}
			auth, err = hunter2.NewPoly1305Auth(*config)
			if err != nil {
				return kerrors.WithMsg(err, "Failed to create decryption auth")
			}
			tag = data.Tag
			msg = hunter2.NewDecStreamReader(stream, auth, msg)
		}
	} else {
		var msgid string
		var subject, body, htmlbody io.Reader
		if emmsg.Kind == mailMsgKindRaw {
			data := emmsg.RawData
			msgid = data.MsgID
			ctx = klog.WithFields(ctx, klog.Fields{
				"mail.msg.kind":      "raw",
				"mail.msg.data.path": data.Path,
				"mail.msg.id":        msgid,
			})
			s.log.Info(ctx, "Received mail msg to send", nil)
			b1, _, err := s.sendMailDir.Get(ctx, data.Path)
			if err != nil {
				if errors.Is(err, objstore.ErrorNotFound{}) {
					s.log.Err(ctx, kerrors.WithMsg(err, "Mail body content not found"), nil)
					return nil
				}
				return kerrors.WithKind(err, ErrorMailEvent{}, "Failed to get mail body")
			}
			defer func() {
				if err := b1.Close(); err != nil {
					s.log.Err(ctx, kerrors.WithMsg(err, "Failed to close mail body"), nil)
				}
			}()
			msgpath = data.Path
			body = b1
			if data.Encrypted {
				cipher, err := s.getCipher(ctx)
				if err != nil {
					return err
				}
				data.Subject, err = cipher.decrypter.Decrypt(data.Subject)
				if err != nil {
					return kerrors.WithKind(err, ErrorMailEvent{}, "Failed to decrypt mail subject")
				}
				data.Key, err = cipher.decrypter.Decrypt(data.Key)
				if err != nil {
					return kerrors.WithKind(err, ErrorMailEvent{}, "Failed to decrypt mail data key")
				}
				config, err := hunter2.ParseChaCha20Config(data.Key)
				if err != nil {
					return kerrors.WithKind(err, ErrorMailEvent{}, "Failed to parse mail data key")
				}
				stream, err := hunter2.NewChaCha20Stream(*config)
				if err != nil {
					return kerrors.WithMsg(err, "Failed to create decryption stream")
				}
				auth, err = hunter2.NewPoly1305Auth(*config)
				if err != nil {
					return kerrors.WithMsg(err, "Failed to create decryption auth")
				}
				tag = data.Tag
				body = hunter2.NewDecStreamReader(stream, auth, body)
			}
			subject = strings.NewReader(data.Subject)
		} else if emmsg.Kind == mailMsgKindTpl {
			data := emmsg.TplData
			msgid = data.MsgID
			ctx = klog.WithFields(ctx, klog.Fields{
				"mail.msg.kind": "tpl",
				"mail.msg.id":   msgid,
			})
			s.log.Info(ctx, "Received mail msg to send", nil)
			if data.Encrypted {
				cipher, err := s.getCipher(ctx)
				if err != nil {
					return err
				}
				data.Emdata, err = cipher.decrypter.Decrypt(data.Emdata)
				if err != nil {
					return kerrors.WithKind(err, ErrorMailEvent{}, "Failed to decrypt mail data")
				}
			}
			emdata := map[string]string{}
			if err := kjson.Unmarshal([]byte(data.Emdata), &emdata); err != nil {
				return kerrors.WithKind(err, ErrorMailEvent{}, "Failed to decode mail data")
			}

			s1 := &bytes.Buffer{}
			b1 := &bytes.Buffer{}
			b2 := &bytes.Buffer{}
			if err := s.tpl.Execute(s1, data.Tpl.Kind, data.Tpl.Name+"_subject.txt", emdata); err != nil {
				return kerrors.WithMsg(err, "Failed to execute mail subject template")
			}
			subject = s1
			if err := s.tpl.Execute(b1, data.Tpl.Kind, data.Tpl.Name+".txt", emdata); err != nil {
				return kerrors.WithMsg(err, "Failed to execute mail body template")
			}
			body = b1
			if err := s.tpl.ExecuteHTML(b2, data.Tpl.Kind, data.Tpl.Name+".html", emdata); err != nil {
				if !errors.Is(err, template.ErrorTemplateDNE{}) {
					s.log.Err(ctx, kerrors.WithMsg(err, "Failed to execute mail html body template"), klog.Fields{
						"mail.tpl.body": data.Tpl.Name + ".html",
					})
				}
				htmlbody = nil
			} else {
				htmlbody = b2
			}
		} else {
			return kerrors.WithKind(nil, ErrorMailEvent{}, "Invalid mail message kind")
		}

		b := &bytes.Buffer{}
		if err := msgToBytes(s.log, ctx, msgid, emmsg.From, emmsg.To, subject, body, htmlbody, b); err != nil {
			return err
		}
		msg = b
	}

	if auth != nil {
		if err := auth.WriteCount(); err != nil {
			return kerrors.WithMsg(err, "Failed to write auth content length")
		}
		if err := auth.Auth(tag); err != nil {
			return kerrors.WithKind(err, ErrorMailEvent{}, "Mail body failed authentication")
		}
	}

	var gcpath []byte
	if msgpath != "" {
		b, err := kjson.Marshal(mailgcmsg{
			MsgPath: msgpath,
		})
		if err != nil {
			return kerrors.WithMsg(err, "Failed to encode mail gc event to json")
		}
		gcpath = b
	}

	to := make([]string, 0, len(emmsg.To))
	for _, i := range emmsg.To {
		to = append(to, i.Address)
	}
	if err := s.handleSendMail(ctx, emmsg.ReturnPath, to, msg); err != nil {
		return err
	}

	if len(gcpath) != 0 {
		if err := s.events.StreamPublish(ctx, s.opts.GCChannel, gcpath); err != nil {
			return kerrors.WithMsg(err, "Failed to publish mail gc event")
		}
	}
	return nil
}

func msgToBytes(log *klog.LevelLogger, ctx context.Context, msgid string, from Addr, to []Addr, subject, body, htmlbody io.Reader, dst io.Writer) error {
	h := emmail.Header{}
	h.SetMessageID(msgid)
	h.SetDate(time.Now().Round(0).UTC())
	subj := strings.Builder{}
	if _, err := io.Copy(&subj, subject); err != nil {
		return kerrors.WithKind(err, ErrorBuildMail{}, "Failed to write mail subject")
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
			return kerrors.WithKind(err, ErrorBuildMail{}, "Failed to create plain mail writer")
		}
		defer func() {
			if err := mw.Close(); err != nil {
				log.Err(ctx, kerrors.WithMsg(err, "Failed closing plain mail writer"), nil)
			}
		}()
		if _, err := io.Copy(mw, body); err != nil {
			return kerrors.WithKind(err, ErrorBuildMail{}, "Failed to write plaintext mail body")
		}
		return nil
	}

	mw, err := emmail.CreateWriter(dst, h)
	if err != nil {
		return kerrors.WithKind(err, ErrorBuildMail{}, "Failed to create mail writer")
	}
	defer func() {
		if err := mw.Close(); err != nil {
			log.Err(ctx, kerrors.WithMsg(err, "Failed closing mail writer"), nil)
		}
	}()

	bw, err := mw.CreateInline()
	if err != nil {
		return kerrors.WithKind(err, ErrorBuildMail{}, "Failed to create mail body writer")
	}
	defer func() {
		if err := bw.Close(); err != nil {
			log.Err(ctx, kerrors.WithMsg(err, "Failed closing mail body writer"), nil)
		}
	}()

	if err := func() error {
		hh := emmail.InlineHeader{}
		hh.SetContentType(mediaTypeTextPlain, map[string]string{"charset": "utf-8"})
		pw, err := bw.CreatePart(hh)
		if err != nil {
			return kerrors.WithKind(err, ErrorBuildMail{}, "Failed to create mail body plaintext writer")
		}
		defer func() {
			if err := pw.Close(); err != nil {
				log.Err(ctx, kerrors.WithMsg(err, "Failed closing mail body plaintext writer"), nil)
			}
		}()
		if _, err := io.Copy(pw, body); err != nil {
			return kerrors.WithKind(err, ErrorBuildMail{}, "Failed to write plaintext mail body")
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
			return kerrors.WithKind(err, ErrorBuildMail{}, "Failed to create mail body html writer")
		}
		defer func() {
			if err := pw.Close(); err != nil {
				log.Err(ctx, kerrors.WithMsg(err, "Failed closing mail body plaintext writer"), nil)
			}
		}()
		if _, err := io.Copy(pw, htmlbody); err != nil {
			return kerrors.WithKind(err, ErrorBuildMail{}, "Failed to write html mail body")
		}
		return nil
	}(); err != nil {
		return err
	}
	return nil
}

func genMsgID(msgiddomain string) (string, error) {
	u, err := uid.NewSnowflake(mailUIDRandSize)
	if err != nil {
		return "", kerrors.WithMsg(err, "Failed to generate mail msg id")
	}
	return fmt.Sprintf("%s@%s", u.Base32(), msgiddomain), nil
}

func (s *Service) gcSubscriber(ctx context.Context, pinger events.Pinger, topic string, msgdata []byte) error {
	var gcmsg mailgcmsg
	if err := kjson.Unmarshal(msgdata, &gcmsg); err != nil {
		return kerrors.WithKind(err, ErrorMailEvent{}, "Failed to decode mail gc message")
	}
	if err := s.sendMailDir.Del(ctx, gcmsg.MsgPath); err != nil {
		if !errors.Is(err, objstore.ErrorNotFound{}) {
			return kerrors.WithMsg(err, "Failed to delete mail body")
		}
	}
	return nil
}

// SendTpl creates and sends a message given a template and data
func (s *Service) SendTpl(ctx context.Context, retpath string, from Addr, to []Addr, tpl Tpl, emdata interface{}, encrypt bool) error {
	if len(to) == 0 {
		return kerrors.WithKind(nil, ErrorInvalidMail{}, "Email must have at least one recipient")
	}

	msgid, err := genMsgID(s.msgiddomain)
	if err != nil {
		return err
	}

	databytes, err := kjson.Marshal(emdata)
	if err != nil {
		return kerrors.WithKind(err, ErrorInvalidMail{}, "Failed to encode email data to JSON")
	}
	datastring := string(databytes)
	if encrypt {
		cipher, err := s.getCipher(ctx)
		if err != nil {
			return err
		}
		datastring, err = cipher.cipher.Encrypt(datastring)
		if err != nil {
			return kerrors.WithMsg(err, "Failed to encrypt mail data")
		}
	}

	if retpath == "" {
		retpath = s.returnpath
	}
	if from.Address == "" {
		from.Address = s.fromAddress
	}
	if from.Name == "" {
		from.Name = s.fromName
	}
	msg := mailmsg{
		ReturnPath: retpath,
		From:       from,
		To:         to,
		Kind:       mailMsgKindTpl,
		TplData: tplData{
			MsgID:     msgid,
			Tpl:       tpl,
			Emdata:    datastring,
			Encrypted: encrypt,
		},
	}

	b, err := kjson.Marshal(msg)
	if err != nil {
		return kerrors.WithMsg(err, "Failed to encode mail event to json")
	}
	if err := s.events.StreamPublish(ctx, s.opts.MailChannel, b); err != nil {
		return kerrors.WithMsg(err, "Failed to publish mail event")
	}
	return nil
}

// SendStream creates and sends a message from a given body
func (s *Service) SendStream(ctx context.Context, retpath string, from Addr, to []Addr, subject string, size int64, body io.Reader, encrypt bool) error {
	if len(to) == 0 {
		return kerrors.WithKind(nil, ErrorInvalidMail{}, "Email must have at least one recipient")
	}

	msgid, err := genMsgID(s.msgiddomain)
	if err != nil {
		return err
	}

	u, err := uid.NewSnowflake(mailUIDRandSize)
	if err != nil {
		return kerrors.WithMsg(err, "Failed to generate mail body obj id")
	}
	path := u.Base32()

	contentType := mediaTypeTextPlain

	var key string
	var tag string
	var auth *hunter2.Poly1305Auth
	if encrypt {
		cipher, err := s.getCipher(ctx)
		if err != nil {
			return err
		}
		subject, err = cipher.cipher.Encrypt(subject)
		if err != nil {
			return kerrors.WithMsg(err, "Failed to encrypt mail subject")
		}

		contentType = mediaTypeOctet
		config, err := hunter2.NewChaCha20Config()
		if err != nil {
			return kerrors.WithMsg(err, "Failed to create mail data key")
		}
		key, err = cipher.cipher.Encrypt(config.String())
		if err != nil {
			return kerrors.WithMsg(err, "Failed to encrypt mail data key")
		}
		stream, err := hunter2.NewChaCha20Stream(*config)
		if err != nil {
			return kerrors.WithMsg(err, "Failed to create encryption stream")
		}
		auth, err = hunter2.NewPoly1305Auth(*config)
		if err != nil {
			return kerrors.WithMsg(err, "Failed to create encryption auth")
		}
		body = hunter2.NewEncStreamReader(stream, auth, body)
	}

	if err := s.sendMailDir.Put(ctx, path, contentType, size, nil, body); err != nil {
		return kerrors.WithMsg(err, "Failed to save mail body")
	}
	if auth != nil {
		if err := auth.WriteCount(); err != nil {
			return kerrors.WithMsg(err, "Failed to write auth content length")
		}
		tag = auth.String()
	}

	if retpath == "" {
		retpath = s.returnpath
	}
	if from.Address == "" {
		from.Address = s.fromAddress
	}
	if from.Name == "" {
		from.Name = s.fromName
	}
	msg := mailmsg{
		ReturnPath: retpath,
		From:       from,
		To:         to,
		Kind:       mailMsgKindRaw,
		RawData: rawData{
			MsgID:     msgid,
			Subject:   subject,
			Path:      path,
			Key:       key,
			Tag:       tag,
			Encrypted: encrypt,
		},
	}

	b, err := kjson.Marshal(msg)
	if err != nil {
		return kerrors.WithMsg(err, "Failed to encode mail event to json")
	}
	if err := s.events.StreamPublish(ctx, s.opts.MailChannel, b); err != nil {
		return kerrors.WithMsg(err, "Failed to publish mail event")
	}
	return nil
}

// FwdStream forwards an rfc5322 message
func (s *Service) FwdStream(ctx context.Context, retpath string, to []string, size int64, body io.Reader, encrypt bool) error {
	if len(to) == 0 {
		return kerrors.WithKind(nil, ErrorInvalidMail{}, "Email must have at least one recipient")
	}

	u, err := uid.NewSnowflake(mailUIDRandSize)
	if err != nil {
		return kerrors.WithMsg(err, "Failed to generate mail body obj id")
	}
	path := u.Base32()

	contentType := mediaTypeTextPlain

	var key string
	var tag string
	var auth *hunter2.Poly1305Auth
	if encrypt {
		cipher, err := s.getCipher(ctx)
		if err != nil {
			return err
		}
		contentType = mediaTypeOctet
		config, err := hunter2.NewChaCha20Config()
		if err != nil {
			return kerrors.WithMsg(err, "Failed to create mail data key")
		}
		key, err = cipher.cipher.Encrypt(config.String())
		if err != nil {
			return kerrors.WithMsg(err, "Failed to encrypt mail data key")
		}
		stream, err := hunter2.NewChaCha20Stream(*config)
		if err != nil {
			return kerrors.WithMsg(err, "Failed to create encryption stream")
		}
		auth, err = hunter2.NewPoly1305Auth(*config)
		if err != nil {
			return kerrors.WithMsg(err, "Failed to create encryption auth")
		}
		body = hunter2.NewEncStreamReader(stream, auth, body)
	}

	if err := s.sendMailDir.Put(ctx, path, contentType, size, nil, body); err != nil {
		return kerrors.WithMsg(err, "Failed to save mail body")
	}
	if auth != nil {
		if err := auth.WriteCount(); err != nil {
			return kerrors.WithMsg(err, "Failed to write auth content length")
		}
		tag = auth.String()
	}

	if retpath == "" {
		retpath = s.returnpath
	}
	toAddrs := make([]Addr, 0, len(to))
	for _, i := range to {
		toAddrs = append(toAddrs, Addr{
			Address: i,
		})
	}
	msg := mailmsg{
		ReturnPath: retpath,
		To:         toAddrs,
		Kind:       mailMsgKindFwd,
		FwdData: fwdData{
			Path:      path,
			Key:       key,
			Tag:       tag,
			Encrypted: encrypt,
		},
	}

	b, err := kjson.Marshal(msg)
	if err != nil {
		return kerrors.WithMsg(err, "Failed to encode mail event to json")
	}
	if err := s.events.StreamPublish(ctx, s.opts.MailChannel, b); err != nil {
		return kerrors.WithMsg(err, "Failed to publish mail event")
	}
	return nil
}
