package role

import (
	"context"
	"encoding/json"
	"errors"

	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/util/rank"
	"xorkevin.dev/kerrors"
)

const (
	cacheValY = "y"
	cacheValN = "n"
)

func (s *service) intersectRolesRepo(ctx context.Context, userid string, roles rank.Rank) (rank.Rank, error) {
	m, err := s.roles.IntersectRoles(ctx, userid, roles)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get roles")
	}
	return m, nil
}

func (s *service) IntersectRoles(ctx context.Context, userid string, roles rank.Rank) (rank.Rank, error) {
	userkv := s.kvroleset.Subtree(userid)

	res := rank.Rank{}
	uncachedRoles := roles

	if multiget, err := userkv.Multi(ctx); err != nil {
		s.logger.Error("Failed to create kvstore multi", map[string]string{
			"error": err.Error(),
		})
	} else {
		resget := make(map[string]kvstore.Resulter, roles.Len())
		for _, i := range roles.ToSlice() {
			resget[i] = multiget.Get(ctx, i)
		}
		if err := multiget.Exec(ctx); err != nil {
			s.logger.Error("Failed to get user roles from cache", map[string]string{
				"error":      err.Error(),
				"actiontype": "getroleset",
			})
			goto end
		}
		uncachedRoles = rank.Rank{}
		for k, v := range resget {
			v, err := v.Result()
			if err != nil {
				if !errors.Is(err, kvstore.ErrNotFound{}) {
					s.logger.Error("Failed to get user role result from cache", map[string]string{
						"error":      err.Error(),
						"actiontype": "getroleresult",
					})
				}
				uncachedRoles.AddOne(k)
			} else {
				if v == cacheValY {
					res.AddOne(k)
				}
			}
		}
	}

end:
	if len(uncachedRoles) == 0 {
		return res, nil
	}

	m, err := s.intersectRolesRepo(ctx, userid, uncachedRoles)
	if err != nil {
		return nil, err
	}

	for _, i := range m.ToSlice() {
		res.AddOne(i)
	}

	multiset, err := userkv.Multi(ctx)
	if err != nil {
		s.logger.Error("Failed to create kvstore multi", map[string]string{
			"error": err.Error(),
		})
		return res, nil
	}
	for _, i := range uncachedRoles.ToSlice() {
		if m.Has(i) {
			multiset.Set(ctx, i, cacheValY, s.roleCacheTime)
		} else {
			multiset.Set(ctx, i, cacheValN, s.roleCacheTime)
		}
	}
	if err := multiset.Exec(ctx); err != nil {
		s.logger.Error("Failed to set user roles in cache", map[string]string{
			"error":      err.Error(),
			"actiontype": "setroleset",
		})
	}

	return res, nil
}

func (s *service) InsertRoles(ctx context.Context, userid string, roles rank.Rank) error {
	b, err := json.Marshal(RolesProps{
		Userid: userid,
		Roles:  roles.ToSlice(),
	})
	if err != nil {
		return kerrors.WithMsg(err, "Failed to encode roles props to json")
	}
	if err := s.roles.InsertRoles(ctx, userid, roles); err != nil {
		return kerrors.WithMsg(err, "Failed to create roles")
	}
	s.clearCache(userid, roles)
	if err := s.events.StreamPublish(ctx, s.opts.CreateChannel, b); err != nil {
		s.logger.Error("Failed to publish new roles event", map[string]string{
			"error":      err.Error(),
			"actiontype": "publishnewroles",
		})
	}
	return nil
}

func (s *service) DeleteRoles(ctx context.Context, userid string, roles rank.Rank) error {
	b, err := json.Marshal(RolesProps{
		Userid: userid,
		Roles:  roles.ToSlice(),
	})
	if err != nil {
		return kerrors.WithMsg(err, "Failed to encode roles props to json")
	}
	if err := s.roles.DeleteRoles(ctx, userid, roles); err != nil {
		return kerrors.WithMsg(err, "Failed to delete roles")
	}
	s.clearCache(userid, roles)
	if err := s.events.StreamPublish(ctx, s.opts.DeleteChannel, b); err != nil {
		s.logger.Error("Failed to publish delete roles event", map[string]string{
			"error":      err.Error(),
			"actiontype": "publishdelroles",
		})
	}
	return nil
}

func (s *service) DeleteByRole(ctx context.Context, roleName string, userids []string) error {
	if len(userids) == 0 {
		return nil
	}
	if err := s.roles.DeleteByRole(ctx, roleName, userids); err != nil {
		return kerrors.WithMsg(err, "Failed to delete role users")
	}
	s.clearCacheRoles(roleName, userids)
	return nil
}

func (s *service) GetRoles(ctx context.Context, userid string, prefix string, amount, offset int) (rank.Rank, error) {
	if len(prefix) == 0 {
		return s.roles.GetRoles(ctx, userid, amount, offset)
	}
	return s.roles.GetRolesPrefix(ctx, userid, prefix, amount, offset)
}

func (s *service) GetByRole(ctx context.Context, roleName string, amount, offset int) ([]string, error) {
	return s.roles.GetByRole(ctx, roleName, amount, offset)
}

func (s *service) clearCache(userid string, roles rank.Rank) {
	if len(roles) == 0 {
		return
	}
	// must make a best effort to clear the cache
	if err := s.kvroleset.Subtree(userid).Del(context.Background(), roles.ToSlice()...); err != nil {
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
	// must make a best effort to clear the cache
	if err := s.kvroleset.Del(context.Background(), args...); err != nil {
		s.logger.Error("Failed to clear role set from cache", map[string]string{
			"error":      err.Error(),
			"actiontype": "clearroleset",
		})
	}
}
