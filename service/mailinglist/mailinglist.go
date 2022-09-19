package mailinglist

import (
	"context"
	"net"
	"strings"
	"time"

	"github.com/emersion/go-smtp"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/mail"
	"xorkevin.dev/governor/service/mailinglist/model"
	"xorkevin.dev/governor/service/objstore"
	"xorkevin.dev/governor/service/ratelimit"
	"xorkevin.dev/governor/service/user"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/service/user/org"
	"xorkevin.dev/governor/util/bytefmt"
	"xorkevin.dev/governor/util/dns"
	"xorkevin.dev/governor/util/rank"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
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
		ratelimiter  ratelimit.Ratelimiter
		gate         gate.Gate
		log          *klog.LevelLogger
		scopens      string
		streamns     string
		opts         svcOpts
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
		s  *service
		rt governor.MiddlewareCtx
	}

	ctxKeyMailingList struct{}

	svcOpts struct {
		StreamName  string
		MailChannel string
		SendChannel string
		DelChannel  string
	}
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
	ratelimiter := ratelimit.GetCtxRatelimiter(inj)
	g := gate.GetCtxGate(inj)
	mailer := mail.GetCtxMailer(inj)
	return New(lists, obj, ev, users, orgs, mailer, ratelimiter, g)
}

// New creates a new MailingList service
func New(lists model.Repo, obj objstore.Bucket, ev events.Events, users user.Users, orgs org.Orgs, mailer mail.Mailer, ratelimiter ratelimit.Ratelimiter, g gate.Gate) Service {
	return &service{
		lists:       lists,
		mailBucket:  obj,
		rcvMailDir:  obj.Subdir("rcvmail"),
		events:      ev,
		users:       users,
		orgs:        orgs,
		mailer:      mailer,
		ratelimiter: ratelimiter,
		gate:        g,
		resolver: dns.NewCachingResolver(&net.Resolver{
			PreferGo: true,
		}, time.Minute),
	}
}

func (s *service) Register(name string, inj governor.Injector, r governor.ConfigRegistrar) {
	setCtxMailingList(inj, s)
	s.scopens = "gov." + name
	streamname := strings.ToUpper(name)
	s.streamns = streamname
	s.opts = svcOpts{
		StreamName:  streamname,
		MailChannel: streamname + ".mail",
		SendChannel: streamname + ".send",
		DelChannel:  streamname + ".del",
	}

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
		s:  s,
		rt: s.ratelimiter.BaseCtx(),
	}
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, log klog.Logger, m governor.Router) error {
	s.log = klog.NewLevelLogger(log)

	s.port = r.GetStr("port")
	s.authdomain = r.GetStr("authdomain")
	s.usrdomain = r.GetStr("usrdomain")
	s.orgdomain = r.GetStr("orgdomain")
	if limit, err := bytefmt.ToBytes(r.GetStr("maxmsgsize")); err != nil {
		return kerrors.WithKind(err, governor.ErrorInvalidConfig{}, "Invalid mail max message size")
	} else {
		s.maxmsgsize = int(limit)
	}
	if t, err := time.ParseDuration(r.GetStr("readtimeout")); err != nil {
		return kerrors.WithKind(err, governor.ErrorInvalidConfig{}, "Invalid read timeout for mail server")
	} else {
		s.readtimeout = t
	}
	if t, err := time.ParseDuration(r.GetStr("writetimeout")); err != nil {
		return kerrors.WithKind(err, governor.ErrorInvalidConfig{}, "Invalid write timeout for mail server")
	} else {
		s.writetimeout = t
	}

	if src := r.GetStr("mockdnssource"); src != "" {
		var err error
		s.resolver, err = dns.NewMockResolverFromFile(src)
		if err != nil {
			return kerrors.WithKind(err, governor.ErrorInvalidConfig{}, "Invalid mockdns source")
		}
		s.log.Info(ctx, "Use mockdns", klog.Fields{
			"mailinglist.mockdns.source": src,
		})
	}

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

	s.log.Info(ctx, "Loaded config", klog.Fields{
		"mailinglist.smtp.port":    s.port,
		"mailinglist.authdomain":   s.authdomain,
		"mailinglist.usrdomain":    s.usrdomain,
		"mailinglist.orgdomain":    s.orgdomain,
		"mailinglist.maxmsgsize":   r.GetStr("maxmsgsize"),
		"mailinglist.readtimeout":  r.GetStr("readtimeout"),
		"mailinglist.writetimeout": r.GetStr("writetimeout"),
		"mailinglist.stream.size":  r.GetStr("streamsize"),
		"mailinglist.event.size":   r.GetStr("eventsize"),
	})

	ctx = klog.WithFields(ctx, klog.Fields{
		"gov.service.phase": "run",
	})

	s.server = s.createSMTPServer()
	go func() {
		if err := s.server.ListenAndServe(); err != nil {
			s.log.Err(ctx, kerrors.WithMsg(err, "Shutting down mailinglist SMTP server"), nil)
		}
	}()

	sr := s.router()
	sr.mountRoutes(m)
	s.log.Info(ctx, "Mounted http routes", nil)

	return nil
}

