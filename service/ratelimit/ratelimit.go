package ratelimit

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/gate"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	// Limiter ratelimits operations
	Limiter interface {
		Ratelimit(ctx context.Context, tags []Tag) error
		Subtree(prefix string) Limiter
	}

	// Tag is a request tag
	Tag struct {
		Key    string
		Value  string
		Params Params
	}

	// Params specify rate limiting params
	Params struct {
		Period int64 `json:"period"`
		Limit  int64 `json:"limit"`
	}

	// ReqLimiter creates new ratelimiting middleware
	ReqLimiter interface {
		Limiter
		BaseTagger() ReqTagger
	}

	// ReqTagger computes tags for requests
	ReqTagger func(c *governor.Context) []Tag

	Service struct {
		tags       kvstore.KVStore
		log        *klog.LevelLogger
		paramsBase Params
		paramsAuth Params
	}
)

func (p Params) String() string {
	var b strings.Builder
	b.WriteString("period:")
	b.WriteString(strconv.FormatInt(p.Period, 10))
	b.WriteString(",limit:")
	b.WriteString(strconv.FormatInt(p.Limit, 10))
	return b.String()
}

// New creates a new [Limiter]
func New(kv kvstore.KVStore) *Service {
	return &Service{
		tags: kv.Subtree("tags"),
	}
}

func (s *Service) Register(r governor.ConfigRegistrar) {
	r.SetDefault("params.base", map[string]interface{}{
		"period": 60,
		"limit":  240,
	})
	r.SetDefault("params.auth", map[string]interface{}{
		"period": 60,
		"limit":  240,
	})
}

func (s *Service) Init(ctx context.Context, r governor.ConfigReader, kit governor.ServiceKit) error {
	s.log = klog.NewLevelLogger(kit.Logger)

	if err := r.Unmarshal("params.base", &s.paramsBase); err != nil {
		return kerrors.WithMsg(err, "Failed to parse base ratelimit params")
	}
	if err := r.Unmarshal("params.auth", &s.paramsAuth); err != nil {
		return kerrors.WithMsg(err, "Failed to parse auth ratelimit params")
	}

	s.log.Info(ctx, "Loaded config",
		klog.AString("base", s.paramsBase.String()),
		klog.AString("auth", s.paramsAuth.String()),
	)

	return nil
}

func (s *Service) Start(ctx context.Context) error {
	return nil
}

func (s *Service) Stop(ctx context.Context) {
}

func (s *Service) Setup(ctx context.Context, req governor.ReqSetup) error {
	return nil
}

func (s *Service) Health(ctx context.Context) error {
	return nil
}

type (
	tagSum struct {
		limit      int64
		current    kvstore.IntResulter
		prev       kvstore.IntResulter
		prevWindow float64
		end        int64
	}
)

