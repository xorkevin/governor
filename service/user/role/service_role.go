package role

import (
	"net/http"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/util/rank"
)

func (s *service) IntersectRoles(userid string, roles rank.Rank) (rank.Rank, error) {
	return s.roles.IntersectRoles(userid, roles)
}

func (s *service) InsertRoles(userid string, roles rank.Rank) error {
	if err := s.roles.InsertRoles(userid, roles); err != nil {
		return err
	}
	s.clearCache(userid)
	return nil
}

func (s *service) DeleteRoles(userid string, roles rank.Rank) error {
	if err := s.roles.DeleteRoles(userid, roles); err != nil {
		return err
	}
	s.clearCache(userid)
	return nil
}

func (s *service) DeleteAllRoles(userid string) error {
	if err := s.roles.DeleteUserRoles(userid); err != nil {
		return err
	}
	s.clearCache(userid)
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

func (s *service) clearCache(userid string) {
	if err := s.kvsummary.Del(userid); err != nil {
		s.logger.Error("Failed to clear role summary from cache", map[string]string{
			"error":      err.Error(),
			"actiontype": "clearcachesummary",
		})
	}
}
