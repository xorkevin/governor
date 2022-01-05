package ratelimit

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/user/gate"
)

type (
	// Ratelimiter creates new ratelimiting middleware
	Ratelimiter interface {
		Ratelimit(tagger Tagger) governor.Middleware
		Subtree(prefix string) Ratelimiter
	}

	// Service is a Gate and governor.Service
	Service interface {
		governor.Service
		Ratelimiter
	}

	service struct {
		tags   kvstore.KVStore
		logger governor.Logger
	}

	// Tag is a request tag
	Tag struct {
		Key    string
		Value  string
		Params Params
	}

	// Tagger computes tags for requests
	Tagger func(c governor.Context) []Tag

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
func NewCtx(inj governor.Injector) Service {
	kv := kvstore.GetCtxKVStore(inj)
	return New(kv)
}

// New creates a new Ratelimiter
func New(kv kvstore.KVStore) Service {
	return &service{
		tags: kv.Subtree("tags"),
	}
}

func (s *service) Register(name string, inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	setCtxRootRL(inj, s)
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, m governor.Router) error {
	s.logger = l
	return nil
}

func (s *service) Setup(req governor.ReqSetup) error {
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

func divroundup(a, b int64) int64 {
	return (a-1)/b + 1
}

type (
	tagSum struct {
		limit   int64
		periods []kvstore.IntResulter
	}
)

func (s *service) rlimit(kv kvstore.KVStore, tagger Tagger) governor.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := governor.NewContext(w, r, s.logger)
			now := time.Now().Round(0).Unix()
			tags := tagger(c)
			if len(tags) > 0 {
				multiget, err := kv.Tx()
				if err != nil {
					s.logger.Error("Failed to create kvstore multi", map[string]string{
						"error": err.Error(),
					})
					goto end
				}
				sums := make([]tagSum, 0, len(tags))
				for _, i := range tags {
					if i.Params.Period <= 0 {
						s.logger.Error("Invalid ratelimit period", map[string]string{
							"error": "Ratelimit period",
						})
						continue
					}
					t := now / i.Params.Period
					l := divroundup(i.Params.Expiration, i.Params.Period)
					periods := make([]kvstore.IntResulter, 0, l)
					k := multiget.Subkey(i.Key, i.Value, strconv.FormatInt(t, 32))
					periods = append(periods, multiget.Incr(k, 1))
					multiget.Expire(k, i.Params.Period+1)
					for j := int64(1); j < l; j++ {
						periods = append(periods, multiget.GetInt(multiget.Subkey(i.Key, i.Value, strconv.FormatInt(t-j, 32))))
					}
					sums = append(sums, tagSum{
						limit:   i.Params.Limit,
						periods: periods,
					})
				}
				if err := multiget.Exec(); err != nil {
					s.logger.Error("Failed to get tags from cache", map[string]string{
						"error":      err.Error(),
						"actiontype": "getratelimittags",
					})
					goto end
				}
				for _, i := range sums {
					var sum int64 = 0
					for _, j := range i.periods {
						k, err := j.Result()
						if err != nil {
							if !errors.Is(err, kvstore.ErrNotFound{}) {
								s.logger.Error("Failed to get tag from cache", map[string]string{
									"error":      err.Error(),
									"actiontype": "getratelimittagresult",
								})
							}
							continue
						}
						sum += k
					}
					if sum > i.limit {
						c.WriteStatus(http.StatusTooManyRequests)
						return
					}
				}
			}
		end:
			next.ServeHTTP(c.R())
		})
	}
}

func (s *service) Ratelimit(tagger Tagger) governor.Middleware {
	return s.rlimit(s.tags, tagger)
}

func (s *service) Subtree(prefix string) Ratelimiter {
	return &tree{
		kv:   s.tags.Subtree(prefix),
		base: s,
	}
}

type (
	tree struct {
		kv   kvstore.KVStore
		base *service
	}
)

func (t *tree) Ratelimit(tagger Tagger) governor.Middleware {
	return t.base.rlimit(t.kv, tagger)
}

func (t *tree) Subtree(prefix string) Ratelimiter {
	return &tree{
		kv:   t.kv.Subtree(prefix),
		base: t.base,
	}
}

// Compose composes rate limit taggers
func Compose(r Ratelimiter, taggers ...Tagger) governor.Middleware {
	return r.Ratelimit(func(c governor.Context) []Tag {
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
	return func(c governor.Context) []Tag {
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
	return func(c governor.Context) []Tag {
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
	return func(c governor.Context) []Tag {
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
