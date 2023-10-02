package apikey

import (
	"context"
	"errors"
	"fmt"

	"xorkevin.dev/governor/service/dbsql"
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

func (s *Service) GetUserKeys(ctx context.Context, userid string, limit, offset int) ([]Props, error) {
	m, err := s.apikeys.GetUserKeys(ctx, userid, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get apikeys")
	}
	res := make([]Props, 0, len(m))
	for _, i := range m {
		res = append(res, Props{
			Keyid:        i.Keyid,
			Userid:       i.Userid,
			Scope:        i.Scope,
			Name:         i.Name,
			Desc:         i.Desc,
			RotateTime:   i.RotateTime,
			UpdateTime:   i.UpdateTime,
			CreationTime: i.CreationTime,
		})
	}
	return res, nil
}

func (s *Service) Check(ctx context.Context, keyid, key string) (*UserScope, error) {
	m, err := s.apikeys.GetByID(ctx, keyid)
	if err != nil {
		return nil, err
	}

	if ok, err := s.apikeys.ValidateKey(key, m); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to validate key")
	} else if !ok {
		return nil, kerrors.WithKind(err, ErrInvalidKey, "Invalid key")
	}
	return &UserScope{
		Userid: m.Userid,
		Scope:  m.Scope,
	}, nil
}

func (s *Service) InsertKey(ctx context.Context, userid string, scope string, name, desc string) (*Key, error) {
	m, key, err := s.apikeys.New(userid, scope, name, desc)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to create apikey keys")
	}
	if err := s.apikeys.Insert(ctx, m); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to create apikey")
	}
	return &Key{
		Keyid: m.Keyid,
		Key:   fmt.Sprintf("ga.%s.%s", m.Keyid, key),
	}, nil
}

func (s *Service) RotateKey(ctx context.Context, userid string, keyid string) (*Key, error) {
	m, err := s.apikeys.GetByID(ctx, keyid)
	if err != nil {
		if errors.Is(err, dbsql.ErrNotFound) {
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
	return &Key{
		Keyid: m.Keyid,
		Key:   fmt.Sprintf("ga.%s.%s", m.Keyid, key),
	}, nil
}

func (s *Service) UpdateKey(ctx context.Context, userid string, keyid string, scope string, name, desc string) error {
	m, err := s.apikeys.GetByID(ctx, keyid)
	if err != nil {
		if errors.Is(err, dbsql.ErrNotFound) {
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
		if errors.Is(err, dbsql.ErrNotFound) {
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

func (s *Service) DeleteUserKeys(ctx context.Context, userid string) error {
	if err := s.apikeys.DeleteUserKeys(ctx, userid); err != nil {
		return kerrors.WithMsg(err, "Failed to delete apikeys")
	}
	return nil
}
