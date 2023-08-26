package apikey

import (
	"context"
	"errors"

	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/gate/apikey/apikeymodel"
	"xorkevin.dev/kerrors"
)

var (
	// ErrNotFound is returned when an apikey is not found
	ErrNotFound errNotFound
	// ErrInvalidKey is returned when an apikey is invalid
	ErrInvalidKey errInvalidKey
)

type (
	errNotFound   struct{}
	errInvalidKey struct{}
)

func (e errNotFound) Error() string {
	return "Apikey not found"
}

func (e errInvalidKey) Error() string {
	return "Invalid apikey"
}

func (s *Service) GetUserKeys(ctx context.Context, userid string, limit, offset int) ([]apikeymodel.Model, error) {
	m, err := s.apikeys.GetUserKeys(ctx, userid, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get apikeys")
	}
	return m, nil
}

func (s *Service) CheckKey(ctx context.Context, keyid, key string) (string, string, error) {
	m, err := s.apikeys.GetByID(ctx, keyid)
	if err != nil {
		return "", "", err
	}

	if ok, err := s.apikeys.ValidateKey(key, m); err != nil {
		return "", "", kerrors.WithMsg(err, "Failed to validate key")
	} else if !ok {
		return "", "", kerrors.WithKind(err, ErrInvalidKey, "Invalid key")
	}
	return m.Userid, m.Scope, nil
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
	return &ResApikeyModel{
		Keyid: m.Keyid,
		Key:   key,
	}, nil
}

func (s *Service) RotateKey(ctx context.Context, userid string, keyid string) (*ResApikeyModel, error) {
	m, err := s.apikeys.GetByID(ctx, keyid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, kerrors.WithKind(err, ErrNotFound, "Apikey not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get apikey")
	}
	if userid != m.Userid {
		return nil, kerrors.WithKind(nil, ErrNotFound, "Apikey not found")
	}
	key, err := s.apikeys.RehashKey(ctx, m)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to rotate apikey")
	}
	return &ResApikeyModel{
		Keyid: m.Keyid,
		Key:   key,
	}, nil
}

func (s *Service) UpdateKey(ctx context.Context, userid string, keyid string, scope string, name, desc string) error {
	m, err := s.apikeys.GetByID(ctx, keyid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return kerrors.WithKind(err, ErrNotFound, "Apikey not found")
		}
		return kerrors.WithMsg(err, "Failed to get apikey")
	}
	if userid != m.Userid {
		return kerrors.WithKind(nil, ErrNotFound, "Apikey not found")
	}
	m.Scope = scope
	m.Name = name
	m.Desc = desc
	if err := s.apikeys.UpdateProps(ctx, m); err != nil {
		return kerrors.WithMsg(err, "Failed to update apikey")
	}
	return nil
}

func (s *Service) DeleteKey(ctx context.Context, userid string, keyid string) error {
	m, err := s.apikeys.GetByID(ctx, keyid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return kerrors.WithKind(err, ErrNotFound, "Apikey not found")
		}
		return kerrors.WithMsg(err, "Failed to get apikey")
	}
	if userid != m.Userid {
		return kerrors.WithKind(nil, ErrNotFound, "Apikey not found")
	}
	if err := s.apikeys.Delete(ctx, m); err != nil {
		return kerrors.WithMsg(err, "Failed to delete apikey")
	}
	return nil
}

func (s *Service) DeleteKeys(ctx context.Context, keyids []string) error {
	if len(keyids) == 0 {
		return nil
	}
	if err := s.apikeys.DeleteKeys(ctx, keyids); err != nil {
		return kerrors.WithMsg(err, "Failed to delete apikeys")
	}
	return nil
}
