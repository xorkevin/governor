package user

import (
	"net/http"
	"time"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/util/rank"
)

func (s *service) UpdateUser(userid string, ruser reqUserPut) error {
	m, err := s.users.GetByID(userid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return governor.NewErrorUser("", 0, err)
		}
		return err
	}
	m.Username = ruser.Username
	m.FirstName = ruser.FirstName
	m.LastName = ruser.LastName
	if err = s.users.Update(m); err != nil {
		if governor.ErrorStatus(err) == http.StatusBadRequest {
			return governor.NewErrorUser("Username must be unique", 0, err)
		}
		return err
	}
	return nil
}

func (s *service) UpdateRank(userid string, updaterid string, editAddRank rank.Rank, editRemoveRank rank.Rank) error {
	updaterRank, err := s.roles.IntersectRoles(updaterid, combineModRoles(editAddRank, editRemoveRank))
	if err != nil {
		return err
	}

	if err := canUpdateRank(editAddRank, updaterRank, userid, updaterid, true); err != nil {
		return err
	}
	if err := canUpdateRank(editRemoveRank, updaterRank, userid, updaterid, false); err != nil {
		return err
	}

	m, err := s.users.GetByID(userid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return governor.NewErrorUser("", 0, err)
		}
		return err
	}

	editAddRank.Remove(editRemoveRank)

	currentRoles, err := s.roles.IntersectRoles(userid, editAddRank)
	if err != nil {
		return err
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

	if err := s.invitations.DeleteByRoles(m.Userid, editAddRank); err != nil {
		return err
	}
	if err := s.invitations.Insert(m.Userid, editAddRank, updaterid, now); err != nil {
		return err
	}
	if err := s.roles.DeleteRoles(m.Userid, editRemoveRank); err != nil {
		return err
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
				// updater cannot change one's own admin status nor change another's admin status if not an admin
				if editid == updaterid {
					return governor.NewErrorUser("Cannot modify own admin status", http.StatusForbidden, nil)
				}
				if !updaterIsAdmin {
					return governor.NewErrorUser("Must be admin to modify admin status", http.StatusForbidden, nil)
				}
			case rank.TagSystem:
				// no one can change the system status
				return governor.NewErrorUser("Forbidden rank edit", http.StatusForbidden, nil)
			case rank.TagUser:
				// only admins can change the user status
				if !updaterIsAdmin {
					return governor.NewErrorUser("Must be admin to modify user status", http.StatusForbidden, nil)
				}
			default:
				// other tags cannot be edited
				return governor.NewErrorUser("Invalid tag name", http.StatusForbidden, nil)
			}
		} else {
			switch prefix {
			case rank.TagModPrefix:
				// updater cannot change one's own mod status nor edit mod rank if not an admin and not a mod of that group
				if !updaterIsAdmin && editid == updaterid {
					return governor.NewErrorUser("Cannot modify own mod status", http.StatusForbidden, nil)
				}
				if !updaterIsAdmin && !updater.HasMod(tag) {
					return governor.NewErrorUser("Must be moderator of the group to modify mod status", http.StatusForbidden, nil)
				}
			case rank.TagBanPrefix:
				// cannot edit ban group rank if not an admin and not a moderator of that group
				if !updaterIsAdmin && !updater.HasMod(tag) {
					return governor.NewErrorUser("Must be moderator of the group to modify ban status", http.StatusForbidden, nil)
				}
			case rank.TagUserPrefix:
				if add {
					// cannot add user if not admin and not a moderator of that group
					if !updaterIsAdmin && !updater.HasMod(tag) {
						return governor.NewErrorUser("Must be a moderator of the group to add user status", http.StatusForbidden, nil)
					}
				} else {
					// cannot remove user if not admin and not a moderator of that group and not self
					if !updaterIsAdmin && !updater.HasMod(tag) && editid != updaterid {
						return governor.NewErrorUser("Cannot update other user status", http.StatusForbidden, nil)
					}
				}
			default:
				// other tags cannot be edited
				return governor.NewErrorUser("Invalid tag name", http.StatusBadRequest, nil)
			}
		}
	}
	return nil
}

func (s *service) AcceptRoleInvitation(userid, role string) error {
	m, err := s.users.GetByID(userid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return governor.NewErrorUser("", 0, err)
		}
		return err
	}

	now := time.Now().Round(0).Unix()
	after := now - s.invitationTime

	inv, err := s.invitations.GetByID(userid, role, after)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return governor.NewErrorUser("", 0, err)
		}
		return err
	}
	if inv.Role == rank.TagAdmin {
		s.logger.Info("add admin role", map[string]string{
			"userid":   m.Userid,
			"username": m.Username,
		})
	}
	if err := s.invitations.DeleteByID(userid, role); err != nil {
		return err
	}
	if err := s.roles.InsertRoles(m.Userid, rank.Rank{}.AddOne(inv.Role)); err != nil {
		return err
	}
	return nil
}
