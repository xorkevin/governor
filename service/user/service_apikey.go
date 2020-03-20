package user

import (
	"net/http"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/util/rank"
)

type (
	resApikey struct {
		Keyid    string `json:"keyid"`
		AuthTags string `json:"auth_tags"`
		Name     string `json:"name"`
		Desc     string `json:"desc"`
		Time     int64  `json:"time"`
	}

	resApikeys struct {
		Apikeys []resApikey `json:"apikeys"`
	}
)

func (s *service) GetUserApikeys(userid string, limit, offset int) (*resApikeys, error) {
	m, err := s.apikeys.GetUserKeys(userid, limit, offset)
	if err != nil {
		return nil, governor.NewError("Failed to get apikeys", http.StatusInternalServerError, err)
	}
	res := make([]resApikey, 0, len(m))
	for _, i := range m {
		res = append(res, resApikey{
			Keyid:    i.Keyid,
			AuthTags: i.AuthTags.Stringify(),
			Name:     i.Name,
			Desc:     i.Desc,
			Time:     i.Time,
		})
	}
	return &resApikeys{
		Apikeys: res,
	}, nil
}

func (s *service) CheckApikey(keyid, key string, authtags rank.Rank) error {
	if _, err := s.apikeys.CheckKey(keyid, key, authtags); err != nil {
		if governor.ErrorStatus(err) == http.StatusUnauthorized {
			return governor.NewError("Invalid key", http.StatusUnauthorized, nil)
		}
		if governor.ErrorStatus(err) == http.StatusForbidden {
			return governor.NewError("User is forbidden", http.StatusForbidden, nil)
		}
		return governor.NewError("Failed to check apikey", http.StatusInternalServerError, err)
	}
	return nil
}

type (
	resApikeyModel struct {
		Keyid string `json:"keyid"`
		Key   string `json:"key"`
	}
)

func (s *service) CreateApikey(userid string, authtags rank.Rank, name, desc string) (*resApikeyModel, error) {
	m, err := s.apikeys.Insert(userid, authtags, name, desc)
	if err != nil {
		return nil, governor.NewError("Failed to create apikey", http.StatusInternalServerError, err)
	}
	return &resApikeyModel{
		Keyid: m.Keyid,
		Key:   m.Key,
	}, nil
}

func (s *service) RotateApikey(keyid string) (*resApikeyModel, error) {
	m, err := s.apikeys.RotateKey(keyid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return nil, governor.NewErrorUser("Apikey not found", http.StatusNotFound, err)
		}
		return nil, governor.NewError("Failed to rotate apikey", http.StatusInternalServerError, err)
	}
	return &resApikeyModel{
		Keyid: m.Keyid,
		Key:   m.Key,
	}, nil
}

func (s *service) UpdateApikey(keyid string, authtags rank.Rank, name, desc string) error {
	if err := s.apikeys.UpdateKey(keyid, authtags, name, desc); err != nil {
		if governor.ErrorStatus(err) == http.StatusBadRequest {
			return governor.NewErrorUser("Invalid apikey", http.StatusBadRequest, err)
		}
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return governor.NewErrorUser("Apikey not found", http.StatusNotFound, err)
		}
		return governor.NewError("Failed to update apikey", http.StatusInternalServerError, err)
	}
	return nil
}

func (s *service) DeleteApikey(keyid string) error {
	if err := s.apikeys.DeleteKey(keyid); err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return governor.NewErrorUser("Apikey not found", http.StatusNotFound, err)
		}
		return governor.NewError("Failed to delete apikey", http.StatusInternalServerError, err)
	}
	return nil
}

func (s *service) DeleteUserApikeys(userid string) error {
	if err := s.apikeys.DeleteUserKeys(userid); err != nil {
		return governor.NewError("Failed to delete user apikeys", http.StatusInternalServerError, err)
	}
	return nil
}
