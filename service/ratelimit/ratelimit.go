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
		Expiration int64 `json:"expiration"`
		Period     int64 `json:"period"`
		Limit      int64 `json:"limit"`
	}

	ctxKeyRootRL struct{}

	ctxKeyRatelimiter struct{}
)

func (p Params) String() string {
	b := strings.Builder{}
	b.WriteString("expiration:")
	b.WriteString(strconv.FormatInt(p.Expiration, 10))
	b.WriteString(",period:")
	b.WriteString(strconv.FormatInt(p.Period, 10))
	b.WriteString(",limit:")
	b.WriteString(strconv.FormatInt(p.Limit, 10))
	return b.String()
}

// getCtxRootRL returns a root Ratelimiter from the context
func getCtxRootRL(inj governor.Injector) Ratelimiter {
	v := inj.Get(ctxKeyRootRL{})
	if v == nil {
		return nil
	}
	return v.(Ratelimiter)
}

// setCtxRootRL sets a root Ratelimiter in the context
func setCtxRootRL(inj governor.Injector, r Ratelimiter) {
	inj.Set(ctxKeyRootRL{}, r)
}

// GetCtxRatelimiter returns a Ratelimiter from the context
func GetCtxRatelimiter(inj governor.Injector) Ratelimiter {
	v := inj.Get(ctxKeyRatelimiter{})
	if v == nil {
		return nil
	}
	return v.(Ratelimiter)
}

// setCtxRatelimiter sets a Ratelimiter in the context
func setCtxRatelimiter(inj governor.Injector, r Ratelimiter) {
	inj.Set(ctxKeyRatelimiter{}, r)
}

// NewSubtreeInCtx creates a new ratelimiter subtree with a prefix and sets it in the context
func NewSubtreeInCtx(inj governor.Injector, prefix string) {
	rt := getCtxRootRL(inj)
	setCtxRatelimiter(inj, rt.Subtree(prefix))
}

// NewCtx creates a new Ratelimiter from a context
func NewCtx(inj governor.Injector) *Service {
	kv := kvstore.GetCtxKVStore(inj)
	return New(kv)
}

// New creates a new Ratelimiter
func New(kv kvstore.KVStore) *Service {
	return &Service{
		tags: kv.Subtree("tags"),
	}
}

func (s *Service) Register(inj governor.Injector, r governor.ConfigRegistrar) {
	setCtxRootRL(inj, s)

	r.SetDefault("params.base", map[string]interface{}{
		"expiration": 60,
		"period":     15,
		"limit":      240,
	})
	r.SetDefault("params.auth", map[string]interface{}{
		"expiration": 60,
		"period":     15,
		"limit":      120,
	})
}

func (s *Service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, log klog.Logger, m governor.Router) error {
	s.log = klog.NewLevelLogger(log)

	if err := r.Unmarshal("params.base", &s.paramsBase); err != nil {
		return kerrors.WithMsg(err, "Failed to parse base ratelimit params")
	}
	if err := r.Unmarshal("params.auth", &s.paramsAuth); err != nil {
		return kerrors.WithMsg(err, "Failed to parse auth ratelimit params")
	}

	s.log.Info(ctx, "Loaded config", klog.Fields{
		"ratelimit.params.base": s.paramsBase.String(),
		"ratelimit.params.auth": s.paramsAuth.String(),
	})

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
		limit   int64
		periods []kvstore.IntResulter
		end     int64
	}
)

func (s *Service) rlimitCtx(kv kvstore.KVStore, tagger Tagger) governor.MiddlewareCtx {
	return func(next governor.RouteHandler) governor.RouteHandler {
		return governor.RouteHandlerFunc(func(c *governor.Context) {
			now := time.Now().Round(0).Unix()
			tags := tagger(c)
			if len(tags) > 0 {
				multiget, err := kv.Tx(c.Ctx())
				if err != nil {
					s.log.Err(c.Ctx(), kerrors.WithMsg(err, "Failed to create kvstore multi"), nil)
					goto end
				}
				sums := make([]tagSum, 0, len(tags))
				for _, i := range tags {
					if i.Params.Period <= 0 {
						s.log.Error(c.Ctx(), "Invalid ratelimit period", klog.Fields{
							"ratelimit.tag.period": i.Params.Period,
						})
						continue
					}
					t := now / i.Params.Period
					l := divroundup(i.Params.Expiration, i.Params.Period)
					periods := make([]kvstore.IntResulter, 0, l)
					k := multiget.Subkey(i.Key, i.Value, strconv.FormatInt(t, 32))
					periods = append(periods, multiget.Incr(c.Ctx(), k, 1))
					multiget.Expire(c.Ctx(), k, time.Duration(i.Params.Period+1)*time.Second)
					for j := int64(1); j < l; j++ {
						periods = append(periods, multiget.GetInt(c.Ctx(), multiget.Subkey(i.Key, i.Value, strconv.FormatInt(t-j, 32))))
					}
					sums = append(sums, tagSum{
						limit:   i.Params.Limit,
						periods: periods,
						end:     (t + 1) * i.Params.Period,
					})
				}
				if err := multiget.Exec(c.Ctx()); err != nil {
					s.log.Err(c.Ctx(), kerrors.WithMsg(err, "Failed to get tags from cache"), nil)
					goto end
				}
				for _, i := range sums {
					var sum int64 = 0
					for _, j := range i.periods {
						k, err := j.Result()
						if err != nil {
							if !errors.Is(err, kvstore.ErrorNotFound{}) {
								s.log.Err(c.Ctx(), kerrors.WithMsg(err, "Failed to get tag from cache"), nil)
							}
							continue
						}
						sum += k
					}
					if sum > i.limit {
						c.WriteError(governor.ErrWithTooManyRequests(nil, time.Unix(i.end, 0).UTC(), "", "Hit rate limit"))
						return
					}
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
		Userid("id", s.paramsBase),
		UseridIPAddress("id_ip", s.paramsAuth),
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

// UseridIPAddress tags userid ip tuples
func UseridIPAddress(key string, params Params) Tagger {
	if params.Period <= 0 {
		panic("period must be positive")
	}
	return func(c *governor.Context) []Tag {
		userid := gate.GetCtxUserid(c)
		if userid == "" {
			return nil
		}
		ip := c.RealIP()
		if ip == nil {
			return nil
		}
		return []Tag{
			{
				Key:    key,
				Value:  userid + "_" + ip.String(),
				Params: params,
			},
		}
	}
}