func (s *service) createSMTPServer() *smtp.Server {
	be := &smtpBackend{
		service: s,
		log:     klog.NewLevelLogger(s.log.Logger.Sublogger("smtpserver", nil)),
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

func (s *service) Start(ctx context.Context) error {
	if _, err := s.events.StreamSubscribe(s.opts.StreamName, s.opts.MailChannel, s.streamns+"_WORKER", s.mailSubscriber, events.StreamConsumerOpts{
		AckWait:     30 * time.Second,
		MaxDeliver:  30,
		MaxPending:  1024,
		MaxRequests: 32,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to mail queue")
	}
	s.log.Info(ctx, "Subscribed to mail queue", nil)

	if _, err := s.events.StreamSubscribe(s.opts.StreamName, s.opts.SendChannel, s.streamns+"SEND_WORKER", s.sendSubscriber, events.StreamConsumerOpts{
		AckWait:     30 * time.Second,
		MaxDeliver:  30,
		MaxPending:  1024,
		MaxRequests: 32,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to mail send queue")
	}
	s.log.Info(ctx, "Subscribed to send queue", nil)

	if _, err := s.events.StreamSubscribe(s.opts.StreamName, s.opts.DelChannel, s.streamns+"_DEL_WORKER", s.deleteSubscriber, events.StreamConsumerOpts{
		AckWait:     15 * time.Second,
		MaxDeliver:  30,
		MaxPending:  8192,
		MaxRequests: 32,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to list delete queue")
	}
	s.log.Info(ctx, "Subscribed to list delete queue", nil)

	if _, err := s.users.StreamSubscribeDelete(s.streamns+"_WORKER_DELETE", s.userDeleteHook, events.StreamConsumerOpts{
		AckWait:     15 * time.Second,
		MaxDeliver:  30,
		MaxPending:  1024,
		MaxRequests: 32,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to user delete queue")
	}
	s.log.Info(ctx, "Subscribed to user delete queue", nil)

	if _, err := s.orgs.StreamSubscribeDelete(s.streamns+"_WORKER_ORG_DELETE", s.orgDeleteHook, events.StreamConsumerOpts{
		AckWait:     15 * time.Second,
		MaxDeliver:  30,
		MaxPending:  1024,
		MaxRequests: 32,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to org delete queue")
	}
	s.log.Info(ctx, "Subscribed to org delete queue", nil)

	return nil
}

func (s *service) Stop(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := s.server.Close(); err != nil {
			s.log.Err(ctx, kerrors.WithMsg(err, "Shutdown mailing list SMTP server error"), nil)
		}
	}()
	select {
	case <-done:
	case <-ctx.Done():
		s.log.Err(ctx, kerrors.WithMsg(ctx.Err(), "Failed to stop"), nil)
	}
}

func (s *service) Setup(ctx context.Context, req governor.ReqSetup) error {
	if err := s.lists.Setup(ctx); err != nil {
		return err
	}
	s.log.Info(ctx, "Created mailing list tables", nil)
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

func (s *service) Health(ctx context.Context) error {
	return nil
}

const (
	listDeleteBatchSize = 256
)

func (s *service) userDeleteHook(ctx context.Context, pinger events.Pinger, props user.DeleteUserProps) error {
	return s.creatorDeleteHook(ctx, pinger, props.Userid)
}

func (s *service) orgDeleteHook(ctx context.Context, pinger events.Pinger, props org.DeleteOrgProps) error {
	return s.creatorDeleteHook(ctx, pinger, rank.ToOrgName(props.OrgID))
}

func (s *service) creatorDeleteHook(ctx context.Context, pinger events.Pinger, creatorid string) error {
	for {
		if err := pinger.Ping(ctx); err != nil {
			return err
		}
		lists, err := s.getCreatorLists(ctx, creatorid, listDeleteBatchSize, 0)
		if err != nil {
			return kerrors.WithMsg(err, "Failed to get user mailinglists")
		}
		if len(lists.Lists) == 0 {
			break
		}
		for _, i := range lists.Lists {
			if err := s.deleteList(ctx, i.CreatorID, i.Listname); err != nil {
				return kerrors.WithMsg(err, "Failed to delete list")
			}
		}
		if len(lists.Lists) < listDeleteBatchSize {
			break
		}
	}
	return nil
}
