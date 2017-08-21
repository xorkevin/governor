package user

import (
	"bytes"
	"encoding/gob"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/model"
	"github.com/hackform/governor/service/user/session"
	"github.com/hackform/governor/service/user/token"
	"github.com/hackform/governor/util/rank"
	"github.com/hackform/governor/util/uid"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"net/http"
	"strings"
	"time"
)

type (
	emailNewUser struct {
		FirstName string
		Key       string
	}
)

const (
	newUserTemplate = "newuser"
	newUserSubject  = "newuser_subject"
)

func (u *userService) confirmUser(c echo.Context, l *logrus.Logger) error {
	db := u.db.DB()
	ch := u.cache.Cache()
	mailer := u.mailer

	ruser := reqUserPost{}
	if err := c.Bind(&ruser); err != nil {
		return governor.NewErrorUser(moduleIDUser, err.Error(), 0, http.StatusBadRequest)
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	m2, err := usermodel.GetByUsername(db, ruser.Username)
	if err != nil && err.Code() != 2 {
		err.AddTrace(moduleIDUser)
		return err
	}
	if m2 != nil && m2.Username == ruser.Username {
		return governor.NewErrorUser(moduleIDUser, "username is already taken", 0, http.StatusBadRequest)
	}

	m, err := usermodel.NewBaseUser(ruser.Username, ruser.Password, ruser.Email, ruser.FirstName, ruser.LastName)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	b := bytes.Buffer{}
	if err := gob.NewEncoder(&b).Encode(m); err != nil {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}

	key, err := uid.NewU(0, 16)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	sessionKey := key.Base64()

	if err := ch.Set(sessionKey, b.String(), time.Duration(u.confirmTime*b1)).Err(); err != nil {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}

	emdata := emailNewUser{
		FirstName: m.FirstName,
		Key:       sessionKey,
	}

	em, err := u.tpl.ExecuteHTML(newUserTemplate, emdata)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	subj, err := u.tpl.ExecuteHTML(newUserSubject, emdata)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	if err := mailer.Send(m.Email, subj, em); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	userid, _ := m.IDBase64()

	return c.JSON(http.StatusCreated, resUserUpdate{
		Userid:   userid,
		Username: m.Username,
	})
}

func (u *userService) postUser(c echo.Context, l *logrus.Logger) error {
	db := u.db.DB()
	ch := u.cache.Cache()

	ruser := reqUserPostConfirm{}
	if err := c.Bind(&ruser); err != nil {
		return governor.NewErrorUser(moduleIDUser, err.Error(), 0, http.StatusBadRequest)
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	gobUser, err := ch.Get(ruser.Key).Result()
	if err != nil {
		return governor.NewErrorUser(moduleIDUser, err.Error(), 0, http.StatusBadRequest)
	}

	m := usermodel.Model{}
	b := bytes.NewBufferString(gobUser)
	if err := gob.NewDecoder(b).Decode(&m); err != nil {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}

	if err := m.Insert(db); err != nil {
		if err.Code() == 3 {
			err.SetErrorUser()
		}
		err.AddTrace(moduleIDUser)
		return err
	}

	if err := ch.Del(ruser.Key).Err(); err != nil {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}

	t, _ := time.Now().MarshalText()
	userid, _ := m.IDBase64()
	l.WithFields(logrus.Fields{
		"time":     string(t),
		"origin":   moduleIDUser,
		"userid":   userid,
		"username": m.Username,
	}).Info("user created")

	return c.JSON(http.StatusCreated, resUserUpdate{
		Userid:   userid,
		Username: m.Username,
	})
}

func (u *userService) putUser(c echo.Context, l *logrus.Logger) error {
	db := u.db.DB()

	reqid := reqUserGetID{
		Userid: c.Param("id"),
	}
	if err := reqid.valid(); err != nil {
		return err
	}
	ruser := reqUserPut{}
	if err := c.Bind(&ruser); err != nil {
		return governor.NewErrorUser(moduleIDUser, err.Error(), 0, http.StatusBadRequest)
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	m, err := usermodel.GetByIDB64(db, reqid.Userid)
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
	if err = m.Update(db); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (u *userService) putEmail(c echo.Context, l *logrus.Logger) error {
	db := u.db.DB()

	reqid := reqUserGetID{
		Userid: c.Param("id"),
	}
	if err := reqid.valid(); err != nil {
		return err
	}
	ruser := reqUserPutEmail{}
	if err := c.Bind(&ruser); err != nil {
		return governor.NewErrorUser(moduleIDUser, err.Error(), 0, http.StatusBadRequest)
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	m, err := usermodel.GetByIDB64(db, reqid.Userid)
	if err != nil {
		if err.Code() == 2 {
			err.SetErrorUser()
		}
		err.AddTrace(moduleIDUser)
		return err
	}
	if !m.ValidatePass(ruser.Password) {
		return governor.NewErrorUser(moduleIDUser, "incorrect password", 0, http.StatusForbidden)
	}
	m.Email = ruser.Email
	if err = m.Update(db); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (u *userService) putPassword(c echo.Context, l *logrus.Logger) error {
	db := u.db.DB()

	reqid := reqUserGetID{
		Userid: c.Param("id"),
	}
	if err := reqid.valid(); err != nil {
		return err
	}
	ruser := reqUserPutPassword{}
	if err := c.Bind(&ruser); err != nil {
		return governor.NewErrorUser(moduleIDUser, err.Error(), 0, http.StatusBadRequest)
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	m, err := usermodel.GetByIDB64(db, reqid.Userid)
	if err != nil {
		if err.Code() == 2 {
			err.SetErrorUser()
		}
		err.AddTrace(moduleIDUser)
		return err
	}
	if !m.ValidatePass(ruser.OldPassword) {
		return governor.NewErrorUser(moduleIDUser, "incorrect password", 0, http.StatusForbidden)
	}
	if err = m.RehashPass(ruser.NewPassword); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	if err = m.Update(db); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (u *userService) forgotPassword(c echo.Context, l *logrus.Logger) error {
	db := u.db.DB()
	ch := u.cache.Cache()
	mailer := u.mailer

	ruser := reqForgotPassword{}
	if err := c.Bind(&ruser); err != nil {
		return governor.NewErrorUser(moduleIDUser, err.Error(), 0, http.StatusBadRequest)
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	m, err := usermodel.GetByUsername(db, ruser.Username)
	if err != nil {
		if err.Code() == 2 {
			err.SetErrorUser()
		}
		err.AddTrace(moduleIDUser)
		return err
	}

	key, err := uid.NewU(0, 16)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	sessionKey := key.Base64()

	userid, err := m.IDBase64()
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	if err := ch.Set(sessionKey, userid, time.Duration(u.passwordResetTime*b1)).Err(); err != nil {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}

	if err := mailer.Send(m.Email, "Password reset", "key: "+sessionKey); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

func (u *userService) forgotPasswordReset(c echo.Context, l *logrus.Logger) error {
	db := u.db.DB()
	ch := u.cache.Cache()

	ruser := reqForgotPasswordReset{}
	if err := c.Bind(&ruser); err != nil {
		return governor.NewErrorUser(moduleIDUser, err.Error(), 0, http.StatusBadRequest)
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	userid := ""
	if result, err := ch.Get(ruser.Key).Result(); err == nil {
		userid = result
	} else {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}

	m, err := usermodel.GetByIDB64(db, userid)
	if err != nil {
		if err.Code() == 2 {
			err.SetErrorUser()
		}
		err.AddTrace(moduleIDUser)
		return err
	}

	if err := m.RehashPass(ruser.NewPassword); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	if err := m.Update(db); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

func (u *userService) killSessions(c echo.Context, l *logrus.Logger) error {
	ch := u.cache.Cache()

	reqid := reqUserGetID{
		Userid: c.Param("id"),
	}
	if err := reqid.valid(); err != nil {
		return err
	}
	ruser := reqUserRmSessions{}
	if err := c.Bind(&ruser); err != nil {
		return governor.NewErrorUser(moduleIDUser, err.Error(), 0, http.StatusBadRequest)
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	s := session.Session{
		Userid: reqid.Userid,
	}

	if err := ch.Del(ruser.SessionIDs...).Err(); err != nil {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}

	if err := ch.HDel(s.UserKey(), ruser.SessionIDs...).Err(); err != nil {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}

	return c.NoContent(http.StatusNoContent)
}

func (u *userService) patchRank(c echo.Context, l *logrus.Logger) error {
	db := u.db.DB()

	reqid := reqUserGetID{
		Userid: c.Param("id"),
	}
	if err := reqid.valid(); err != nil {
		return err
	}
	ruser := reqUserPutRank{}
	if err := c.Bind(&ruser); err != nil {
		return governor.NewErrorUser(moduleIDUser, err.Error(), 0, http.StatusBadRequest)
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	updaterClaims, ok := c.Get("user").(*token.Claims)
	if !ok {
		return governor.NewErrorUser(moduleIDUser, "invalid auth claims", 0, http.StatusUnauthorized)
	}
	updaterRank, _ := rank.FromStringUser(updaterClaims.AuthTags)
	editAddRank, _ := rank.FromStringUser(ruser.Add)
	editRemoveRank, _ := rank.FromStringUser(ruser.Remove)

	if err := canUpdateRank(editAddRank, updaterRank, reqid.Userid, updaterClaims.Userid, updaterRank.Has(rank.TagAdmin)); err != nil {
		return err
	}
	if err := canUpdateRank(editRemoveRank, updaterRank, reqid.Userid, updaterClaims.Userid, updaterRank.Has(rank.TagAdmin)); err != nil {
		return err
	}

	m, err := usermodel.GetByIDB64(db, reqid.Userid)
	if err != nil {
		if err.Code() == 2 {
			err.SetErrorUser()
		}
		err.AddTrace(moduleIDUser)
		return err
	}

	if editAddRank.Has("admin") {
		t, _ := time.Now().MarshalText()
		userid, _ := m.IDBase64()
		l.WithFields(logrus.Fields{
			"time":     string(t),
			"origin":   moduleIDUser,
			"userid":   userid,
			"username": m.Username,
		}).Info("admin status added")
	}
	if editRemoveRank.Has("admin") {
		t, _ := time.Now().MarshalText()
		userid, _ := m.IDBase64()
		l.WithFields(logrus.Fields{
			"time":     string(t),
			"origin":   moduleIDUser,
			"userid":   userid,
			"username": m.Username,
		}).Info("admin status removed")
	}

	finalRank, _ := rank.FromStringUser(m.Auth.Tags)
	finalRank.Add(editAddRank)
	finalRank.Remove(editRemoveRank)

	m.Auth.Tags = finalRank.Stringify()
	if err = m.Update(db); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	return c.NoContent(http.StatusNoContent)
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
			// cannot edit group rank if not an admin or a moderator of that group
			if !isAdmin && updater.HasMod(k[1]) {
				return governor.NewErrorUser(moduleIDUser, "forbidden rank edit", 0, http.StatusForbidden)
			}
		}
	}
	return nil
}

func (u *userService) deleteUser(c echo.Context, l *logrus.Logger) error {
	db := u.db.DB()
	ch := u.cache.Cache()

	reqid := &reqUserGetID{
		Userid: c.Param("id"),
	}
	if err := reqid.valid(); err != nil {
		return err
	}
	ruser := reqUserDelete{}
	if err := c.Bind(&ruser); err != nil {
		return governor.NewErrorUser(moduleIDUser, err.Error(), 0, http.StatusBadRequest)
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	if reqid.Userid != ruser.Userid {
		return governor.NewErrorUser(moduleIDUser, "information does not match", 0, http.StatusBadRequest)
	}

	m, err := usermodel.GetByIDB64(db, reqid.Userid)
	if err != nil {
		if err.Code() == 2 {
			err.SetErrorUser()
		}
		err.AddTrace(moduleIDUser)
		return err
	}

	if m.Username != ruser.Username {
		return governor.NewErrorUser(moduleIDUser, "information does not match", 0, http.StatusBadRequest)
	}

	if !m.ValidatePass(ruser.Password) {
		return governor.NewErrorUser(moduleIDUser, "incorrect password", 0, http.StatusForbidden)
	}

	s := session.Session{
		Userid: reqid.Userid,
	}

	var sessionIDs []string
	if smap, err := ch.HGetAll(s.UserKey()).Result(); err == nil {
		sessionIDs = make([]string, 0, len(smap))
		for k := range smap {
			sessionIDs = append(sessionIDs, k)
		}
	} else {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}

	if err := ch.Del(sessionIDs...).Err(); err != nil {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}

	if err := ch.HDel(s.UserKey(), sessionIDs...).Err(); err != nil {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}

	if err := m.Delete(db); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	return c.NoContent(http.StatusNoContent)
}
