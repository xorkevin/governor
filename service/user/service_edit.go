package user

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/util/rank"
	"net/http"
	"strings"
)

func (u *userService) UpdateUser(userid string, ruser reqUserPut) error {
	m, err := u.repo.GetByID(userid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return governor.NewErrorUser("", 0, err)
		}
		return err
	}
	m.Username = ruser.Username
	m.FirstName = ruser.FirstName
	m.LastName = ruser.LastName
	if err = u.repo.Update(m); err != nil {
		return err
	}
	return nil
}

func (u *userService) UpdateRank(userid string, updaterid string, updaterRank rank.Rank, editAddRank rank.Rank, editRemoveRank rank.Rank) error {
	if err := canUpdateRank(editAddRank, updaterRank, userid, updaterid, updaterRank.Has(rank.TagAdmin)); err != nil {
		return err
	}
	if err := canUpdateRank(editRemoveRank, updaterRank, userid, updaterid, updaterRank.Has(rank.TagAdmin)); err != nil {
		return err
	}

	m, err := u.repo.GetByID(userid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return governor.NewErrorUser("", 0, err)
		}
		return err
	}

	if editAddRank.Has("admin") {
		u.logger.Info("add admin status", map[string]string{
			"userid":   userid,
			"username": m.Username,
		})
	}
	if editRemoveRank.Has("admin") {
		u.logger.Info("remove admin status", map[string]string{
			"userid":   userid,
			"username": m.Username,
		})
	}

	diff := make(map[string]int)
	for k := range editAddRank {
		diff[k] = u.repo.RoleAddAction()
	}
	for k := range editRemoveRank {
		diff[k] = u.repo.RoleRemoveAction()
	}

	if err := u.KillAllSessions(userid); err != nil {
		return err
	}
	if err := u.repo.UpdateRoles(m, diff); err != nil {
		return err
	}
	return nil
}

func canUpdateRank(edit, updater rank.Rank, editid, updaterid string, isAdmin bool) error {
	for key := range edit {
		k := strings.SplitN(key, "_", 2)
		if len(k) == 1 {
			switch k[0] {
			case rank.TagAdmin:
				// updater cannot change one's own admin status nor change another's admin status if he is not admin
				if editid == updaterid {
					return governor.NewErrorUser("Cannot modify own admin status", http.StatusForbidden, nil)
				}
				if !isAdmin {
					return governor.NewErrorUser("Must be admin to modify admin status", http.StatusForbidden, nil)
				}
			case rank.TagSystem:
				// no one can change the system status
				return governor.NewErrorUser("Forbidden rank edit", http.StatusForbidden, nil)
			case rank.TagUser:
				// only admins can change the user status
				if !isAdmin {
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
				if !isAdmin && !updater.HasMod(k[1]) {
					return governor.NewErrorUser("Must be moderator of the group to modify mod status", http.StatusForbidden, nil)
				}
			case rank.TagBanPrefix:
				// cannot edit ban group rank if not an admin and not a moderator of that group
				if !isAdmin && !updater.HasMod(k[1]) {
					return governor.NewErrorUser("Must be moderator of the group to modify ban status", http.StatusForbidden, nil)
				}
			case rank.TagUserPrefix:
				// can only edit own id
				if !isAdmin && editid != updaterid {
					return governor.NewErrorUser("Cannot update other user status", http.StatusForbidden, nil)
				}
			default:
				// other tags cannot be edited
				return governor.NewErrorUser("Invalid tag name", http.StatusBadRequest, nil)
			}
		}
	}
	return nil
}
