package user

import (
	"net/http"
	"xorkevin.dev/governor"
)

const (
	roleLimit = 256
)

func (s *service) GetUserRoles(userid string) (string, error) {
	roles, err := s.kvroles.Get(userid)
	if err != nil {
		if governor.ErrorStatus(err) != http.StatusNotFound {
			return "", governor.NewError("Failed to get user roles", http.StatusInternalServerError, err)
		}
		roles, err := s.roles.GetRoles(userid, roleLimit, 0)
		if err != nil {
			return "", err
		}
		k := roles.Stringify()
		if err := s.kvroles.Set(userid, k, s.roleCacheTime); err != nil {
			return "", governor.NewError("Failed to cache user roles", http.StatusInternalServerError, err)
		}
		return roles.Stringify(), nil
	}
	return roles, nil
}

func (s *service) DeleteCachedUserRoles(userid string) error {
	if err := s.kvroles.Del(userid); err != nil {
		return governor.NewError("Failed to delete user roles", http.StatusInternalServerError, err)
	}
	return nil
}

func (s *service) DeleteUserRoles(userid string) error {
	if err := s.DeleteCachedUserRoles(userid); err != nil {
		return err
	}
	if err := s.roles.DeleteUserRoles(userid); err != nil {
		return err
	}
	return nil
}
