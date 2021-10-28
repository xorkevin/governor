package mailinglist

import (
	"context"
	"net"
	"time"

	"github.com/emersion/go-smtp"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/mail"
	"xorkevin.dev/governor/service/mailinglist/model"
	"xorkevin.dev/governor/service/objstore"
	"xorkevin.dev/governor/service/user"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/service/user/org"
	"xorkevin.dev/governor/util/bytefmt"
	"xorkevin.dev/governor/util/dns"
)

type (
	// MailingList is a mailing list service
	MailingList interface {
	}

	// Service is a MailingList and governor.Service
	Service interface {
		MailingList
		governor.Service
	}

	service struct {
		lists        model.Repo
		mailBucket   objstore.Bucket
		rcvMailDir   objstore.Dir
		users        user.Users
		orgs         org.Orgs
		mailer       mail.Mailer
		gate         gate.Gate
		logger       governor.Logger
		resolver     dns.Resolver
		server       *smtp.Server
		port         string
		authdomain   string
		usrdomain    string
		orgdomain    string
		maxmsgsize   int
		readtimeout  time.Duration
		writetimeout time.Duration
	}

	router struct {
		s service
	}

	ctxKeyMailingList struct{}
)

// GetCtxMailingList returns a MailingList service from the context
func GetCtxMailingList(inj governor.Injector) MailingList {
	v := inj.Get(ctxKeyMailingList{})
	if v == nil {
		return nil
	}
	return v.(MailingList)
}

// setCtxMailingList sets a MailingList service in the context
func setCtxMailingList(inj governor.Injector, m MailingList) {
	inj.Set(ctxKeyMailingList{}, m)
}

// NewCtx creates a new MailingList service from a context
func NewCtx(inj governor.Injector) Service {
	lists := model.GetCtxRepo(inj)
	obj := objstore.GetCtxBucket(inj)
	users := user.GetCtxUsers(inj)
	orgs := org.GetCtxOrgs(inj)
	g := gate.GetCtxGate(inj)
	mailer := mail.GetCtxMailer(inj)
	return New(lists, obj, users, orgs, mailer, g)
}

// New creates a new MailingList service
func New(lists model.Repo, obj objstore.Bucket, users user.Users, orgs org.Orgs, mailer mail.Mailer, g gate.Gate) Service {
	return &service{
		lists:      lists,
		mailBucket: obj,
		rcvMailDir: obj.Subdir("rcvmail"),
		users:      users,
		orgs:       orgs,
		mailer:     mailer,
		gate:       g,
		resolver: dns.NewCachingResolver(&net.Resolver{
			PreferGo: true,
		}, time.Minute),
	}
}

func (s *service) Register(inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	setCtxMailingList(inj, s)

	r.SetDefault("port", "2525")
	r.SetDefault("authdomain", "lists.mail.localhost")
	r.SetDefault("usrdomain", "lists.mail.localhost")
	r.SetDefault("orgdomain", "org.lists.mail.localhost")
	r.SetDefault("maxmsgsize", "2M")
	r.SetDefault("readtimeout", "5s")
	r.SetDefault("writetimeout", "5s")
	r.SetDefault("mockdnssource", "")
}

func (s *service) router() *router {
	return &router{
		s: *s,
	}
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, m governor.Router) error {
	s.logger = l
	l = s.logger.WithData(map[string]string{
		"phase": "init",
	})

	s.port = r.GetStr("port")
	s.authdomain = r.GetStr("authdomain")
	s.usrdomain = r.GetStr("usrdomain")
	s.orgdomain = r.GetStr("orgdomain")
	if limit, err := bytefmt.ToBytes(r.GetStr("maxmsgsize")); err != nil {
		return governor.ErrWithKind(err, governor.ErrInvalidConfig{}, "Invalid mail max message size")
	} else {
		s.maxmsgsize = int(limit)
	}
	if t, err := time.ParseDuration(r.GetStr("readtimeout")); err != nil {
		return governor.ErrWithKind(err, governor.ErrInvalidConfig{}, "Invalid read timeout for mail server")
	} else {
		s.readtimeout = t
	}
	if t, err := time.ParseDuration(r.GetStr("writetimeout")); err != nil {
		return governor.ErrWithKind(err, governor.ErrInvalidConfig{}, "Invalid write timeout for mail server")
	} else {
		s.writetimeout = t
	}

	if src := r.GetStr("mockdnssource"); src != "" {
		var err error
		s.resolver, err = dns.NewMockResolverFromFile(src)
		if err != nil {
			return governor.ErrWithKind(err, governor.ErrInvalidConfig{}, "Invalid mockdns source")
		}
		l.Info("Use mockdns", map[string]string{
			"source": src,
		})
	}

	s.server = s.createSMTPServer()
	go func() {
		if err := s.server.ListenAndServe(); err != nil {
			s.logger.Info("Shutting down mailing list SMTP server", map[string]string{
				"error": err.Error(),
			})
		}
	}()

	l.Info("Initialize mailing list", map[string]string{
		"port":               s.port,
		"authdomain":         r.GetStr("authdomain"),
		"usrdomain":          r.GetStr("usrdomain"),
		"orgdomain":          r.GetStr("orgdomain"),
		"maxmsgsize (bytes)": r.GetStr("maxmsgsize"),
		"read timeout":       r.GetStr("readtimeout"),
		"write timeout":      r.GetStr("writetimeout"),
	})

	sr := s.router()
	sr.mountRoutes(m)
	l.Info("Mounted http routes", nil)

	return nil
}

func (s *service) createSMTPServer() *smtp.Server {
	be := &smtpBackend{
		service: s,
	}
	server := smtp.NewServer(be)
	server.Addr = ":" + s.port
	server.Domain = s.authdomain
	server.MaxRecipients = 1
	server.MaxMessageBytes = s.maxmsgsize
	server.ReadTimeout = s.readtimeout
	server.WriteTimeout = s.writetimeout
	server.AuthDisabled = true
	return server
}

func (s *service) Setup(req governor.ReqSetup) error {
	l := s.logger.WithData(map[string]string{
		"phase": "setup",
	})
	if err := s.lists.Setup(); err != nil {
		return err
	}
	l.Info("Created mailing list tables", nil)
	if err := s.mailBucket.Init(); err != nil {
		return governor.ErrWithMsg(err, "Failed to init mail bucket")
	}
	l.Info("Created mail bucket", nil)
	return nil
}

func (s *service) PostSetup(req governor.ReqSetup) error {
	return nil
}

func (s *service) Start(ctx context.Context) error {
	return nil
}

func (s *service) Stop(ctx context.Context) {
	l := s.logger.WithData(map[string]string{
		"phase": "stop",
	})
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := s.server.Close(); err != nil {
			l.Error("Shutdown mailing list SMTP server error", map[string]string{
				"error": err.Error(),
			})
		}
	}()
	select {
	case <-done:
	case <-ctx.Done():
		l.Warn("Failed to stop", nil)
	}
}

func (s *service) Health() error {
	return nil
}
