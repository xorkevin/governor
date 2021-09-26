package dns

import (
	"context"
	"net"
	"sync"
	"time"
)

type (
	Resolver interface {
		LookupTXT(ctx context.Context, name string) ([]string, error)
		LookupMX(ctx context.Context, name string) ([]*net.MX, error)
		LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
		LookupAddr(ctx context.Context, addr string) ([]string, error)
	}

	cachedTXT struct {
		record []string
		valid  time.Time
	}

	cachedMX struct {
		record []net.MX
		valid  time.Time
	}

	cachedIPAddr struct {
		record []net.IPAddr
		valid  time.Time
	}

	cachedAddr struct {
		record []string
		valid  time.Time
	}

	CachingResolver struct {
		resolver      Resolver
		cacheDuration time.Duration
		lockTXT       *sync.RWMutex
		cacheTXT      map[string]cachedTXT
		lockMX        *sync.RWMutex
		cacheMX       map[string]cachedMX
		lockIPAddr    *sync.RWMutex
		cacheIPAddr   map[string]cachedIPAddr
		lockAddr      *sync.RWMutex
		cacheAddr     map[string]cachedAddr
	}
)

func NewCachingResolver(r Resolver, t time.Duration) *CachingResolver {
	return &CachingResolver{
		resolver:      r,
		cacheDuration: t,
		lockTXT:       &sync.RWMutex{},
		cacheTXT:      map[string]cachedTXT{},
		lockMX:        &sync.RWMutex{},
		cacheMX:       map[string]cachedMX{},
		lockIPAddr:    &sync.RWMutex{},
		cacheIPAddr:   map[string]cachedIPAddr{},
		lockAddr:      &sync.RWMutex{},
		cacheAddr:     map[string]cachedAddr{},
	}
}

func (r *CachingResolver) checkCacheTXT(name string) ([]string, bool) {
	r.lockTXT.RLock()
	defer r.lockTXT.RUnlock()
	c, ok := r.cacheTXT[name]
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

	r.lockTXT.Lock()
	defer r.lockTXT.Unlock()
	r.cacheTXT[name] = cachedTXT{
		record: rec,
		valid:  valid,
	}
	return res, nil
}

func (r *CachingResolver) checkCacheMX(name string) ([]*net.MX, bool) {
	r.lockMX.RLock()
	defer r.lockMX.RUnlock()
	c, ok := r.cacheMX[name]
	if !ok || time.Now().Round(0).After(c.valid) {
		return nil, false
	}
	res := make([]*net.MX, 0, len(c.record))
	for _, i := range c.record {
		k := i
		res = append(res, &k)
	}
	return res, true
}

func (r *CachingResolver) LookupMX(ctx context.Context, name string) ([]*net.MX, error) {
	if res, ok := r.checkCacheMX(name); ok {
		return res, nil
	}
	res, err := r.resolver.LookupMX(ctx, name)
	if err != nil {
		return nil, err
	}
	rec := make([]net.MX, 0, len(res))
	for _, i := range res {
		if i == nil {
			continue
		}
		rec = append(rec, *i)
	}
	valid := time.Now().Round(0).Add(r.cacheDuration)

	r.lockMX.Lock()
	defer r.lockMX.Unlock()
	r.cacheMX[name] = cachedMX{
		record: rec,
		valid:  valid,
	}
	return res, nil
}

func (r *CachingResolver) checkCacheIPAddr(name string) ([]net.IPAddr, bool) {
	r.lockIPAddr.RLock()
	defer r.lockIPAddr.RUnlock()
	c, ok := r.cacheIPAddr[name]
	if !ok || time.Now().Round(0).After(c.valid) {
		return nil, false
	}
	res := make([]net.IPAddr, len(c.record))
	copy(res, c.record)
	return res, true
}

func (r *CachingResolver) LookupIPAddr(ctx context.Context, name string) ([]net.IPAddr, error) {
	if res, ok := r.checkCacheIPAddr(name); ok {
		return res, nil
	}
	rec, err := r.resolver.LookupIPAddr(ctx, name)
	if err != nil {
		return nil, err
	}
	res := make([]net.IPAddr, len(rec))
	copy(res, rec)
	valid := time.Now().Round(0).Add(r.cacheDuration)

	r.lockIPAddr.Lock()
	defer r.lockIPAddr.Unlock()
	r.cacheIPAddr[name] = cachedIPAddr{
		record: rec,
		valid:  valid,
	}
	return res, nil
}

func (r *CachingResolver) checkCacheAddr(name string) ([]string, bool) {
	r.lockAddr.RLock()
	defer r.lockAddr.RUnlock()
	c, ok := r.cacheAddr[name]
	if !ok || time.Now().Round(0).After(c.valid) {
		return nil, false
	}
	res := make([]string, len(c.record))
	copy(res, c.record)
	return res, true
}

func (r *CachingResolver) LookupAddr(ctx context.Context, name string) ([]string, error) {
	if res, ok := r.checkCacheAddr(name); ok {
		return res, nil
	}
	rec, err := r.resolver.LookupAddr(ctx, name)
	if err != nil {
		return nil, err
	}
	res := make([]string, len(rec))
	copy(res, rec)
	valid := time.Now().Round(0).Add(r.cacheDuration)

	r.lockAddr.Lock()
	defer r.lockAddr.Unlock()
	r.cacheAddr[name] = cachedAddr{
		record: rec,
		valid:  valid,
	}
	return res, nil
}
