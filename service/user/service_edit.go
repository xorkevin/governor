package user

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/rank"
	"xorkevin.dev/kerrors"
)

func (s *service) UpdateUser(ctx context.Context, userid string, ruser reqUserPut) error {
	m, err := s.users.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "User not found")
		}
		return kerrors.WithMsg(err, "Failed to get user")
	}
	updUsername := m.Username != ruser.Username
	m.Username = ruser.Username
	m.FirstName = ruser.FirstName
	m.LastName = ruser.LastName
	b, err := json.Marshal(UpdateUserProps{
		Userid:   m.Userid,
		Username: m.Username,
	})
	if err != nil {
		return kerrors.WithMsg(err, "Failed to encode update user props to json")
	}
	if err = s.users.UpdateProps(ctx, m); err != nil {
		if errors.Is(err, db.ErrUnique{}) {
			return governor.ErrWithRes(err, http.StatusBadRequest, "", "Username must be unique")
		}
		return kerrors.WithMsg(err, "Failed to update user")
	}
	if updUsername {
		// must make a best effort to publish username update
		if err := s.events.StreamPublish(context.Background(), s.opts.UpdateChannel, b); err != nil {
			s.logger.Error("Failed to publish update user props event", map[string]string{
				"error":      err.Error(),
				"actiontype": "user_publish_update_props",
			})
		}
	}
	return nil
}

func (s *service) UpdateRank(ctx context.Context, userid string, updaterid string, editAddRank rank.Rank, editRemoveRank rank.Rank) error {
	updaterRank, err := s.roles.IntersectRoles(ctx, updaterid, combineModRoles(editAddRank, editRemoveRank))
	if err != nil {
		return kerrors.WithMsg(err, "Failed to get updater roles")
	}

	if err := canUpdateRank(editAddRank, updaterRank, userid, updaterid, true); err != nil {
		return err
	}
	if err := canUpdateRank(editRemoveRank, updaterRank, userid, updaterid, false); err != nil {
		return err
	}

	m, err := s.users.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "User not found")
		}
		return kerrors.WithMsg(err, "Failed to get user")
	}

	editAddRank.Remove(editRemoveRank)

	currentRoles, err := s.roles.IntersectRoles(ctx, userid, editAddRank)
	if err != nil {
		return kerrors.WithMsg(err, "Failed to get user roles")
	}

	editAddRank.Remove(currentRoles)

	if editAddRank.Has(rank.TagAdmin) {
		s.logger.Info("Invite add admin role", map[string]string{
			"userid":     m.Userid,
			"username":   m.Username,
			"actiontype": "user_invite_role_admin",
		})
	}
	if editRemoveRank.Has(rank.TagAdmin) {
		s.logger.Info("Remove admin role", map[string]string{
			"userid":     m.Userid,
			"username":   m.Username,
			"actiontype": "user_remove_role_admin",
		})
	}

	now := time.Now().Round(0).Unix()

	if editAddRank.Has(rank.TagUser) {
		userRole := rank.Rank{}.AddOne(rank.TagUser)
		editAddRank.Remove(userRole)
		if err := s.roles.InsertRoles(ctx, m.Userid, userRole); err != nil {
			return kerrors.WithMsg(err, "Failed to update user roles")
		}
	}

	if err := s.invitations.DeleteByRoles(ctx, m.Userid, editAddRank.Union(editRemoveRank)); err != nil {
		return kerrors.WithMsg(err, "Failed to remove role invitations")
	}
	if err := s.invitations.Insert(ctx, m.Userid, editAddRank, updaterid, now); err != nil {
		return kerrors.WithMsg(err, "Failed to add role invitations")
	}
	if err := s.roles.DeleteRoles(ctx, m.Userid, editRemoveRank); err != nil {
		return kerrors.WithMsg(err, "Failed to remove user roles")
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
			case rank.TagSystem:
				// no one can change the system status
				return governor.ErrWithRes(nil, http.StatusForbidden, "", "Cannot modify system rank")
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

func (s *service) AcceptRoleInvitation(ctx context.Context, userid, role string) error {
	m, err := s.users.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "User not found")
		}
		return kerrors.WithMsg(err, "Failed to get user")
	}

	now := time.Now().Round(0).Unix()
	after := now - s.invitationTime

	inv, err := s.invitations.GetByID(ctx, userid, role, after)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "Role invitation not found")
		}
		return kerrors.WithMsg(err, "Failed to get role invitation")
	}
	if inv.Role == rank.TagAdmin {
		s.logger.Info("Add admin role", map[string]string{
			"userid":     m.Userid,
			"username":   m.Username,
			"actiontype": "user_add_role_admin",
		})
	}
	if err := s.invitations.DeleteByID(ctx, userid, role); err != nil {
		return kerrors.WithMsg(err, "Failed to delete role invitation")
	}
	if err := s.roles.InsertRoles(ctx, m.Userid, rank.Rank{}.AddOne(inv.Role)); err != nil {
		return kerrors.WithMsg(err, "Failed to update roles")
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

func (s *service) GetUserRoleInvitations(ctx context.Context, userid string, amount, offset int) (*resUserRoleInvitations, error) {
	now := time.Now().Round(0).Unix()
	after := now - s.invitationTime

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

func (s *service) GetRoleInvitations(ctx context.Context, role string, amount, offset int) (*resUserRoleInvitations, error) {
	now := time.Now().Round(0).Unix()
	after := now - s.invitationTime

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

func (s *service) DeleteRoleInvitation(ctx context.Context, userid, role string) error {
	if err := s.invitations.DeleteByID(ctx, userid, role); err != nil {
		return kerrors.WithMsg(err, "Failed to delete role invitation")
	}
	return nil
}

func (s *service) DeleteRoleInvitations(ctx context.Context, role string) error {
	if err := s.invitations.DeleteRole(ctx, role); err != nil {
		return kerrors.WithMsg(err, "Failed to delete role invitations")
	}
	return nil
}
