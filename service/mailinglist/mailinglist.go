package mailinglist

import (
	"context"
	"encoding/json"
	"net"
	"sync/atomic"
	"time"

	"github.com/emersion/go-smtp"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/mail"
	"xorkevin.dev/governor/service/mailinglist/mailinglistmodel"
	"xorkevin.dev/governor/service/objstore"
	"xorkevin.dev/governor/service/ratelimit"
	"xorkevin.dev/governor/service/user"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/service/user/org"
	"xorkevin.dev/governor/util/bytefmt"
	"xorkevin.dev/governor/util/dns"
	"xorkevin.dev/governor/util/kjson"
	"xorkevin.dev/governor/util/ksync"
	"xorkevin.dev/governor/util/rank"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

const (
	listEventKindMail   = "mail"
	listEventKindSend   = "send"
	listEventKindDelete = "delete"
)

type (
	listEventDec struct {
		Kind    string          `json:"kind"`
		Payload json.RawMessage `json:"payload"`
	}

	listEventEnc struct {
		Kind    string      `json:"kind"`
		Payload interface{} `json:"payload"`
	}

	listEvent struct {
		Kind   string
		Mail   mailProps
		Send   sendProps
		Delete delProps
	}

	mailProps struct {
		ListID string `json:"listid"`
		MsgID  string `json:"msgid"`
	}

	sendProps struct {
		ListID string `json:"listid"`
		MsgID  string `json:"msgid"`
	}

	delProps struct {
		ListID string `json:"listid"`
	}

	// MailingList is a mailing list service
	MailingList interface{}

	Service struct {
		lists        mailinglistmodel.Repo
		mailBucket   objstore.Bucket
		rcvMailDir   objstore.Dir
		events       events.Events
		users        user.Users
		orgs         org.Orgs
		mailer       mail.Mailer
		ratelimiter  ratelimit.Ratelimiter
		gate         gate.Gate
		config       governor.ConfigReader
		log          *klog.LevelLogger
		scopens      string
		streamns     string
		streammail   string
		resolver     dns.Resolver
		server       *smtp.Server
		port         string
		authdomain   string
		usrdomain    string
		orgdomain    string
		maxmsgsize   int64
		readtimeout  time.Duration
		writetimeout time.Duration
		streamsize   int64
		eventsize    int32
		wg           *ksync.WaitGroup
	}

	router struct {
		s  *Service
		rt governor.MiddlewareCtx
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
func NewCtx(inj governor.Injector) *Service {
	lists := mailinglistmodel.GetCtxRepo(inj)
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
func New(lists mailinglistmodel.Repo, obj objstore.Bucket, ev events.Events, users user.Users, orgs org.Orgs, mailer mail.Mailer, ratelimiter ratelimit.Ratelimiter, g gate.Gate) *Service {
	return &Service{
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
		wg: ksync.NewWaitGroup(),
	}
}

func (s *Service) Register(inj governor.Injector, r governor.ConfigRegistrar) {
	setCtxMailingList(inj, s)
	s.scopens = "gov." + r.Name()
	s.streamns = r.Name()
	s.streammail = r.Name()

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

func (s *Service) router() *router {
	return &router{
		s:  s,
		rt: s.ratelimiter.BaseCtx(),
	}
}

func (s *Service) Init(ctx context.Context, r governor.ConfigReader, log klog.Logger, m governor.Router) error {
	s.log = klog.NewLevelLogger(log)
	s.config = r

	s.port = r.GetStr("port")
	s.authdomain = r.GetStr("authdomain")
	s.usrdomain = r.GetStr("usrdomain")
	s.orgdomain = r.GetStr("orgdomain")
	if limit, err := bytefmt.ToBytes(r.GetStr("maxmsgsize")); err != nil {
		return kerrors.WithMsg(err, "Invalid mail max message size")
	} else {
		s.maxmsgsize = limit
	}
	var err error
	s.readtimeout, err = r.GetDuration("readtimeout")
	if err != nil {
		return kerrors.WithMsg(err, "Invalid read timeout for mail server")
	}
	s.writetimeout, err = r.GetDuration("writetimeout")
	if err != nil {
		return kerrors.WithKind(err, governor.ErrInvalidConfig, "Invalid write timeout for mail server")
	}

	if src := r.GetStr("mockdnssource"); src != "" {
		var err error
		s.resolver, err = dns.NewMockResolverFromFile(src)
		if err != nil {
			return kerrors.WithKind(err, governor.ErrInvalidConfig, "Invalid mockdns source")
		}
		s.log.Info(ctx, "Use mockdns",
			klog.AString("source", src),
		)
	}

	s.streamsize, err = bytefmt.ToBytes(r.GetStr("streamsize"))
	if err != nil {
		return kerrors.WithMsg(err, "Invalid stream size")
	}
	eventsize, err := bytefmt.ToBytes(r.GetStr("eventsize"))
	if err != nil {
		return kerrors.WithMsg(err, "Invalid msg size")
	}
	s.eventsize = int32(eventsize)

	s.log.Info(ctx, "Loaded config",
		klog.AString("smtp.port", s.port),
		klog.AString("authdomain", s.authdomain),
		klog.AString("usrdomain", s.usrdomain),
		klog.AString("orgdomain", s.orgdomain),
		klog.AString("maxmsgsize", r.GetStr("maxmsgsize")),
		klog.AString("readtimeout", s.readtimeout.String()),
		klog.AString("writetimeout", s.writetimeout.String()),
		klog.AString("streamsize", r.GetStr("streamsize")),
		klog.AString("eventsize", r.GetStr("eventsize")),
	)

	ctx = klog.CtxWithAttrs(ctx, klog.AString("gov.phase", "run"))

	s.server = s.createSMTPServer()
	go func() {
		if err := s.server.ListenAndServe(); err != nil {
			s.log.Err(ctx, kerrors.WithMsg(err, "Shutting down mailinglist SMTP server"))
		}
	}()

	sr := s.router()
	sr.mountRoutes(m)
	s.log.Info(ctx, "Mounted http routes")

	return nil
}

func (s *Service) createSMTPServer() *smtp.Server {
	be := &smtpBackend{
		service:  s,
		instance: s.config.Config().Instance,
		log:      klog.NewLevelLogger(s.log.Logger.Sublogger("smtpserver")),
		reqcount: &atomic.Uint32{},
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

func (s *Service) Start(ctx context.Context) error {
	s.wg.Add(1)
	go events.NewWatcher(
		s.events,
		s.log.Logger,
		s.streammail,
		s.streamns+".worker",
		events.ConsumerOpts{},
		events.HandlerFunc(s.listEventHandler),
		nil,
		0,
		s.config.Config().Instance,
	).Watch(ctx, s.wg, events.WatchOpts{})
	s.log.Info(ctx, "Subscribed to mailinglist stream")

	s.wg.Add(1)
	go s.users.WatchUsers(s.streamns+".worker.users", events.ConsumerOpts{}, s.userEventHandler, nil, 0).Watch(ctx, s.wg, events.WatchOpts{})
	s.log.Info(ctx, "Subscribed to users stream")

	s.wg.Add(1)
	go s.orgs.WatchOrgs(s.streamns+".worker.orgs", events.ConsumerOpts{}, s.orgEventHandler, nil, 0).Watch(ctx, s.wg, events.WatchOpts{})
	s.log.Info(ctx, "Subscribed to orgs stream")

	return nil
}

func (s *Service) Stop(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := s.server.Close(); err != nil {
			s.log.Err(ctx, kerrors.WithMsg(err, "Shutdown mailing list SMTP server error"))
		}
	}()
	if err := s.wg.Wait(ctx); err != nil {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to stop"))
	}
	select {
	case <-done:
	case <-ctx.Done():
		s.log.Warn(ctx, "Failed to stop")
	}
}

func (s *Service) Setup(ctx context.Context, req governor.ReqSetup) error {
	if err := s.lists.Setup(ctx); err != nil {
		return err
	}
	s.log.Info(ctx, "Created mailing list tables")
	if err := s.mailBucket.Init(ctx); err != nil {
		return kerrors.WithMsg(err, "Failed to init mail bucket")
	}
	s.log.Info(ctx, "Created mail bucket")
	if err := s.events.InitStream(ctx, s.streammail, events.StreamOpts{
		Partitions:     16,
		Replicas:       1,
		ReplicaQuorum:  1,
		RetentionAge:   30 * 24 * time.Hour,
		RetentionBytes: int(s.streamsize),
		MaxMsgBytes:    int(s.eventsize),
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to init mailinglist stream")
	}
	s.log.Info(ctx, "Created mailinglist stream")
	return nil
}

func (s *Service) Health(ctx context.Context) error {
	return nil
}

type (
	// errListEvent is returned when the mailinglist message is malformed
	errListEvent struct{}
)

func (e errListEvent) Error() string {
	return "Malformed mailinglist message"
}

func decodeListEvent(msgdata []byte) (*listEvent, error) {
	var m listEventDec
	if err := kjson.Unmarshal(msgdata, &m); err != nil {
		return nil, kerrors.WithKind(err, errListEvent{}, "Failed to decode mailinglist event")
	}
	props := &listEvent{
		Kind: m.Kind,
	}
	switch m.Kind {
	case listEventKindMail:
		if err := kjson.Unmarshal(m.Payload, &props.Mail); err != nil {
			return nil, kerrors.WithKind(err, errListEvent{}, "Failed to decode mail event")
		}
	case listEventKindSend:
		if err := kjson.Unmarshal(m.Payload, &props.Send); err != nil {
			return nil, kerrors.WithKind(err, errListEvent{}, "Failed to decode send event")
		}
	case listEventKindDelete:
		if err := kjson.Unmarshal(m.Payload, &props.Delete); err != nil {
			return nil, kerrors.WithKind(err, errListEvent{}, "Failed to decode delete event")
		}
	default:
		return nil, kerrors.WithKind(nil, errListEvent{}, "Invalid list event kind")
	}
	return props, nil
}

func encodeListEventMail(props mailProps) ([]byte, error) {
	b, err := kjson.Marshal(listEventEnc{
		Kind:    listEventKindMail,
		Payload: props,
	})
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to encode mail props to json")
	}
	return b, nil
}

func encodeListEventSend(props sendProps) ([]byte, error) {
	b, err := kjson.Marshal(listEventEnc{
		Kind:    listEventKindSend,
		Payload: props,
	})
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to encode send props to json")
	}
	return b, nil
}

func encodeListEventDelete(props delProps) ([]byte, error) {
	b, err := kjson.Marshal(listEventEnc{
		Kind:    listEventKindDelete,
		Payload: props,
	})
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to encode delete props to json")
	}
	return b, nil
}

func (s *Service) listEventHandler(ctx context.Context, msg events.Msg) error {
	props, err := decodeListEvent(msg.Value)
	if err != nil {
		return err
	}
	switch props.Kind {
	case listEventKindMail:
		return s.mailEventHandler(ctx, props.Mail)
	case listEventKindSend:
		return s.sendEventHandler(ctx, props.Send)
	case listEventKindDelete:
		return s.deleteEventHandler(ctx, props.Delete)
	default:
		return nil
	}
}

const (
	listDeleteBatchSize = 256
)

func (s *Service) userEventHandler(ctx context.Context, props user.UserEvent) error {
	switch props.Kind {
	case user.UserEventKindDelete:
		return s.userDeleteEventHandler(ctx, props.Delete)
	default:
		return nil
	}
}

func (s *Service) userDeleteEventHandler(ctx context.Context, props user.DeleteUserProps) error {
	return s.creatorDeleteEventHandler(ctx, props.Userid)
}

func (s *Service) orgEventHandler(ctx context.Context, props org.OrgEvent) error {
	switch props.Kind {
	case org.OrgEventKindDelete:
		return s.orgDeleteEventHandler(ctx, props.Delete)
	default:
		return nil
	}
}

func (s *Service) orgDeleteEventHandler(ctx context.Context, props org.DeleteOrgProps) error {
	return s.creatorDeleteEventHandler(ctx, rank.ToOrgName(props.OrgID))
}

func (s *Service) creatorDeleteEventHandler(ctx context.Context, creatorid string) error {
	for {
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
