package apikey

import (
	"encoding/json"
	"net/http"
	"strings"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/apikey/model"
)

const (
	cacheValTombstone = "-"
)

type (
	keyhashKVVal struct {
		Hash  string `json:"hash"`
		Scope string `json:"scope"`
	}
)

func (s *service) GetUserKeys(userid string, limit, offset int) ([]apikeymodel.Model, error) {
	return s.apikeys.GetUserKeys(userid, limit, offset)
}

func (s *service) useridFromKeyid(keyid string) (string, error) {
	k := strings.SplitN(keyid, "|", 2)
	if len(k) != 2 {
		return "", governor.NewError("Invalid apikey", http.StatusBadRequest, nil)
	}
	return k[0], nil
}

func (s *service) getKeyHash(keyid string) (string, string, error) {
	if result, err := s.kvkey.Get(keyid); err != nil {
		if governor.ErrorStatus(err) != http.StatusNotFound {
			s.logger.Error("Failed to get apikey key from cache", map[string]string{
				"error":      err.Error(),
				"actiontype": "getcacheapikey",
			})
		}
	} else if result == cacheValTombstone {
		return "", "", governor.NewError("Apikey not found", http.StatusNotFound, nil)
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
		if governor.ErrorStatus(err) == http.StatusNotFound {
			if err := s.kvkey.Set(keyid, cacheValTombstone, s.scopeCacheTime); err != nil {
				s.logger.Error("Failed to set apikey key in cache", map[string]string{
					"error":      err.Error(),
					"actiontype": "setcacheapikey",
				})
			}
		}
		return "", "", err
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
	userid, err := s.useridFromKeyid(keyid)
	if err != nil {
		return "", "", governor.NewError("Invalid key", http.StatusUnauthorized, nil)
	}

	keyhash, keyscope, err := s.getKeyHash(keyid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return "", "", governor.NewError("Invalid key", http.StatusUnauthorized, nil)
		}
		return "", "", err
	}

	m := apikeymodel.Model{
		KeyHash: keyhash,
	}
	if ok, err := s.apikeys.ValidateKey(key, &m); err != nil || !ok {
		return "", "", governor.NewError("Invalid key", http.StatusUnauthorized, nil)
	}
	return userid, keyscope, nil
}

type (
	ResApikeyModel struct {
		Keyid string `json:"keyid"`
		Key   string `json:"key"`
	}
)

func (s *service) Insert(userid string, scope string, name, desc string) (*ResApikeyModel, error) {
	m, key, err := s.apikeys.New(userid, scope, name, desc)
	if err != nil {
		return nil, err
	}
	if err := s.apikeys.Insert(m); err != nil {
		return nil, err
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
		return nil, err
	}
	key, err := s.apikeys.RehashKey(m)
	if err != nil {
		return nil, err
	}
	if err := s.apikeys.Update(m); err != nil {
		return nil, err
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
		return err
	}
	m.Scope = scope
	m.Name = name
	m.Desc = desc
	if err := s.apikeys.Update(m); err != nil {
		return err
	}
	s.clearCache(m.Keyid)
	return nil
}

func (s *service) DeleteKey(keyid string) error {
	m, err := s.apikeys.GetByID(keyid)
	if err != nil {
		return err
	}
	if err := s.apikeys.Delete(m); err != nil {
		return err
	}
	s.clearCache(m.Keyid)
	return nil
}

func (s *service) DeleteUserKeys(userid string) error {
	keys, err := s.GetUserKeys(userid, 65536, 0)
	if err != nil {
		return err
	}
	if err := s.apikeys.DeleteUserKeys(userid); err != nil {
		return err
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
