package user

import (
	"net/http"
	"strings"
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

	if editAddRank.Has("admin") {
		s.logger.Info("add admin status", map[string]string{
			"userid":   userid,
			"username": m.Username,
		})
	}
	if editRemoveRank.Has("admin") {
		s.logger.Info("remove admin status", map[string]string{
			"userid":   userid,
			"username": m.Username,
		})
	}

	if err := s.roles.InsertRoles(m.Userid, editAddRank); err != nil {
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
		k := strings.SplitN(key, "_", 2)
		if len(k) == 1 {
			continue
		}
		roles.AddMod(k[1])
	}
	for key := range r2 {
		k := strings.SplitN(key, "_", 2)
		if len(k) == 1 {
			continue
		}
		roles.AddMod(k[1])
	}
	return roles
}

func canUpdateRank(edit, updater rank.Rank, editid, updaterid string, add bool) error {
	updaterIsAdmin := updater.Has(rank.TagAdmin)
	for key := range edit {
		k := strings.SplitN(key, "_", 2)
		if len(k) == 1 {
			switch k[0] {
			case rank.TagAdmin:
				// updater cannot change one's own admin status nor change another's admin status if he is not admin
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
			switch k[0] {
			case rank.TagModPrefix:
				// cannot edit mod group rank if not an admin and not a moderator of that group
				if !updaterIsAdmin && !updater.HasMod(k[1]) {
					return governor.NewErrorUser("Must be moderator of the group to modify mod status", http.StatusForbidden, nil)
				}
			case rank.TagBanPrefix:
				// cannot edit ban group rank if not an admin and not a moderator of that group
				if !updaterIsAdmin && !updater.HasMod(k[1]) {
					return governor.NewErrorUser("Must be moderator of the group to modify ban status", http.StatusForbidden, nil)
				}
			case rank.TagUserPrefix:
				if add {
					// cannot add user if not admin and not a moderator of that group
					if !updaterIsAdmin && !updater.HasMod(k[1]) {
						return governor.NewErrorUser("Must be a moderator of the group to add user status", http.StatusForbidden, nil)
					}
				} else {
					// cannot remove user if not admin and not a moderator of that group and not self
					if !updaterIsAdmin && !updater.HasMod(k[1]) && editid != updaterid {
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
