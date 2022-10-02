package user

import (
	"context"

	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/util/rank"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

func (s *Service) GetRoleUsers(ctx context.Context, roleName string, amount, offset int) ([]string, error) {
	userids, err := s.roles.GetByRole(ctx, roleName, amount, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get role users")
	}
	return userids, nil
}

func (s *Service) InsertRoles(ctx context.Context, userid string, roles rank.Rank) error {
	if len(roles) == 0 {
		return nil
	}

	b, err := encodeUserEventRoles(RolesProps{
		Add:    true,
		Userid: userid,
		Roles:  roles.ToSlice(),
	})
	if err != nil {
		return err
	}

	if err := s.roles.InsertRoles(ctx, userid, roles); err != nil {
		return kerrors.WithMsg(err, "Failed to insert user roles")
	}

	// must make a best effort attempt to publish role update events
	ctx = klog.ExtendCtx(context.Background(), ctx, nil)
	if err := s.events.Publish(ctx, events.NewMsgs(s.streamusers, userid, b)...); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to publish user roles event"), nil)
	}
	return nil
}

func (s *Service) DeleteRolesByRole(ctx context.Context, roleName string, userids []string) error {
	if len(userids) == 0 {
		return nil
	}

	msgs := make([]events.PublishMsg, 0, len(userids))
	userRole := rank.Rank{}.AddOne(roleName).ToSlice()
	for _, i := range userids {
		b, err := encodeUserEventRoles(RolesProps{
			Add:    false,
			Userid: i,
			Roles:  userRole,
		})
		if err != nil {
			return err
		}
		msgs = append(msgs, events.NewMsgs(s.streamusers, i, b)...)
	}

	if err := s.roles.DeleteByRole(ctx, roleName, userids); err != nil {
		return kerrors.WithMsg(err, "Failed to delete user roles")
	}

	// must make a best effort attempt to publish role update events
	ctx = klog.ExtendCtx(context.Background(), ctx, nil)
	if err := s.events.Publish(ctx, msgs...); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to publish user roles event"), nil)
	}
	return nil
}
