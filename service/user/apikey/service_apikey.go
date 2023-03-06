package apikey

import (
	"context"
	"errors"

	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/user/apikey/apikeymodel"
	"xorkevin.dev/governor/util/kjson"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

var (
	// ErrorNotFound is returned when an apikey is not found
	ErrorNotFound errorNotFound
	// ErrorInvalidKey is returned when an apikey is invalid
	ErrorInvalidKey errorInvalidKey
)

type (
	errorNotFound   struct{}
	errorInvalidKey struct{}
)

func (e errorNotFound) Error() string {
	return "Apikey not found"
}

func (e errorInvalidKey) Error() string {
	return "Invalid apikey"
}

const (
	cacheValTombstone = "-"
)

type (
	keyhashKVVal struct {
		Hash  string `json:"hash"`
		Scope string `json:"scope"`
	}
)

func (s *Service) GetUserKeys(ctx context.Context, userid string, limit, offset int) ([]apikeymodel.Model, error) {
	m, err := s.apikeys.GetUserKeys(ctx, userid, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get apikeys")
	}
	return m, nil
}

func (s *Service) getKeyHash(ctx context.Context, keyid string) (string, string, error) {
	if result, err := s.kvkey.Get(ctx, keyid); err != nil {
		if !errors.Is(err, kvstore.ErrorNotFound) {
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to get apikey key from cache"))
		}
	} else if result == cacheValTombstone {
		return "", "", kerrors.WithKind(nil, ErrorNotFound, "Apikey not found")
	} else {
		var kvVal keyhashKVVal
		if err := kjson.Unmarshal([]byte(result), &kvVal); err != nil {
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to decode apikey from cache"))
		} else {
			return kvVal.Hash, kvVal.Scope, nil
		}
	}

	m, err := s.apikeys.GetByID(ctx, keyid)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			if err := s.kvkey.Set(ctx, keyid, cacheValTombstone, s.scopeCacheDuration); err != nil {
				s.log.Err(ctx, kerrors.WithMsg(err, "Failed to set apikey key in cache"))
			}
			return "", "", kerrors.WithKind(err, ErrorNotFound, "Apikey not found")
		}
		return "", "", kerrors.WithMsg(err, "Failed to get apikey")
	}

	if kvVal, err := kjson.Marshal(keyhashKVVal{
		Hash:  m.KeyHash,
		Scope: m.Scope,
	}); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to marshal json for apikey"))
	} else if err := s.kvkey.Set(ctx, keyid, string(kvVal), s.scopeCacheDuration); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to set apikey key in cache"))
	}

	return m.KeyHash, m.Scope, nil
}

func (s *Service) CheckKey(ctx context.Context, keyid, key string) (string, string, error) {
	userid, err := apikeymodel.ParseIDUserid(keyid)
	if err != nil {
		return "", "", kerrors.WithKind(err, ErrorInvalidKey, "Invalid key")
	}

	keyhash, keyscope, err := s.getKeyHash(ctx, keyid)
	if err != nil {
		return "", "", err
	}

	m := apikeymodel.Model{
		KeyHash: keyhash,
	}
	if ok, err := s.apikeys.ValidateKey(key, &m); err != nil {
		return "", "", kerrors.WithMsg(err, "Failed to validate key")
	} else if !ok {
		return "", "", kerrors.WithKind(err, ErrorInvalidKey, "Invalid key")
	}
	return userid, keyscope, nil
}

type (
	// ResApikeyModel is apikey info
	ResApikeyModel struct {
		Keyid string `json:"keyid"`
		Key   string `json:"key"`
	}
)

func (s *Service) Insert(ctx context.Context, userid string, scope string, name, desc string) (*ResApikeyModel, error) {
	m, key, err := s.apikeys.New(userid, scope, name, desc)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to create apikey keys")
	}
	if err := s.apikeys.Insert(ctx, m); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to create apikey")
	}
	// must make a best effort to clear the cache
	ctx = klog.ExtendCtx(context.Background(), ctx)
	s.clearCache(ctx, m.Keyid)
	return &ResApikeyModel{
		Keyid: m.Keyid,
		Key:   key,
	}, nil
}

func (s *Service) RotateKey(ctx context.Context, keyid string) (*ResApikeyModel, error) {
	m, err := s.apikeys.GetByID(ctx, keyid)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			return nil, kerrors.WithKind(err, ErrorNotFound, "Apikey not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get apikey")
	}
	key, err := s.apikeys.RehashKey(ctx, m)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to rotate apikey")
	}
	// must make a best effort to clear the cache
	ctx = klog.ExtendCtx(context.Background(), ctx)
	s.clearCache(ctx, m.Keyid)
	return &ResApikeyModel{
		Keyid: m.Keyid,
		Key:   key,
	}, nil
}

func (s *Service) UpdateKey(ctx context.Context, keyid string, scope string, name, desc string) error {
	m, err := s.apikeys.GetByID(ctx, keyid)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			return kerrors.WithKind(err, ErrorNotFound, "Apikey not found")
		}
		return kerrors.WithMsg(err, "Failed to get apikey")
	}
	m.Scope = scope
	m.Name = name
	m.Desc = desc
	if err := s.apikeys.UpdateProps(ctx, m); err != nil {
		return kerrors.WithMsg(err, "Failed to update apikey")
	}
	// must make a best effort to clear the cache
	ctx = klog.ExtendCtx(context.Background(), ctx)
	s.clearCache(ctx, m.Keyid)
	return nil
}

func (s *Service) DeleteKey(ctx context.Context, keyid string) error {
	m, err := s.apikeys.GetByID(ctx, keyid)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			return kerrors.WithKind(err, ErrorNotFound, "Apikey not found")
		}
		return kerrors.WithMsg(err, "Failed to get apikey")
	}
	if err := s.apikeys.Delete(ctx, m); err != nil {
		return kerrors.WithMsg(err, "Failed to delete apikey")
	}
	// must make a best effort to clear the cache
	ctx = klog.ExtendCtx(context.Background(), ctx)
	s.clearCache(ctx, m.Keyid)
	return nil
}

func (s *Service) DeleteKeys(ctx context.Context, keyids []string) error {
	if len(keyids) == 0 {
		return nil
	}
	if err := s.apikeys.DeleteKeys(ctx, keyids); err != nil {
		return kerrors.WithMsg(err, "Failed to delete apikeys")
	}
	// must make a best effort to clear the cache
	ctx = klog.ExtendCtx(context.Background(), ctx)
	s.clearCache(ctx, keyids...)
	return nil
}

func (s *Service) clearCache(ctx context.Context, keyids ...string) {
	if err := s.kvkey.Del(ctx, keyids...); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to clear keys from cache"))
	}
}
