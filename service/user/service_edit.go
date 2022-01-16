package user

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/rank"
)

func (s *service) UpdateUser(userid string, ruser reqUserPut) error {
	m, err := s.users.GetByID(userid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "User not found",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to get user")
	}
	m.Username = ruser.Username
	m.FirstName = ruser.FirstName
	m.LastName = ruser.LastName
	b, err := json.Marshal(UpdateUserProps{
		Userid:    m.Userid,
		Username:  m.Username,
		FirstName: m.FirstName,
		LastName:  m.LastName,
	})
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to encode update user props to json")
	}
	if err = s.users.Update(m); err != nil {
		if errors.Is(err, db.ErrUnique{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusBadRequest,
				Message: "Username must be unique",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to update user")
	}
	if err := s.events.StreamPublish(s.opts.UpdateChannel, b); err != nil {
		s.logger.Error("Failed to publish update user event", map[string]string{
			"error":      err.Error(),
			"actiontype": "publishupdateuser",
		})
	}
	return nil
}

func (s *service) UpdateRank(userid string, updaterid string, editAddRank rank.Rank, editRemoveRank rank.Rank) error {
	updaterRank, err := s.roles.IntersectRoles(updaterid, combineModRoles(editAddRank, editRemoveRank))
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to get updater roles")
	}

	if err := canUpdateRank(editAddRank, updaterRank, userid, updaterid, true); err != nil {
		return err
	}
	if err := canUpdateRank(editRemoveRank, updaterRank, userid, updaterid, false); err != nil {
		return err
	}

	m, err := s.users.GetByID(userid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "User not found",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to get user")
	}

	editAddRank.Remove(editRemoveRank)

	currentRoles, err := s.roles.IntersectRoles(userid, editAddRank)
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to get user roles")
	}

	editAddRank.Remove(currentRoles)

	if editAddRank.Has(rank.TagAdmin) {
		s.logger.Info("invite add admin role", map[string]string{
			"userid":   m.Userid,
			"username": m.Username,
		})
	}
	if editRemoveRank.Has(rank.TagAdmin) {
		s.logger.Info("remove admin role", map[string]string{
			"userid":   m.Userid,
			"username": m.Username,
		})
	}

	now := time.Now().Round(0).Unix()

	if editAddRank.Has(rank.TagUser) {
		userRole := rank.Rank{}.AddOne(rank.TagUser)
		editAddRank.Remove(userRole)
		if err := s.roles.InsertRoles(m.Userid, userRole); err != nil {
			return governor.ErrWithMsg(err, "Failed to update user roles")
		}
	}

	if err := s.invitations.DeleteByRoles(m.Userid, editAddRank.Union(editRemoveRank)); err != nil {
		return governor.ErrWithMsg(err, "Failed to remove role invitations")
	}
	if err := s.invitations.Insert(m.Userid, editAddRank, updaterid, now); err != nil {
		return governor.ErrWithMsg(err, "Failed to add role invitations")
	}
	if err := s.roles.DeleteRoles(m.Userid, editRemoveRank); err != nil {
		return governor.ErrWithMsg(err, "Failed to remove user roles")
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
					return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
						Status:  http.StatusForbidden,
						Message: "Cannot modify own admin status",
					}))
				}
				// updater cannot change another's admin status if not an admin
				if !updaterIsAdmin {
					return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
						Status:  http.StatusForbidden,
						Message: "Must be admin to modify admin status",
					}))
				}
			case rank.TagSystem:
				// no one can change the system status
				return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
					Status:  http.StatusForbidden,
					Message: "Cannot modify system rank",
				}))
			case rank.TagUser:
				// only admins can change the user status
				if !updaterIsAdmin {
					return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
						Status:  http.StatusForbidden,
						Message: "Must be admin to modify user status",
					}))
				}
			default:
				// other tags cannot be edited
				return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
					Status:  http.StatusBadRequest,
					Message: "Invalid tag name",
				}))
			}
		} else {
			switch prefix {
			case rank.TagModPrefix:
				// updater cannot change one's own mod status if not an admin
				if !updaterIsAdmin && editid == updaterid {
					return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
						Status:  http.StatusForbidden,
						Message: "Cannot modify own mod status",
					}))
				}
				// updater cannot change another's mod rank if not an admin and not a mod of that group
				if !updaterIsAdmin && !updater.HasMod(tag) {
					return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
						Status:  http.StatusForbidden,
						Message: "Must be moderator of the group to modify mod status",
					}))
				}
			case rank.TagBanPrefix:
				// cannot edit ban group rank if not an admin and not a moderator of that group
				if !updaterIsAdmin && !updater.HasMod(tag) {
					return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
						Status:  http.StatusForbidden,
						Message: "Must be moderator of the group to modify ban status",
					}))
				}
			case rank.TagUserPrefix:
				if add {
					// cannot add user if not admin and not a moderator of that group
					if !updaterIsAdmin && !updater.HasMod(tag) {
						return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
							Status:  http.StatusForbidden,
							Message: "Must be a moderator of the group to add user status",
						}))
					}
				} else {
					// cannot remove user if not admin and not a moderator of that group and not self
					if !updaterIsAdmin && !updater.HasMod(tag) && editid != updaterid {
						return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
							Status:  http.StatusForbidden,
							Message: "Cannot update other user status",
						}))
					}
				}
			default:
				// other tags cannot be edited
				return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
					Status:  http.StatusBadRequest,
					Message: "Invalid tag name",
				}))
			}
		}
	}
	return nil
}

func (s *service) AcceptRoleInvitation(userid, role string) error {
	m, err := s.users.GetByID(userid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "User not found",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to get user")
	}

	now := time.Now().Round(0).Unix()
	after := now - s.invitationTime

	inv, err := s.invitations.GetByID(userid, role, after)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "Role invitation not found",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to get role invitation")
	}
	if inv.Role == rank.TagAdmin {
		s.logger.Info("Add admin role", map[string]string{
			"userid":   m.Userid,
			"username": m.Username,
		})
	}
	if err := s.invitations.DeleteByID(userid, role); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete role invitation")
	}
	if err := s.roles.InsertRoles(m.Userid, rank.Rank{}.AddOne(inv.Role)); err != nil {
		return governor.ErrWithMsg(err, "Failed to update roles")
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

func (s *service) GetUserRoleInvitations(userid string, amount, offset int) (*resUserRoleInvitations, error) {
	now := time.Now().Round(0).Unix()
	after := now - s.invitationTime

	m, err := s.invitations.GetByUser(userid, after, amount, offset)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get role invitations")
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

func (s *service) GetRoleInvitations(role string, amount, offset int) (*resUserRoleInvitations, error) {
	now := time.Now().Round(0).Unix()
	after := now - s.invitationTime

	m, err := s.invitations.GetByRole(role, after, amount, offset)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get role invitations")
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

func (s *service) DeleteRoleInvitation(userid, role string) error {
	if err := s.invitations.DeleteByID(userid, role); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete role invitation")
	}
	return nil
}
