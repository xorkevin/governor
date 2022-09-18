package role

import (
	"context"
	"errors"

	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/util/kjson"
	"xorkevin.dev/governor/util/rank"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
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
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to create kvstore multi"), nil)
	} else {
		resget := make(map[string]kvstore.Resulter, roles.Len())
		for _, i := range roles.ToSlice() {
			resget[i] = multiget.Get(ctx, i)
		}
		if err := multiget.Exec(ctx); err != nil {
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to get user roles from cache"), nil)
			goto end
		}
		uncachedRoles = rank.Rank{}
		for k, v := range resget {
			v, err := v.Result()
			if err != nil {
				if !errors.Is(err, kvstore.ErrorNotFound{}) {
					s.log.Err(ctx, kerrors.WithMsg(err, "Failed to get user role result from cache"), nil)
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
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to create kvstore multi"), nil)
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
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to set user roles in cache"), nil)
	}

	return res, nil
}

func (s *service) InsertRoles(ctx context.Context, userid string, roles rank.Rank) error {
	b, err := kjson.Marshal(RolesProps{
		Userid: userid,
		Roles:  roles.ToSlice(),
	})
	if err != nil {
		return kerrors.WithMsg(err, "Failed to encode roles props to json")
	}
	if err := s.roles.InsertRoles(ctx, userid, roles); err != nil {
		return kerrors.WithMsg(err, "Failed to create roles")
	}
	// must make a best effort to clear the cache and publish role event
	ctx = klog.ExtendCtx(context.Background(), ctx, nil)
	s.clearCache(ctx, userid, roles)
	if err := s.events.StreamPublish(ctx, s.opts.CreateChannel, b); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to publish new roles event"), nil)
	}
	return nil
}

func (s *service) DeleteRoles(ctx context.Context, userid string, roles rank.Rank) error {
	b, err := kjson.Marshal(RolesProps{
		Userid: userid,
		Roles:  roles.ToSlice(),
	})
	if err != nil {
		return kerrors.WithMsg(err, "Failed to encode roles props to json")
	}
	if err := s.events.StreamPublish(ctx, s.opts.DeleteChannel, b); err != nil {
		return kerrors.WithMsg(err, "Failed to publish delete roles event")
	}
	if err := s.roles.DeleteRoles(ctx, userid, roles); err != nil {
		return kerrors.WithMsg(err, "Failed to delete roles")
	}
	// must make a best effort to clear the cache
	ctx = klog.ExtendCtx(context.Background(), ctx, nil)
	s.clearCache(ctx, userid, roles)
	return nil
}

func (s *service) DeleteByRole(ctx context.Context, roleName string, userids []string) error {
	if len(userids) == 0 {
		return nil
	}
	if err := s.roles.DeleteByRole(ctx, roleName, userids); err != nil {
		return kerrors.WithMsg(err, "Failed to delete role users")
	}
	// must make a best effort to clear the cache
	ctx = klog.ExtendCtx(context.Background(), ctx, nil)
	s.clearCacheRoles(ctx, roleName, userids)
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

func (s *service) clearCache(ctx context.Context, userid string, roles rank.Rank) {
	if len(roles) == 0 {
		return
	}
	if err := s.kvroleset.Subtree(userid).Del(ctx, roles.ToSlice()...); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to clear role set from cache"), nil)
	}
}

func (s *service) clearCacheRoles(ctx context.Context, role string, userids []string) {
	if len(userids) == 0 {
		return
	}
	args := make([]string, 0, len(userids))
	for _, i := range userids {
		args = append(args, s.kvroleset.Subkey(i, role))
	}
	if err := s.kvroleset.Del(ctx, args...); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to clear role set from cache"), nil)
	}
}
