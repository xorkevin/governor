package role

import (
	"errors"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/util/rank"
)

const (
	cacheValY = "y"
	cacheValN = "n"
)

func (s *service) intersectRolesRepo(userid string, roles rank.Rank) (rank.Rank, error) {
	m, err := s.roles.IntersectRoles(userid, roles)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get roles")
	}
	return m, nil
}

func (s *service) IntersectRoles(userid string, roles rank.Rank) (rank.Rank, error) {
	userkv := s.kvroleset.Subtree(userid)

	txget, err := userkv.Tx()
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to create kvstore transaction")
	}
	resget := make(map[string]kvstore.Resulter, roles.Len())
	for _, i := range roles.ToSlice() {
		resget[i] = txget.Get(i)
	}
	if err := txget.Exec(); err != nil {
		s.logger.Error("Failed to get user roles from cache", map[string]string{
			"error":      err.Error(),
			"actiontype": "getroleset",
		})
		return s.intersectRolesRepo(userid, roles)
	}

	uncachedRoles := rank.Rank{}
	res := rank.Rank{}
	for k, v := range resget {
		r, err := v.Result()
		if err != nil {
			if !errors.Is(err, kvstore.ErrNotFound{}) {
				s.logger.Error("Failed to get user role result from cache", map[string]string{
					"error":      err.Error(),
					"actiontype": "getroleresult",
				})
			}
			uncachedRoles.AddOne(k)
		} else if r == cacheValY {
			res.AddOne(k)
		}
	}

	if len(uncachedRoles) == 0 {
		return res, nil
	}

	m, err := s.intersectRolesRepo(userid, uncachedRoles)
	if err != nil {
		return nil, err
	}

	txset, err := userkv.Tx()
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to create kvstore transaction")
	}
	for _, i := range uncachedRoles.ToSlice() {
		if m.Has(i) {
			res.AddOne(i)
			txset.Set(i, cacheValY, s.roleCacheTime)
		} else {
			txset.Set(i, cacheValN, s.roleCacheTime)
		}
	}
	if err := txset.Exec(); err != nil {
		s.logger.Error("Failed to set user roles in cache", map[string]string{
			"error":      err.Error(),
			"actiontype": "setroleset",
		})
	}

	return res, nil
}

func (s *service) InsertRoles(userid string, roles rank.Rank) error {
	if err := s.roles.InsertRoles(userid, roles); err != nil {
		return governor.ErrWithMsg(err, "Failed to create roles")
	}
	s.clearCache(userid, roles)
	return nil
}

func (s *service) DeleteRoles(userid string, roles rank.Rank) error {
	if err := s.roles.DeleteRoles(userid, roles); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete roles")
	}
	s.clearCache(userid, roles)
	return nil
}

func (s *service) DeleteAllRoles(userid string) error {
	roles, err := s.GetRoles(userid, "", 65536, 0)
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to get user roles")
	}
	if err := s.roles.DeleteUserRoles(userid); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete user roles")
	}
	s.clearCache(userid, roles)
	return nil
}

func (s *service) GetRoles(userid string, prefix string, amount, offset int) (rank.Rank, error) {
	if len(prefix) == 0 {
		return s.roles.GetRoles(userid, amount, offset)
	}
	return s.roles.GetRolesPrefix(userid, prefix, amount, offset)
}

func (s *service) GetByRole(roleName string, amount, offset int) ([]string, error) {
	return s.roles.GetByRole(roleName, amount, offset)
}

func (s *service) DeleteByRole(roleName string) error {
	userids, err := s.GetByRole(roleName, 65536, 0)
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to get role users")
	}
	if err := s.roles.DeleteByRole(roleName); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete role users")
	}
	s.clearCacheRoles(roleName, userids)
	return nil
}

func (s *service) clearCache(userid string, roles rank.Rank) {
	if len(roles) == 0 {
		return
	}
	if err := s.kvroleset.Subtree(userid).Del(roles.ToSlice()...); err != nil {
		s.logger.Error("Failed to clear role set from cache", map[string]string{
			"error":      err.Error(),
			"actiontype": "clearroleset",
		})
	}
}

func (s *service) clearCacheRoles(role string, userids []string) {
	if len(userids) == 0 {
		return
	}
	args := make([]string, 0, len(userids))
	for _, i := range userids {
		args = append(args, s.kvroleset.Subkey(i, role))
	}
	if err := s.kvroleset.Del(args...); err != nil {
		s.logger.Error("Failed to clear role set from cache", map[string]string{
			"error":      err.Error(),
			"actiontype": "clearroleset",
		})
	}
}
