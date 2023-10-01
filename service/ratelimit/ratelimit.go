package ratelimit

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	// Ratelimiter creates new ratelimiting middleware
	Ratelimiter interface {
		RatelimitCtx(tagger Tagger) governor.MiddlewareCtx
		Ratelimit(tagger Tagger) governor.Middleware
		Subtree(prefix string) Ratelimiter
		BaseCtx() governor.MiddlewareCtx
		Base() governor.Middleware
	}

	Service struct {
		tags       kvstore.KVStore
		log        *klog.LevelLogger
		paramsBase Params
		paramsAuth Params
	}

	// Tag is a request tag
	Tag struct {
		Key    string
		Value  string
		Params Params
	}

	// Tagger computes tags for requests
	Tagger func(c *governor.Context) []Tag

	// Params specify rate limiting params
	Params struct {
		Period int64 `json:"period"`
		Limit  int64 `json:"limit"`
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

// New creates a new Ratelimiter
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

func divroundup(a, b int64) int64 {
	return (a-1)/b + 1
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

func (s *Service) rlimitCtx(kv kvstore.KVStore, tagger Tagger) governor.MiddlewareCtx {
	return func(next governor.RouteHandler) governor.RouteHandler {
		return governor.RouteHandlerFunc(func(c *governor.Context) {
			now := time.Now().Round(0).Unix()
			tags := tagger(c)
			if len(tags) > 0 {
				multiget, err := kv.Multi(c.Ctx())
				if err != nil {
					s.log.Err(c.Ctx(), kerrors.WithMsg(err, "Failed to create kvstore multi"))
					goto end
				}
				sums := make([]tagSum, 0, len(tags))
				for _, i := range tags {
					if i.Params.Period <= 0 {
						s.log.Error(c.Ctx(), "Invalid ratelimit period",
							klog.AInt64("period", i.Params.Period),
						)
						continue
					}
					t := now / i.Params.Period
					prevWindow := float64(i.Params.Period-(now%i.Params.Period)) / float64(i.Params.Period)
					k1 := multiget.Subkey(i.Key, i.Value, strconv.FormatInt(t, 32))
					multiget.SetNX(c.Ctx(), k1, "0", time.Duration(i.Params.Period+1)*time.Second)
					k0 := multiget.Subkey(i.Key, i.Value, strconv.FormatInt(t-1, 32))
					sums = append(sums, tagSum{
						limit:      i.Params.Limit,
						current:    multiget.Incr(c.Ctx(), k1, 1),
						prev:       multiget.GetInt(c.Ctx(), k0),
						prevWindow: prevWindow,
						end:        (t + 1) * i.Params.Period,
					})
				}
				if len(sums) == 0 {
					goto end
				}
				if err := multiget.Exec(c.Ctx()); err != nil {
					s.log.Err(c.Ctx(), kerrors.WithMsg(err, "Failed to get tags from cache"))
					goto end
				}
				var minRatelimitEnd int64 = 0
				for _, i := range sums {
					current, err := i.current.Result()
					if err != nil {
						if !errors.Is(err, kvstore.ErrNotFound) {
							s.log.Err(c.Ctx(), kerrors.WithMsg(err, "Failed to get tag from cache"))
						}
						current = 0
					}
					prev, err := i.prev.Result()
					if err != nil {
						if !errors.Is(err, kvstore.ErrNotFound) {
							s.log.Err(c.Ctx(), kerrors.WithMsg(err, "Failed to get tag from cache"))
						}
						prev = 0
					}
					sum := min(current, i.limit) + int64(float64(min(prev, i.limit))*i.prevWindow)
					if sum <= i.limit {
						goto end
					}
					if minRatelimitEnd == 0 || i.end < minRatelimitEnd {
						minRatelimitEnd = i.end
					}
				}
				if minRatelimitEnd > 0 {
					c.WriteError(governor.ErrWithTooManyRequests(nil, time.Unix(minRatelimitEnd, 0).UTC(), "", "Hit rate limit"))
					return
				}
			}
		end:
			next.ServeHTTPCtx(c)
		})
	}
}

func (s *Service) RatelimitCtx(tagger Tagger) governor.MiddlewareCtx {
	return s.rlimitCtx(s.tags, tagger)
}

func (s *Service) Ratelimit(tagger Tagger) governor.Middleware {
	return governor.MiddlewareFromCtx(s.log.Logger, s.RatelimitCtx(tagger))
}

func (s *Service) Subtree(prefix string) Ratelimiter {
	return &tree{
		kv:   s.tags.Subtree(prefix),
		base: s,
	}
}

func (s *Service) BaseCtx() governor.MiddlewareCtx {
	return Compose(
		s,
		IPAddress("ip", s.paramsBase),
		Userid("id", s.paramsAuth),
	)
}

func (s *Service) Base() governor.Middleware {
	return governor.MiddlewareFromCtx(s.log.Logger, s.BaseCtx())
}

type (
	tree struct {
		kv   kvstore.KVStore
		base *Service
	}
)

func (t *tree) RatelimitCtx(tagger Tagger) governor.MiddlewareCtx {
	return t.base.rlimitCtx(t.kv, tagger)
}

func (t *tree) Ratelimit(tagger Tagger) governor.Middleware {
	return governor.MiddlewareFromCtx(t.base.log.Logger, t.RatelimitCtx(tagger))
}

func (t *tree) Subtree(prefix string) Ratelimiter {
	return &tree{
		kv:   t.kv.Subtree(prefix),
		base: t.base,
	}
}

func (t *tree) BaseCtx() governor.MiddlewareCtx {
	return t.base.BaseCtx()
}

func (t *tree) Base() governor.Middleware {
	return t.base.Base()
}

// Compose composes rate limit taggers
func Compose(r Ratelimiter, taggers ...Tagger) governor.MiddlewareCtx {
	return r.RatelimitCtx(func(c *governor.Context) []Tag {
		var tags []Tag
		for _, i := range taggers {
			tags = append(tags, i(c)...)
		}
		return tags
	})
}

// IPAddress tags ips
func IPAddress(key string, params Params) Tagger {
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

// Userid tags userids
func Userid(key string, params Params) Tagger {
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
