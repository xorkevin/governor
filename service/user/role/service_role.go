package role

import (
	"net/http"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/util/rank"
)

const (
	cacheValY = "y"
	cacheValN = "n"
)

func (s *service) intersectRolesRepo(userid string, roles rank.Rank) (rank.Rank, error) {
	return s.roles.IntersectRoles(userid, roles)
}

func (s *service) IntersectRoles(userid string, roles rank.Rank) (rank.Rank, error) {
	userkv := s.kvroleset.Subtree(userid)

	txget := userkv.Tx()
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
			if governor.ErrorStatus(err) != http.StatusNotFound {
				s.logger.Error("Failed to get user role result from cache", map[string]string{
					"error":      err.Error(),
					"actiontype": "getroleresult",
				})
			}
			uncachedRoles.AddOne(k)
		} else {
			if r == cacheValY {
				res.AddOne(k)
			}
		}
	}

	if len(uncachedRoles) == 0 {
		return res, nil
	}

	m, err := s.intersectRolesRepo(userid, uncachedRoles)
	if err != nil {
		return nil, err
	}

	txset := userkv.Tx()
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
		return err
	}
	s.clearCache(userid, roles)
	return nil
}

func (s *service) DeleteRoles(userid string, roles rank.Rank) error {
	if err := s.roles.DeleteRoles(userid, roles); err != nil {
		return err
	}
	s.clearCache(userid, roles)
	return nil
}

func (s *service) DeleteAllRoles(userid string) error {
	roles, err := s.GetRoles(userid, 65536, 0)
	if err != nil {
		return err
	}
	if err := s.roles.DeleteUserRoles(userid); err != nil {
		return err
	}
	s.clearCache(userid, roles)
	return nil
}

func (s *service) GetRoles(userid string, amount, offset int) (rank.Rank, error) {
	return s.roles.GetRoles(userid, amount, offset)
}

func (s *service) GetByRole(roleName string, amount, offset int) ([]string, error) {
	return s.roles.GetByRole(roleName, amount, offset)
}

const (
	roleLimit = 256
)

func (s *service) getRoleSummaryRepo(userid string) (rank.Rank, error) {
	roles, err := s.GetRoles(userid, roleLimit, 0)
	if err != nil {
		return nil, err
	}
	if err := s.kvsummary.Set(userid, roles.Stringify(), s.roleCacheTime); err != nil {
		s.logger.Error("Failed to cache role summary", map[string]string{
			"error":      err.Error(),
			"actiontype": "cachesummary",
		})
	}
	return roles, nil
}

func (s *service) GetRoleSummary(userid string) (rank.Rank, error) {
	k, err := s.kvsummary.Get(userid)
	if err != nil {
		if governor.ErrorStatus(err) != http.StatusNotFound {
			s.logger.Error("Failed to get role summary from cache", map[string]string{
				"error":      err.Error(),
				"actiontype": "getcachesummary",
			})
		}
		return s.getRoleSummaryRepo(userid)
	}
	roles, err := rank.FromStringUser(k)
	if err != nil {
		s.logger.Error("Invalid role summary", map[string]string{
			"error":      err.Error(),
			"actiontype": "parsecachesummary",
		})
		return s.getRoleSummaryRepo(userid)
	}
	return roles, nil
}

func (s *service) clearCache(userid string, roles rank.Rank) {
	if err := s.kvsummary.Del(userid); err != nil {
		s.logger.Error("Failed to clear role summary from cache", map[string]string{
			"error":      err.Error(),
			"actiontype": "clearcachesummary",
		})
	}

	if len(roles) == 0 {
		return
	}

	tx := s.kvroleset.Subtree(userid).Tx()
	for _, i := range roles.ToSlice() {
		tx.Del(i)
	}
	if err := tx.Exec(); err != nil {
		s.logger.Error("Failed to clear role set from cache", map[string]string{
			"error":      err.Error(),
			"actiontype": "clearroleset",
		})
	}
}
