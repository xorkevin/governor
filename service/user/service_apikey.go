package user

import (
	"errors"
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/apikey"
)

type (
	resApikey struct {
		Keyid string `json:"keyid"`
		Scope string `json:"scope"`
		Name  string `json:"name"`
		Desc  string `json:"desc"`
		Time  int64  `json:"time"`
	}

	resApikeys struct {
		Apikeys []resApikey `json:"apikeys"`
	}
)

func (s *service) GetUserApikeys(userid string, limit, offset int) (*resApikeys, error) {
	m, err := s.apikeys.GetUserKeys(userid, limit, offset)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get apikeys")
	}
	res := make([]resApikey, 0, len(m))
	for _, i := range m {
		res = append(res, resApikey{
			Keyid: i.Keyid,
			Scope: i.Scope,
			Name:  i.Name,
			Desc:  i.Desc,
			Time:  i.Time,
		})
	}
	return &resApikeys{
		Apikeys: res,
	}, nil
}

type (
	resApikeyModel struct {
		Keyid string `json:"keyid"`
		Key   string `json:"key"`
	}
)

func (s *service) CreateApikey(userid string, scope string, name, desc string) (*resApikeyModel, error) {
	m, err := s.apikeys.Insert(userid, scope, name, desc)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to create apikey")
	}
	return &resApikeyModel{
		Keyid: m.Keyid,
		Key:   m.Key,
	}, nil
}

func (s *service) RotateApikey(keyid string) (*resApikeyModel, error) {
	m, err := s.apikeys.RotateKey(keyid)
	if err != nil {
		if errors.Is(err, apikey.ErrNotFound{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "Apikey not found",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to rotate apikey")
	}
	return &resApikeyModel{
		Keyid: m.Keyid,
		Key:   m.Key,
	}, nil
}

func (s *service) UpdateApikey(keyid string, scope string, name, desc string) error {
	if err := s.apikeys.UpdateKey(keyid, scope, name, desc); err != nil {
		if errors.Is(err, apikey.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "Apikey not found",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to update apikey")
	}
	return nil
}

func (s *service) DeleteApikey(keyid string) error {
	if err := s.apikeys.DeleteKey(keyid); err != nil {
		if errors.Is(err, apikey.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "Apikey not found",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to delete apikey")
	}
	return nil
}

func (s *service) DeleteUserApikeys(userid string) error {
	if err := s.apikeys.DeleteUserKeys(userid); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete user apikeys")
	}
	return nil
}
