package user

import (
	"context"
	"errors"
	"net/http"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/util/rank"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

func (s *Service) updateUser(ctx context.Context, userid string, ruser reqUserPut) error {
	m, err := s.users.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "User not found")
		}
		return kerrors.WithMsg(err, "Failed to get user")
	}
	updUsername := m.Username != ruser.Username
	m.Username = ruser.Username
	m.FirstName = ruser.FirstName
	m.LastName = ruser.LastName
	var b []byte
	if updUsername {
		var err error
		b, err = encodeUserEventUpdate(UpdateUserProps{
			Userid:   m.Userid,
			Username: m.Username,
		})
		if err != nil {
			return err
		}
	}

	if err = s.users.UpdateProps(ctx, m); err != nil {
		if errors.Is(err, db.ErrorUnique) {
			return governor.ErrWithRes(err, http.StatusBadRequest, "", "Username must be unique")
		}
		return kerrors.WithMsg(err, "Failed to update user")
	}

	if updUsername {
		// must make a best effort to publish username update
		ctx = klog.ExtendCtx(context.Background(), ctx, nil)
		if err := s.events.Publish(ctx, events.NewMsgs(s.streamusers, userid, b)...); err != nil {
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to publish update user props event"), nil)
		}
	}
	return nil
}

func (s *Service) updateRoles(ctx context.Context, userid string, updaterid string, editAddRoles rank.Rank, editRemoveRoles rank.Rank) error {
	updaterRank, err := s.roles.IntersectRoles(ctx, updaterid, combineModRoles(editAddRoles, editRemoveRoles))
	if err != nil {
		return kerrors.WithMsg(err, "Failed to get updater roles")
	}

	if err := canUpdateRank(editAddRoles, updaterRank, userid, updaterid, true); err != nil {
		return err
	}
	if err := canUpdateRank(editRemoveRoles, updaterRank, userid, updaterid, false); err != nil {
		return err
	}

	m, err := s.users.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "User not found")
		}
		return kerrors.WithMsg(err, "Failed to get user")
	}

	editAddRoles.Remove(editRemoveRoles)

	currentRoles, err := s.roles.IntersectRoles(ctx, userid, editAddRoles)
	if err != nil {
		return kerrors.WithMsg(err, "Failed to get user roles")
	}

	editAddRoles.Remove(currentRoles)

	now := time.Now().Round(0).Unix()

	if editAddRoles.Has(rank.TagUser) {
		userRole := rank.BaseUser()
		editAddRoles.Remove(userRole)

		b, err := encodeUserEventRoles(RolesProps{
			Add:    true,
			Userid: userid,
			Roles:  userRole.ToSlice(),
		})
		if err != nil {
			return err
		}

		if err := s.roles.InsertRoles(ctx, m.Userid, userRole); err != nil {
			return kerrors.WithMsg(err, "Failed to update user roles")
		}

		// must make a best effort attempt to publish role update event
		ectx := klog.ExtendCtx(context.Background(), ctx, nil)
		if err := s.events.Publish(ectx, events.NewMsgs(s.streamusers, userid, b)...); err != nil {
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to publish user roles event"), nil)
		}
	}

	if err := s.invitations.DeleteByRoles(ctx, m.Userid, editAddRoles.Union(editRemoveRoles)); err != nil {
		return kerrors.WithMsg(err, "Failed to remove role invitations")
	}
	if err := s.invitations.Insert(ctx, m.Userid, editAddRoles, updaterid, now); err != nil {
		return kerrors.WithMsg(err, "Failed to add role invitations")
	}
	if editAddRoles.Has(rank.TagAdmin) {
		s.log.Info(ctx, "Invite add admin role", klog.Fields{
			"user.userid":   m.Userid,
			"user.username": m.Username,
		})
	}

	if editRemoveRoles.Len() > 0 {
		b, err := encodeUserEventRoles(RolesProps{
			Add:    false,
			Userid: userid,
			Roles:  editRemoveRoles.ToSlice(),
		})
		if err != nil {
			return err
		}

		if err := s.roles.DeleteRoles(ctx, m.Userid, editRemoveRoles); err != nil {
			return kerrors.WithMsg(err, "Failed to remove user roles")
		}

		if editRemoveRoles.Has(rank.TagAdmin) {
			s.log.Info(ctx, "Remove admin role", klog.Fields{
				"user.userid":   m.Userid,
				"user.username": m.Username,
			})
		}

		// must make a best effort attempt to publish role update events
		ctx = klog.ExtendCtx(context.Background(), ctx, nil)
		if err := s.events.Publish(ctx, events.NewMsgs(s.streamusers, userid, b)...); err != nil {
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to publish user roles event"), nil)
		}
	}

	return nil
}

func combineModRoles(r1, r2 rank.Rank) rank.Rank {
	roles := rank.Rank{
		rank.TagAdmin: struct{}{},
	}
	for key := range r1 {
		_, tag, err := rank.SplitTag(key)
		if err != nil {
			continue
		}
		roles.AddMod(tag)
	}
	for key := range r2 {
		_, tag, err := rank.SplitTag(key)
		if err != nil {
			continue
		}
		roles.AddMod(tag)
	}
	return roles
}

