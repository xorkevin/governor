package apikey

import (
	"encoding/json"
	"errors"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/user/apikey/model"
)

type (
	// ErrNotFound is returned when an apikey is not found
	ErrNotFound struct{}
	// ErrUnique is returned when an apikey already exists
	ErrUnique struct{}
	// ErrInvalidKey is returned when an apikey is invalid
	ErrInvalidKey struct{}
)

func (e ErrNotFound) Error() string {
	return "Apikey not found"
}

func (e ErrUnique) Error() string {
	return "Error apikey uniqueness"
}

func (e ErrInvalidKey) Error() string {
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

func (s *service) GetUserKeys(userid string, limit, offset int) ([]model.Model, error) {
	m, err := s.apikeys.GetUserKeys(userid, limit, offset)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get apikeys")
	}
	return m, nil
}

func (s *service) getKeyHash(keyid string) (string, string, error) {
	if result, err := s.kvkey.Get(keyid); err != nil {
		if !errors.Is(err, kvstore.ErrNotFound{}) {
			s.logger.Error("Failed to get apikey key from cache", map[string]string{
				"error":      err.Error(),
				"actiontype": "getcacheapikey",
			})
		}
	} else if result == cacheValTombstone {
		return "", "", governor.ErrWithKind(nil, ErrNotFound{}, "Apikey not found")
	} else {
		kvVal := keyhashKVVal{}
		if err := json.Unmarshal([]byte(result), &kvVal); err != nil {
			s.logger.Error("Failed to decode apikey from cache", map[string]string{
				"error":      err.Error(),
				"actiontype": "getcacheapikeydecode",
			})
		} else {
			return kvVal.Hash, kvVal.Scope, nil
		}
	}

	m, err := s.apikeys.GetByID(keyid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			if err := s.kvkey.Set(keyid, cacheValTombstone, s.scopeCacheTime); err != nil {
				s.logger.Error("Failed to set apikey key in cache", map[string]string{
					"error":      err.Error(),
					"actiontype": "setcacheapikey",
				})
			}
			return "", "", governor.ErrWithKind(err, ErrNotFound{}, "Apikey not found")
		}
		return "", "", governor.ErrWithMsg(err, "Failed to get apikey")
	}

	if kvVal, err := json.Marshal(keyhashKVVal{
		Hash:  m.KeyHash,
		Scope: m.Scope,
	}); err != nil {
		s.logger.Error("Failed to marshal json for apikey", map[string]string{
			"error":      err.Error(),
			"actiontype": "cacheapikeyencode",
		})
	} else if err := s.kvkey.Set(keyid, string(kvVal), s.scopeCacheTime); err != nil {
		s.logger.Error("Failed to set apikey key in cache", map[string]string{
			"error":      err.Error(),
			"actiontype": "setcacheapikey",
		})
	}

	return m.KeyHash, m.Scope, nil
}

func (s *service) CheckKey(keyid, key string) (string, string, error) {
	userid, err := model.ParseIDUserid(keyid)
	if err != nil {
		return "", "", governor.ErrWithKind(err, ErrInvalidKey{}, "Invalid key")
	}

	keyhash, keyscope, err := s.getKeyHash(keyid)
	if err != nil {
		return "", "", err
	}

	m := model.Model{
		KeyHash: keyhash,
	}
	if ok, err := s.apikeys.ValidateKey(key, &m); err != nil || !ok {
		return "", "", governor.ErrWithKind(err, ErrInvalidKey{}, "Invalid key")
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

func (s *service) Insert(userid string, scope string, name, desc string) (*ResApikeyModel, error) {
	m, key, err := s.apikeys.New(userid, scope, name, desc)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to create apikey keys")
	}
	if err := s.apikeys.Insert(m); err != nil {
		if errors.Is(err, db.ErrUnique{}) {
			return nil, governor.ErrWithKind(err, ErrUnique{}, "Failed to create apikey")
		}
		return nil, governor.ErrWithMsg(err, "Failed to create apikey")
	}
	s.clearCache(m.Keyid)
	return &ResApikeyModel{
		Keyid: m.Keyid,
		Key:   key,
	}, nil
}

func (s *service) RotateKey(keyid string) (*ResApikeyModel, error) {
	m, err := s.apikeys.GetByID(keyid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.ErrWithKind(err, ErrNotFound{}, "Apikey not found")
		}
		return nil, governor.ErrWithMsg(err, "Failed to get apikey")
	}
	key, err := s.apikeys.RehashKey(m)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to rotate apikey")
	}
	if err := s.apikeys.Update(m); err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to update apikey")
	}
	s.clearCache(m.Keyid)
	return &ResApikeyModel{
		Keyid: m.Keyid,
		Key:   key,
	}, nil
}

func (s *service) UpdateKey(keyid string, scope string, name, desc string) error {
	m, err := s.apikeys.GetByID(keyid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.ErrWithKind(err, ErrNotFound{}, "Apikey not found")
		}
		return governor.ErrWithMsg(err, "Failed to get apikey")
	}
	m.Scope = scope
	m.Name = name
	m.Desc = desc
	if err := s.apikeys.Update(m); err != nil {
		return governor.ErrWithMsg(err, "Failed to update apikey")
	}
	s.clearCache(m.Keyid)
	return nil
}

func (s *service) DeleteKey(keyid string) error {
	m, err := s.apikeys.GetByID(keyid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.ErrWithKind(err, ErrNotFound{}, "Apikey not found")
		}
		return governor.ErrWithMsg(err, "Failed to get apikey")
	}
	if err := s.apikeys.Delete(m); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete apikey")
	}
	s.clearCache(m.Keyid)
	return nil
}

func (s *service) DeleteUserKeys(userid string) error {
	keys, err := s.GetUserKeys(userid, 65536, 0)
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to get user keys")
	}
	if err := s.apikeys.DeleteUserKeys(userid); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete user keys")
	}

	if len(keys) == 0 {
		return nil
	}

	keyids := make([]string, 0, len(keys))
	for _, i := range keys {
		keyids = append(keyids, i.Keyid)
	}
	s.clearCache(keyids...)
	return nil
}

func (s *service) clearCache(keyids ...string) {
	if err := s.kvkey.Del(keyids...); err != nil {
		s.logger.Error("Failed to clear keys from cache", map[string]string{
			"error":      err.Error(),
			"actiontype": "clearcacheapikey",
		})
	}
}
