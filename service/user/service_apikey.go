package user

import (
	"context"
	"errors"
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/gate/apikey"
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
		Apikeys []apikey.Props `json:"apikeys"`
	}
)

func (s *Service) getUserApikeys(ctx context.Context, userid string, limit, offset int) (*resApikeys, error) {
	m, err := s.apikeys.GetUserKeys(ctx, userid, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get apikeys")
	}
	return &resApikeys{
		Apikeys: m,
	}, nil
}

type (
	resApikeyModel struct {
		Keyid string `json:"keyid"`
		Key   string `json:"key"`
	}
)

func (s *Service) createApikey(ctx context.Context, userid string, scope string, name, desc string) (*resApikeyModel, error) {
	m, err := s.apikeys.InsertKey(ctx, userid, scope, name, desc)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to create apikey")
	}
	return &resApikeyModel{
		Keyid: m.Keyid,
		Key:   m.Key,
	}, nil
}

func (s *Service) rotateApikey(ctx context.Context, userid string, keyid string) (*resApikeyModel, error) {
	m, err := s.apikeys.RotateKey(ctx, userid, keyid)
	if err != nil {
		if errors.Is(err, apikey.ErrNotFound) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "Apikey not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to rotate apikey")
	}
	return &resApikeyModel{
		Keyid: m.Keyid,
		Key:   m.Key,
	}, nil
}

func (s *Service) updateApikey(ctx context.Context, userid string, keyid string, scope string, name, desc string) error {
	if err := s.apikeys.UpdateKey(ctx, userid, keyid, scope, name, desc); err != nil {
		if errors.Is(err, apikey.ErrNotFound) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "Apikey not found")
		}
		return kerrors.WithMsg(err, "Failed to update apikey")
	}
	return nil
}

func (s *Service) deleteApikey(ctx context.Context, userid string, keyid string) error {
	if err := s.apikeys.DeleteKey(ctx, userid, keyid); err != nil {
		if errors.Is(err, apikey.ErrNotFound) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "Apikey not found")
		}
		return kerrors.WithMsg(err, "Failed to delete apikey")
	}
	return nil
}