func canUpdateRank(edit, updater rank.Rank, editid, updaterid string, add bool) error {
	updaterIsAdmin := updater.Has(rank.TagAdmin)
	for key := range edit {
		prefix, tag, err := rank.SplitTag(key)
		if err != nil {
			switch key {
			case rank.TagAdmin:
				// updater cannot change one's own admin status
				if editid == updaterid {
					return governor.ErrWithRes(nil, http.StatusForbidden, "", "Cannot modify own admin status")
				}
				// updater cannot change another's admin status if not an admin
				if !updaterIsAdmin {
					return governor.ErrWithRes(nil, http.StatusForbidden, "", "Must be admin to modify admin status")
				}
			case rank.TagUser:
				// only admins can change the user status
				if !updaterIsAdmin {
					return governor.ErrWithRes(nil, http.StatusForbidden, "", "Must be admin to modify user status")
				}
			default:
				// other tags cannot be edited
				return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Invalid tag name")
			}
		} else {
			switch prefix {
			case rank.TagModPrefix:
				// updater cannot change one's own mod status if not an admin
				if !updaterIsAdmin && editid == updaterid {
					return governor.ErrWithRes(nil, http.StatusForbidden, "", "Cannot modify own mod status")
				}
				// updater cannot change another's mod rank if not an admin and not a mod of that group
				if !updaterIsAdmin && !updater.HasMod(tag) {
					return governor.ErrWithRes(nil, http.StatusForbidden, "", "Must be moderator of the group to modify mod status")
				}
			case rank.TagBanPrefix:
				// cannot edit ban group rank if not an admin and not a moderator of that group
				if !updaterIsAdmin && !updater.HasMod(tag) {
					return governor.ErrWithRes(nil, http.StatusForbidden, "", "Must be moderator of the group to modify ban status")
				}
			case rank.TagUserPrefix:
				if add {
					// cannot add user if not admin and not a moderator of that group
					if !updaterIsAdmin && !updater.HasMod(tag) {
						return governor.ErrWithRes(nil, http.StatusForbidden, "", "Must be a moderator of the group to add user status")
					}
				} else {
					// cannot remove user if not admin and not a moderator of that group and not self
					if !updaterIsAdmin && !updater.HasMod(tag) && editid != updaterid {
						return governor.ErrWithRes(nil, http.StatusForbidden, "", "Cannot update other user status")
					}
				}
			default:
				// other tags cannot be edited
				return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Invalid tag name")
			}
		}
	}
	return nil
}

func (s *Service) acceptRoleInvitation(ctx context.Context, userid, role string) error {
	m, err := s.users.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "User not found")
		}
		return kerrors.WithMsg(err, "Failed to get user")
	}

	after := time.Now().Round(0).Add(-s.authsettings.invitationDuration).Unix()

	inv, err := s.invitations.GetByID(ctx, userid, role, after)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "Role invitation not found")
		}
		return kerrors.WithMsg(err, "Failed to get role invitation")
	}

	userRole := rank.Rank{}.AddOne(inv.Role)
	b, err := encodeUserEventRoles(RolesProps{
		Add:    true,
		Userid: userid,
		Roles:  userRole.ToSlice(),
	})
	if err != nil {
		return err
	}

	if err := s.invitations.DeleteByID(ctx, userid, role); err != nil {
		return kerrors.WithMsg(err, "Failed to delete role invitation")
	}
	if err := s.roles.InsertRoles(ctx, m.Userid, userRole); err != nil {
		return kerrors.WithMsg(err, "Failed to update roles")
	}

	if inv.Role == rank.TagAdmin {
		s.log.Info(ctx, "Add admin role", klog.Fields{
			"user.userid":   m.Userid,
			"user.username": m.Username,
		})
	}
	// must make a best effort attempt to publish role update events
	ctx = klog.ExtendCtx(context.Background(), ctx, nil)
	if err := s.events.Publish(ctx, events.NewMsgs(s.streamusers, userid, b)...); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to publish user roles event"), nil)
	}
	return nil
}

type (
	resUserRoleInvitation struct {
		Userid       string `json:"userid"`
		Role         string `json:"role"`
		InvitedBy    string `json:"invited_by"`
		CreationTime int64  `json:"creation_time"`
	}

	resUserRoleInvitations struct {
		Invitations []resUserRoleInvitation `json:"invitations"`
	}
)

func (s *Service) getUserRoleInvitations(ctx context.Context, userid string, amount, offset int) (*resUserRoleInvitations, error) {
	after := time.Now().Round(0).Add(-s.authsettings.invitationDuration).Unix()

	m, err := s.invitations.GetByUser(ctx, userid, after, amount, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get role invitations")
	}
	res := make([]resUserRoleInvitation, 0, len(m))
	for _, i := range m {
		res = append(res, resUserRoleInvitation{
			Userid:       i.Userid,
			Role:         i.Role,
			InvitedBy:    i.InvitedBy,
			CreationTime: i.CreationTime,
		})
	}
	return &resUserRoleInvitations{
		Invitations: res,
	}, nil
}

func (s *Service) getRoleInvitations(ctx context.Context, role string, amount, offset int) (*resUserRoleInvitations, error) {
	after := time.Now().Round(0).Add(-s.authsettings.invitationDuration).Unix()

	m, err := s.invitations.GetByRole(ctx, role, after, amount, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get role invitations")
	}
	res := make([]resUserRoleInvitation, 0, len(m))
	for _, i := range m {
		res = append(res, resUserRoleInvitation{
			Userid:       i.Userid,
			Role:         i.Role,
			InvitedBy:    i.InvitedBy,
			CreationTime: i.CreationTime,
		})
	}
	return &resUserRoleInvitations{
		Invitations: res,
	}, nil
}

func (s *Service) deleteRoleInvitation(ctx context.Context, userid, role string) error {
	if err := s.invitations.DeleteByID(ctx, userid, role); err != nil {
		return kerrors.WithMsg(err, "Failed to delete role invitation")
	}
	return nil
}

func (s *Service) DeleteRoleInvitations(ctx context.Context, role string) error {
	if err := s.invitations.DeleteRole(ctx, role); err != nil {
		return kerrors.WithMsg(err, "Failed to delete role invitations")
	}
	return nil
}
