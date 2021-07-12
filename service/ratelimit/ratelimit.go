package ratelimit

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/kvstore"
)

type (
	// Ratelimiter creates new ratelimiting middleware
	Ratelimiter interface {
		Ratelimit(tagger Tagger) governor.Middleware
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
		Key        string
		Value      string
		Expiration int64
		Period     int64
		Limit      int64
	}

	// Tagger computes tags for requests
	Tagger func(ctx governor.Context) []Tag

	ctxKeyRatelimiter struct{}
)

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

func (s *service) Register(inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	setCtxRatelimiter(inj, s)
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

func (s *service) Ratelimit(tagger Tagger) governor.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := governor.NewContext(w, r, s.logger)
			now := time.Now().Round(0).Unix()
			tags := tagger(c)
			if len(tags) > 0 {
				multiget, err := s.tags.Multi()
				if err != nil {
					s.logger.Error("Failed to create kvstore multi", map[string]string{
						"error": err.Error(),
					})
					goto end
				}
				sums := make([]tagSum, 0, len(tags))
				for _, i := range tags {
					if i.Period == 0 {
						s.logger.Error("Invalid ratelimit period 0", map[string]string{
							"error": "Ratelimit period 0",
						})
						continue
					}
					t := now / i.Period
					k := divroundup(i.Expiration, i.Period)
					periods := make([]kvstore.IntResulter, 0, k)
					periods = append(periods, multiget.Incr(multiget.Subkey(i.Key, i.Value, strconv.FormatInt(t, 32)), 1))
					for j := int64(1); j < k; j++ {
						periods = append(periods, multiget.GetInt(multiget.Subkey(i.Key, i.Value, strconv.FormatInt(t-j, 32))))
					}
					sums = append(sums, tagSum{
						limit:   i.Limit,
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
