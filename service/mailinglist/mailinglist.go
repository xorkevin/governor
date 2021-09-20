package mailinglist

import (
	"context"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/objstore"
	"xorkevin.dev/governor/util/bytefmt"
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
		mailBucket   objstore.Bucket
		rcvMailDir   objstore.Dir
		logger       governor.Logger
		port         string
		domain       string
		maxmsgsize   int
		readtimeout  time.Duration
		writetimeout time.Duration
	}

	ctxKeyMailingList struct{}
)

// GetCtxMailingList returns a MailingList service from the context
func GetCtxMailer(inj governor.Injector) MailingList {
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

func New(obj objstore.Bucket) Service {
	return &service{
		mailBucket: obj,
		rcvMailDir: obj.Subdir("rcvmail"),
	}
}

func (s *service) Register(inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	setCtxMailingList(inj, s)

	r.SetDefault("port", "2525")
	r.SetDefault("domain", "localhost")
	r.SetDefault("maxmsgsize", "2M")
	r.SetDefault("readtimeout", "5s")
	r.SetDefault("writetimeout", "5s")
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, m governor.Router) error {
	s.logger = l
	l = s.logger.WithData(map[string]string{
		"phase": "init",
	})

	s.port = r.GetStr("port")
	s.domain = r.GetStr("domain")
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

	l.Info("Initialize mailing list", map[string]string{
		"port":               s.port,
		"domain":             s.domain,
		"maxmsgsize (bytes)": r.GetStr("maxmsgsize"),
		"read timeout":       r.GetStr("readtimeout"),
		"write timeout":      r.GetStr("writetimeout"),
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
	return nil
}

func (s *service) PostSetup(req governor.ReqSetup) error {
	return nil
}

func (s *service) Start(ctx context.Context) error {
	return nil
}

func (s *service) Stop(ctx context.Context) {
}

func (s *service) Health() error {
	return nil
}
