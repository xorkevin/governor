package user

import (
	"context"
	"errors"
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/apikey"
	"xorkevin.dev/kerrors"
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

func (s *Service) getUserApikeys(ctx context.Context, userid string, limit, offset int) (*resApikeys, error) {
	m, err := s.apikeys.GetUserKeys(ctx, userid, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get apikeys")
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

func (s *Service) createApikey(ctx context.Context, userid string, scope string, name, desc string) (*resApikeyModel, error) {
	m, err := s.apikeys.Insert(ctx, userid, scope, name, desc)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to create apikey")
	}
	return &resApikeyModel{
		Keyid: m.Keyid,
		Key:   m.Key,
	}, nil
}

func (s *Service) rotateApikey(ctx context.Context, keyid string) (*resApikeyModel, error) {
	m, err := s.apikeys.RotateKey(ctx, keyid)
	if err != nil {
		if errors.Is(err, apikey.ErrorNotFound) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "Apikey not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to rotate apikey")
	}
	return &resApikeyModel{
		Keyid: m.Keyid,
		Key:   m.Key,
	}, nil
}

func (s *Service) updateApikey(ctx context.Context, keyid string, scope string, name, desc string) error {
	if err := s.apikeys.UpdateKey(ctx, keyid, scope, name, desc); err != nil {
		if errors.Is(err, apikey.ErrorNotFound) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "Apikey not found")
		}
		return kerrors.WithMsg(err, "Failed to update apikey")
	}
	return nil
}

func (s *Service) deleteApikey(ctx context.Context, keyid string) error {
	if err := s.apikeys.DeleteKey(ctx, keyid); err != nil {
		if errors.Is(err, apikey.ErrorNotFound) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "Apikey not found")
		}
		return kerrors.WithMsg(err, "Failed to delete apikey")
	}
	return nil
}
