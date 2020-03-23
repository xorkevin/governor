package apikey

import (
	"net/http"
	"strings"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/apikey/model"
	"xorkevin.dev/governor/util/rank"
)

const (
	cacheValDNE = "-"
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

func (s *service) getKeyHash(keyid string) (string, error) {
	if keyhash, err := s.kvkey.Get(keyid); err != nil {
		if governor.ErrorStatus(err) != http.StatusNotFound {
			s.logger.Error("Failed to get apikey key from cache", map[string]string{
				"error":      err.Error(),
				"actiontype": "getcacheapikey",
			})
		}
	} else if keyhash == cacheValDNE {
		return "", governor.NewError("Apikey not found", http.StatusNotFound, nil)
	} else {
		return keyhash, nil
	}

	m, err := s.apikeys.GetByID(keyid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			if err := s.kvkey.Set(keyid, cacheValDNE, s.roleCacheTime); err != nil {
				s.logger.Error("Failed to set apikey key in cache", map[string]string{
					"error":      err.Error(),
					"actiontype": "setcacheapikey",
				})
			}
		}
		return "", err
	}

	if err := s.kvkey.Set(keyid, m.KeyHash, s.roleCacheTime); err != nil {
		s.logger.Error("Failed to set apikey key in cache", map[string]string{
			"error":      err.Error(),
			"actiontype": "setcacheapikey",
		})
	}

	return m.KeyHash, nil
}

func (s *service) getRoleset(keyid string) (rank.Rank, error) {
	if rolestring, err := s.kvroleset.Get(keyid); err != nil {
		if governor.ErrorStatus(err) != http.StatusNotFound {
			s.logger.Error("Failed to get apikey roleset from cache", map[string]string{
				"error":      err.Error(),
				"actiontype": "getcacheroleset",
			})
		}
	} else if rolestring == cacheValDNE {
		return nil, governor.NewError("Apikey not found", http.StatusNotFound, nil)
	} else {
		k, _ := rank.FromStringUser(rolestring)
		return k, nil
	}

	m, err := s.apikeys.GetByID(keyid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			if err := s.kvroleset.Set(keyid, cacheValDNE, s.roleCacheTime); err != nil {
				s.logger.Error("Failed to set apikey roleset in cache", map[string]string{
					"error":      err.Error(),
					"actiontype": "setcacheroleset",
				})
			}
		}
		return nil, err
	}

	if err := s.kvroleset.Set(keyid, m.AuthTags.Stringify(), s.roleCacheTime); err != nil {
		s.logger.Error("Failed to set apikey roleset in cache", map[string]string{
			"error":      err.Error(),
			"actiontype": "setcacheroleset",
		})
	}

	return m.AuthTags, nil
}

func (s *service) CheckKey(keyid, key string) (string, error) {
	userid, err := s.useridFromKeyid(keyid)
	if err != nil {
		return "", governor.NewError("Invalid key", http.StatusUnauthorized, nil)
	}

	keyhash, err := s.getKeyHash(keyid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return "", governor.NewError("Invalid key", http.StatusUnauthorized, nil)
		}
		return "", err
	}

	m := apikeymodel.Model{
		KeyHash: keyhash,
	}
	if ok, err := s.apikeys.ValidateKey(key, &m); err != nil || !ok {
		return "", governor.NewError("Invalid key", http.StatusUnauthorized, nil)
	}
	return userid, nil
}

func (s *service) IntersectRoles(keyid string, authtags rank.Rank) (rank.Rank, error) {
	userid, err := s.useridFromKeyid(keyid)
	if err != nil {
		return nil, governor.NewError("Invalid key", http.StatusBadRequest, err)
	}

	m, err := s.getRoleset(keyid)
	if err != nil {
		return nil, err
	}

	inter, err := s.roles.IntersectRoles(userid, m.Intersect(authtags))
	if err != nil {
		return nil, governor.NewError("Unable to get user roles", http.StatusInternalServerError, err)
	}
	return inter, nil
}

type (
	ResApikeyModel struct {
		Keyid string `json:"keyid"`
		Key   string `json:"key"`
	}
)

func (s *service) Insert(userid string, authtags rank.Rank, name, desc string) (*ResApikeyModel, error) {
	intersect, err := s.roles.IntersectRoles(userid, authtags)
	if err != nil {
		return nil, err
	}
	m, key, err := s.apikeys.New(userid, intersect, name, desc)
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

func (s *service) UpdateKey(keyid string, authtags rank.Rank, name, desc string) error {
	userid, err := s.useridFromKeyid(keyid)
	if err != nil {
		return err
	}

	m, err := s.apikeys.GetByID(keyid)
	if err != nil {
		return err
	}
	intersect, err := s.roles.IntersectRoles(userid, authtags)
	if err != nil {
		return governor.NewError("Unable to get user roles", http.StatusInternalServerError, err)
	}
	m.AuthTags = intersect
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
	if err := s.kvroleset.Del(keyids...); err != nil {
		s.logger.Error("Failed to clear rolesets from cache", map[string]string{
			"error":      err.Error(),
			"actiontype": "clearcacheroleset",
		})
	}
}
