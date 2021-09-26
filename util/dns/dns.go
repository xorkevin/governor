package dns

import (
	"context"
	"sync"
	"time"
)

type (
	Resolver interface {
		LookupTXT(ctx context.Context, name string) ([]string, error)
	}

	cachedTxt struct {
		record []string
		valid  time.Time
	}

	CachingResolver struct {
		resolver      Resolver
		cacheDuration time.Duration
		lock          *sync.RWMutex
		cacheTxt      map[string]cachedTxt
	}
)

func NewCachingResolver(r Resolver, t time.Duration) *CachingResolver {
	return &CachingResolver{
		resolver:      r,
		cacheDuration: t,
		lock:          &sync.RWMutex{},
		cacheTxt:      map[string]cachedTxt{},
	}
}

func (r *CachingResolver) checkCacheTXT(name string) ([]string, bool) {
	r.lock.RLock()
	defer r.lock.RUnlock()
	c, ok := r.cacheTxt[name]
	if !ok || time.Now().Round(0).After(c.valid) {
		return nil, false
	}
	res := make([]string, len(c.record))
	copy(res, c.record)
	return res, true
}

func (r *CachingResolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	if res, ok := r.checkCacheTXT(name); ok {
		return res, nil
	}
	rec, err := r.resolver.LookupTXT(ctx, name)
	if err != nil {
		return nil, err
	}
	res := make([]string, len(rec))
	copy(res, rec)
	valid := time.Now().Round(0).Add(r.cacheDuration)

	r.lock.Lock()
	defer r.lock.Unlock()
	r.cacheTxt[name] = cachedTxt{
		record: rec,
		valid:  valid,
	}
	return res, nil
}
