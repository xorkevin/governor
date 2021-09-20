package mailinglist

import (
	"context"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/util/bytefmt"
)

type (
	service struct {
		logger       governor.Logger
		port         string
		domain       string
		maxmsgsize   int
		readtimeout  time.Duration
		writetimeout time.Duration
	}
)

func (s *service) Register(inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
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