func (s *Service) rlimit(ctx context.Context, kv kvstore.KVStore, tags []Tag) error {
	if len(tags) == 0 {
		return nil
	}

	now := time.Now().Round(0).Unix()
	multiget, err := kv.Multi(ctx)
	if err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to create kvstore multi"))
		return nil
	}
	sums := make([]tagSum, 0, len(tags))
	for _, i := range tags {
		if i.Params.Period <= 0 {
			s.log.Error(ctx, "Invalid ratelimit period",
				klog.AInt64("period", i.Params.Period),
			)
			continue
		}
		t := now / i.Params.Period
		prevWindow := float64(i.Params.Period-(now%i.Params.Period)) / float64(i.Params.Period)
		k1 := multiget.Subkey(i.Key, i.Value, strconv.FormatInt(t, 32))
		multiget.SetNX(ctx, k1, "0", time.Duration(i.Params.Period+1)*time.Second)
		k0 := multiget.Subkey(i.Key, i.Value, strconv.FormatInt(t-1, 32))
		sums = append(sums, tagSum{
			limit:      i.Params.Limit,
			current:    multiget.Incr(ctx, k1, 1),
			prev:       multiget.GetInt(ctx, k0),
			prevWindow: prevWindow,
			end:        (t + 1) * i.Params.Period,
		})
	}
	if len(sums) == 0 {
		return nil
	}
	if err := multiget.Exec(ctx); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to get tags from cache"))
		return nil
	}
	var minRatelimitEnd int64 = 0
	for _, i := range sums {
		current, err := i.current.Result()
		if err != nil {
			if !errors.Is(err, kvstore.ErrNotFound) {
				s.log.Err(ctx, kerrors.WithMsg(err, "Failed to get tag from cache"))
			}
			current = 0
		}
		prev, err := i.prev.Result()
		if err != nil {
			if !errors.Is(err, kvstore.ErrNotFound) {
				s.log.Err(ctx, kerrors.WithMsg(err, "Failed to get tag from cache"))
			}
			prev = 0
		}
		sum := min(current, i.limit) + int64(float64(min(prev, i.limit))*i.prevWindow)
		if sum <= i.limit {
			return nil
		}
		if minRatelimitEnd == 0 || i.end < minRatelimitEnd {
			minRatelimitEnd = i.end
		}
	}
	if minRatelimitEnd > 0 {
		return governor.ErrWithTooManyRequests(nil, time.Unix(minRatelimitEnd, 0).UTC(), "", "Hit rate limit")
	}
	return nil
}

func (s *Service) Ratelimit(ctx context.Context, tags []Tag) error {
	return s.rlimit(ctx, s.tags, tags)
}

func (s *Service) Subtree(prefix string) Limiter {
	return &tree{
		kv:   s.tags.Subtree(prefix),
		base: s,
	}
}

type (
	tree struct {
		kv   kvstore.KVStore
		base *Service
	}
)

func (t *tree) Ratelimit(ctx context.Context, tags []Tag) error {
	return t.base.rlimit(ctx, t.kv, tags)
}

func (t *tree) Subtree(prefix string) Limiter {
	return &tree{
		kv:   t.kv.Subtree(prefix),
		base: t.base,
	}
}

func (s *Service) BaseTagger() ReqTagger {
	return ComposeReqTaggers(
		ReqTaggerIPAddress("ip", s.paramsBase),
		ReqTaggerUserid("id", s.paramsAuth),
	)
}

// LimitReqCtx creates ratelimiting middleware
func LimitReqCtx(l Limiter, tagger ReqTagger) governor.MiddlewareCtx {
	return func(next governor.RouteHandler) governor.RouteHandler {
		return governor.RouteHandlerFunc(func(c *governor.Context) {
			tags := tagger(c)
			l.Ratelimit(c.Ctx(), tags)
			next.ServeHTTPCtx(c)
		})
	}
}

// ComposeReqTaggers composes rate limit req taggers
func ComposeReqTaggers(taggers ...ReqTagger) ReqTagger {
	return func(c *governor.Context) []Tag {
		var tags []Tag
		for _, i := range taggers {
			tags = append(tags, i(c)...)
		}
		return tags
	}
}

// ReqTaggerIPAddress tags ips
func ReqTaggerIPAddress(key string, params Params) ReqTagger {
	if params.Period <= 0 {
		panic("period must be positive")
	}
	return func(c *governor.Context) []Tag {
		ip := c.RealIP()
		if ip == nil {
			return nil
		}
		return []Tag{
			{
				Key:    key,
				Value:  ip.String(),
				Params: params,
			},
		}
	}
}

// ReqTaggerUserid tags userids
func ReqTaggerUserid(key string, params Params) ReqTagger {
	if params.Period <= 0 {
		panic("period must be positive")
	}
	return func(c *governor.Context) []Tag {
		userid := gate.GetCtxUserid(c)
		if userid == "" {
			return nil
		}
		return []Tag{
			{
				Key:    key,
				Value:  userid,
				Params: params,
			},
		}
	}
}
