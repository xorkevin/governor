package user

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/util/rank"
	"net/http"
	"strings"
)

func (u *userService) UpdateUser(userid string, ruser reqUserPut) *governor.Error {
	m, err := u.repo.GetByIDB64(userid)
	if err != nil {
		if err.Code() == 2 {
			err.SetErrorUser()
		}
		err.AddTrace(moduleIDUser)
		return err
	}
	m.Username = ruser.Username
	m.FirstName = ruser.FirstName
	m.LastName = ruser.LastName
	if err = u.repo.Update(m); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	return nil
}

func (u *userService) UpdateRank(userid string, updaterid string, updaterRank rank.Rank, editAddRank rank.Rank, editRemoveRank rank.Rank) *governor.Error {
	if err := canUpdateRank(editAddRank, updaterRank, userid, updaterid, updaterRank.Has(rank.TagAdmin)); err != nil {
		return err
	}
	if err := canUpdateRank(editRemoveRank, updaterRank, userid, updaterid, updaterRank.Has(rank.TagAdmin)); err != nil {
		return err
	}

	m, err := u.repo.GetByIDB64(userid)
	if err != nil {
		if err.Code() == 2 {
			err.SetErrorUser()
		}
		err.AddTrace(moduleIDUser)
		return err
	}

	if editAddRank.Has("admin") {
		u.logger.Info("admin status added", moduleIDUser, "add admin status", 0, map[string]string{
			"userid":   userid,
			"username": m.Username,
		})
	}
	if editRemoveRank.Has("admin") {
		u.logger.Info("admin status removed", moduleIDUser, "remove admin status", 0, map[string]string{
			"userid":   userid,
			"username": m.Username,
		})
	}

	diff := make(map[string]int)
	for k, v := range editAddRank {
		if v {
			diff[k] = u.repo.RoleAddAction()
		}
	}
	for k, v := range editRemoveRank {
		if v {
			diff[k] = u.repo.RoleRemoveAction()
		}
	}

	if err := u.KillAllSessions(userid); err != nil {
		return err
	}
	if err := u.repo.UpdateRoles(m, diff); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	return nil
}

func canUpdateRank(edit, updater rank.Rank, editid, updaterid string, isAdmin bool) *governor.Error {
	for key := range edit {
		k := strings.SplitN(key, "_", 2)
		if len(k) == 1 {
			switch k[0] {
			case rank.TagAdmin:
				// updater cannot change one's own admin status nor change another's admin status if he is not admin
				if editid == updaterid || !isAdmin {
					return governor.NewErrorUser(moduleIDUser, "forbidden rank edit", 0, http.StatusForbidden)
				}
			case rank.TagSystem:
				// no one can change the system status
				return governor.NewErrorUser(moduleIDUser, "forbidden rank edit", 0, http.StatusForbidden)
			case rank.TagUser:
				// only admins can change the user status
				if !isAdmin {
					return governor.NewErrorUser(moduleIDUser, "forbidden rank edit", 0, http.StatusForbidden)
				}
			default:
				// other tags cannot be edited
				return governor.NewErrorUser(moduleIDUser, "forbidden rank edit", 0, http.StatusBadRequest)
			}
		} else {
			switch k[0] {
			case rank.TagModPrefix:
				// cannot edit mod group rank if not an admin and not a moderator of that group
				if !isAdmin && !updater.HasMod(k[1]) {
					return governor.NewErrorUser(moduleIDUser, "forbidden rank edit", 0, http.StatusForbidden)
				}
			case rank.TagBanPrefix:
				// cannot edit ban group rank if not an admin and not a moderator of that group
				if !isAdmin && !updater.HasMod(k[1]) {
					return governor.NewErrorUser(moduleIDUser, "forbidden rank edit", 0, http.StatusForbidden)
				}
			case rank.TagUserPrefix:
				// can only edit own id
				if !isAdmin && editid != updaterid {
					return governor.NewErrorUser(moduleIDUser, "forbidden rank edit", 0, http.StatusForbidden)
				}
			default:
				// other tags cannot be edited
				return governor.NewErrorUser(moduleIDUser, "forbidden rank edit", 0, http.StatusBadRequest)
			}
		}
	}
	return nil
}
