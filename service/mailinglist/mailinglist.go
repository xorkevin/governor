package mailinglist

import (
	"context"
	"net"
	"time"

	"github.com/emersion/go-smtp"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/mail"
	"xorkevin.dev/governor/service/mailinglist/model"
	"xorkevin.dev/governor/service/objstore"
	"xorkevin.dev/governor/service/user"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/service/user/org"
	"xorkevin.dev/governor/util/bytefmt"
	"xorkevin.dev/governor/util/dns"
	"xorkevin.dev/governor/util/rank"
)

const (
	eventStream         = "DEV_XORKEVIN_GOV_MAILINGLIST"
	eventStreamChannels = eventStream + ".>"
	mailChannel         = eventStream + ".mail"
	sendChannel         = eventStream + ".send"
	delChannel          = eventStream + ".del"
	mailWorker          = eventStream + "_WORKER"
	sendWorker          = eventStream + "_SEND_WORKER"
	delWorker           = eventStream + "_DEL_WORKER"
)

const (
	govworkerdelete    = "DEV_XORKEVIN_GOV_MAILINGLIST_WORKER_DELETE"
	govworkerorgdelete = "DEV_XORKEVIN_GOV_MAILINGLIST_WORKER_ORG_DELETE"
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
		events       events.Events
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
		streamsize   int64
		eventsize    int32
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
	ev := events.GetCtxEvents(inj)
	users := user.GetCtxUsers(inj)
	orgs := org.GetCtxOrgs(inj)
	g := gate.GetCtxGate(inj)
	mailer := mail.GetCtxMailer(inj)
	return New(lists, obj, ev, users, orgs, mailer, g)
}

// New creates a new MailingList service
func New(lists model.Repo, obj objstore.Bucket, ev events.Events, users user.Users, orgs org.Orgs, mailer mail.Mailer, g gate.Gate) Service {
	return &service{
		lists:      lists,
		mailBucket: obj,
		rcvMailDir: obj.Subdir("rcvmail"),
		events:     ev,
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
	r.SetDefault("streamsize", "200M")
	r.SetDefault("eventsize", "16K")
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

	s.server = s.createSMTPServer()
	go func() {
		if err := s.server.ListenAndServe(); err != nil {
			s.logger.Info("Shutting down mailing list SMTP server", map[string]string{
				"error": err.Error(),
			})
		}
	}()

	l.Info("Initialize mailing list", map[string]string{
		"port":                s.port,
		"authdomain":          r.GetStr("authdomain"),
		"usrdomain":           r.GetStr("usrdomain"),
		"orgdomain":           r.GetStr("orgdomain"),
		"maxmsgsize (bytes)":  r.GetStr("maxmsgsize"),
		"read timeout":        r.GetStr("readtimeout"),
		"write timeout":       r.GetStr("writetimeout"),
		"stream size (bytes)": r.GetStr("streamsize"),
		"event size (bytes)":  r.GetStr("eventsize"),
	})

	sr := s.router()
	sr.mountRoutes(m)
	l.Info("Mounted http routes", nil)

	return nil
}

func (s *service) createSMTPServer() *smtp.Server {
	be := &smtpBackend{
		service: s,
		logger: s.logger.WithData(map[string]string{
			"agent": "smtp_server",
		}),
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

	if _, err := s.events.StreamSubscribe(eventStream, sendChannel, sendWorker, s.sendSubscriber, events.StreamConsumerOpts{
		AckWait:     30 * time.Second,
		MaxDeliver:  30,
		MaxPending:  1024,
		MaxRequests: 32,
	}); err != nil {
		return governor.ErrWithMsg(err, "Failed to subscribe to mail send queue")
	}
	l.Info("Subscribed to send queue", nil)

	if _, err := s.events.StreamSubscribe(user.EventStream, user.DeleteChannel, govworkerdelete, s.UserDeleteHook, events.StreamConsumerOpts{
		AckWait:     15 * time.Second,
		MaxDeliver:  30,
		MaxPending:  1024,
		MaxRequests: 32,
	}); err != nil {
		return governor.ErrWithMsg(err, "Failed to subscribe to user delete queue")
	}
	l.Info("Subscribed to user delete queue", nil)

	if _, err := s.events.StreamSubscribe(org.EventStream, org.DeleteChannel, govworkerorgdelete, s.OrgDeleteHook, events.StreamConsumerOpts{
		AckWait:     15 * time.Second,
		MaxDeliver:  30,
		MaxPending:  1024,
		MaxRequests: 32,
	}); err != nil {
		return governor.ErrWithMsg(err, "Failed to subscribe to org delete queue")
	}
	l.Info("Subscribed to org delete queue", nil)

	if _, err := s.events.StreamSubscribe(eventStream, delChannel, delWorker, s.deleteSubscriber, events.StreamConsumerOpts{
		AckWait:     15 * time.Second,
		MaxDeliver:  30,
		MaxPending:  8192,
		MaxRequests: 32,
	}); err != nil {
		return governor.ErrWithMsg(err, "Failed to subscribe to list delete queue")
	}
	l.Info("Subscribed to list delete queue", nil)

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

const (
	listDeleteBatchSize = 256
)

// UserDeleteHook deletes the roles of a deleted user
func (s *service) UserDeleteHook(pinger events.Pinger, msgdata []byte) error {
	props, err := user.DecodeDeleteUserProps(msgdata)
	if err != nil {
		return err
	}
	return s.creatorDeleteHook(pinger, props.Userid)
}

// OrgDeleteHook deletes the roles of a deleted org
func (s *service) OrgDeleteHook(pinger events.Pinger, msgdata []byte) error {
	props, err := org.DecodeDeleteOrgProps(msgdata)
	if err != nil {
		return err
	}
	return s.creatorDeleteHook(pinger, rank.ToOrgName(props.OrgID))
}

// creatorDeleteHook deletes the roles of a deleted creator
func (s *service) creatorDeleteHook(pinger events.Pinger, creatorid string) error {
	for {
		if err := pinger.Ping(); err != nil {
			return err
		}
		lists, err := s.GetCreatorLists(creatorid, listDeleteBatchSize, 0)
		if err != nil {
			return governor.ErrWithMsg(err, "Failed to get user roles")
		}
		if len(lists.Lists) == 0 {
			break
		}
		for _, i := range lists.Lists {
			if err := s.DeleteList(i.CreatorID, i.Listname); err != nil {
				return governor.ErrWithMsg(err, "Failed to delete list")
			}
		}
		if len(lists.Lists) < listDeleteBatchSize {
			break
		}
	}
	return nil
}
