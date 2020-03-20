package apikey

import (
	"net/http"
	"strings"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/apikey/model"
	"xorkevin.dev/governor/util/rank"
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

func (s *service) CheckKey(keyid, key string, authtags rank.Rank) (string, error) {
	userid, err := s.useridFromKeyid(keyid)
	if err != nil {
		return "", governor.NewError("Invalid key", http.StatusUnauthorized, err)
	}

	m, err := s.apikeys.GetByID(keyid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return "", governor.NewError("Invalid key", http.StatusUnauthorized, nil)
		}
		return "", err
	}

	if ok, err := s.apikeys.ValidateKey(key, m); err != nil || !ok {
		return "", governor.NewError("Invalid key", http.StatusUnauthorized, nil)
	}

	if authtags == nil || authtags.Len() == 0 {
		return userid, nil
	}

	if inter := m.AuthTags.Intersect(authtags); inter.Len() != authtags.Len() {
		return "", governor.NewError("User is forbidden", http.StatusForbidden, nil)
	}
	if inter, err := s.roles.IntersectRoles(userid, authtags); err != nil {
		return "", governor.NewError("Unable to get user roles", http.StatusInternalServerError, err)
	} else if inter.Len() != authtags.Len() {
		return "", governor.NewError("User is forbidden", http.StatusForbidden, nil)
	}
	return userid, nil
}

func (s *service) IntersectRoles(keyid string, authtags rank.Rank) (rank.Rank, error) {
	userid, err := s.useridFromKeyid(keyid)
	if err != nil {
		return nil, governor.NewError("Invalid key", http.StatusBadRequest, err)
	}

	m, err := s.apikeys.GetByID(keyid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return nil, governor.NewError("Apikey not found", http.StatusNotFound, nil)
		}
		return nil, err
	}

	inter, err := s.roles.IntersectRoles(userid, m.AuthTags.Intersect(authtags))
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
	return &ResApikeyModel{
		Keyid: m.Keyid,
		Key:   key,
	}, nil
}

func (s *service) UpdateKey(keyid string, authtags rank.Rank, name, desc string) error {
	userid, err := s.useridFromKeyid(keyid)
	if err != nil {
		return governor.NewError("Invalid apikey", http.StatusBadRequest, err)
	}

	m, err := s.apikeys.GetByID(keyid)
	if err != nil {
		return err
	}
	intersect, err := s.roles.IntersectRoles(userid, authtags)
	if err != nil {
		return err
	}
	m.AuthTags = intersect
	m.Name = name
	m.Desc = desc
	if err := s.apikeys.Update(m); err != nil {
		return err
	}
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
	return nil
}

func (s *service) DeleteUserKeys(userid string) error {
	if err := s.apikeys.DeleteUserKeys(userid); err != nil {
		return err
	}
	return nil
}
