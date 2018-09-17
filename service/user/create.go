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
	emailForgotPass struct {
		FirstName string
		Username  string
		Key       string
	}

	emailPassReset struct {
		FirstName string
		Username  string
	}

	emailPassChange struct {
		FirstName string
		Username  string
		Key       string
	}

	emailEmailChange struct {
		FirstName string
		Username  string
		Key       string
	}

	emailEmailChangeNotify struct {
		FirstName string
		Username  string
	}
)

const (
	forgotPassTemplate        = "forgotpass"
	forgotPassSubject         = "forgotpass_subject"
	passResetTemplate         = "passreset"
	passResetSubject          = "passreset_subject"
	passChangeTemplate        = "passchange"
	passChangeSubject         = "passchange_subject"
	emailChangeTemplate       = "emailchange"
	emailChangeSubject        = "emailchange_subject"
	emailChangeNotifyTemplate = "emailchangenotify"
	emailChangeNotifySubject  = "emailchangenotify_subject"
)

const (
	emailChangeEscapeSequence = "%email%"
)

func (u *userRouter) putUser(c echo.Context, l *logrus.Logger) error {
	db := u.service.db.DB()

	userid := c.Get("userid").(string)

	ruser := reqUserPut{}
	if err := c.Bind(&ruser); err != nil {
		return governor.NewErrorUser(moduleIDUser, err.Error(), 0, http.StatusBadRequest)
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	m, err := usermodel.GetByIDB64(db, userid)
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

func (u *userRouter) putEmail(c echo.Context, l *logrus.Logger) error {
	db := u.service.db.DB()
	ch := u.service.cache.Cache()
	mailer := u.service.mailer

	userid := c.Get("userid").(string)

	ruser := reqUserPutEmail{}
	if err := c.Bind(&ruser); err != nil {
		return governor.NewErrorUser(moduleIDUser, err.Error(), 0, http.StatusBadRequest)
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	m, err := usermodel.GetByIDB64(db, userid)
	if err != nil {
		if err.Code() == 2 {
			err.SetErrorUser()
		}
		err.AddTrace(moduleIDUser)
		return err
	}
	if m.Email == ruser.Email {
		return governor.NewErrorUser(moduleIDUser, "emails cannot be the same", 0, http.StatusBadRequest)
	}
	if !m.ValidatePass(ruser.Password) {
		return governor.NewErrorUser(moduleIDUser, "incorrect password", 0, http.StatusForbidden)
	}

	key, err := uid.NewU(0, 16)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	sessionKey := key.Base64()

	if err := ch.Set(sessionKey, userid+emailChangeEscapeSequence+ruser.Email, time.Duration(u.service.passwordResetTime*b1)).Err(); err != nil {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}

	emdata := emailEmailChange{
		FirstName: m.FirstName,
		Username:  m.Username,
		Key:       sessionKey,
	}

	em, err := u.service.tpl.ExecuteHTML(emailChangeTemplate, emdata)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	subj, err := u.service.tpl.ExecuteHTML(emailChangeSubject, emdata)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	emdatanotify := emailEmailChangeNotify{
		FirstName: m.FirstName,
		Username:  m.Username,
	}

	emnotify, err := u.service.tpl.ExecuteHTML(emailChangeNotifyTemplate, emdatanotify)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	subjnotify, err := u.service.tpl.ExecuteHTML(emailChangeNotifySubject, emdatanotify)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	if err := mailer.Send(m.Email, subjnotify, emnotify); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	if err := mailer.Send(ruser.Email, subj, em); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

func (u *userRouter) putEmailVerify(c echo.Context, l *logrus.Logger) error {
	db := u.service.db.DB()
	ch := u.service.cache.Cache()

	ruser := reqUserPutEmailVerify{}
	if err := c.Bind(&ruser); err != nil {
		return governor.NewErrorUser(moduleIDUser, err.Error(), 0, http.StatusBadRequest)
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	var userid, email string

	if result, err := ch.Get(ruser.Key).Result(); err == nil {
		k := strings.SplitN(result, emailChangeEscapeSequence, 2)
		if len(k) != 2 {
			return governor.NewError(moduleIDUser, "incorrect sessionKey value in cache during email verification", 0, http.StatusInternalServerError)
		}
		userid = k[0]
		email = k[1]
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

	if !m.ValidatePass(ruser.Password) {
		return governor.NewErrorUser(moduleIDUser, "incorrect password", 0, http.StatusForbidden)
	}

	if err := ch.Del(ruser.Key).Err(); err != nil {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}

	m.Email = email
	if err = m.Update(db); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (u *userRouter) putPassword(c echo.Context, l *logrus.Logger) error {
	db := u.service.db.DB()
	ch := u.service.cache.Cache()
	mailer := u.service.mailer

	userid := c.Get("userid").(string)

	ruser := reqUserPutPassword{}
	if err := c.Bind(&ruser); err != nil {
		return governor.NewErrorUser(moduleIDUser, err.Error(), 0, http.StatusBadRequest)
	}
	if err := ruser.valid(u.service.passwordMinSize); err != nil {
		return err
	}

	m, err := usermodel.GetByIDB64(db, userid)
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

	key, err := uid.NewU(0, 16)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	sessionKey := key.Base64()

	if err := ch.Set(sessionKey, userid, time.Duration(u.service.passwordResetTime*b1)).Err(); err != nil {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}

	emdata := emailPassChange{
		FirstName: m.FirstName,
		Username:  m.Username,
		Key:       sessionKey,
	}

	em, err := u.service.tpl.ExecuteHTML(passChangeTemplate, emdata)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	subj, err := u.service.tpl.ExecuteHTML(passChangeSubject, emdata)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	if err := mailer.Send(m.Email, subj, em); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	if err = m.Update(db); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (u *userRouter) forgotPassword(c echo.Context, l *logrus.Logger) error {
	db := u.service.db.DB()
	ch := u.service.cache.Cache()
	mailer := u.service.mailer

	ruser := reqForgotPassword{}
	if err := c.Bind(&ruser); err != nil {
		return governor.NewErrorUser(moduleIDUser, err.Error(), 0, http.StatusBadRequest)
	}
	isEmail := false
	if err := ruser.validEmail(); err == nil {
		isEmail = true
	}
	var m *usermodel.Model
	if isEmail {
		mu, err := usermodel.GetByEmail(db, ruser.Username)
		if err != nil {
			if err.Code() == 2 {
				err.SetErrorUser()
			}
			err.AddTrace(moduleIDUser)
			return err
		}
		m = mu
	} else {
		if err := ruser.valid(); err != nil {
			return err
		}
		mu, err := usermodel.GetByUsername(db, ruser.Username)
		if err != nil {
			if err.Code() == 2 {
				err.SetErrorUser()
			}
			err.AddTrace(moduleIDUser)
			return err
		}
		m = mu
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

	if err := ch.Set(sessionKey, userid, time.Duration(u.service.passwordResetTime*b1)).Err(); err != nil {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}

	emdata := emailForgotPass{
		FirstName: m.FirstName,
		Username:  m.Username,
		Key:       sessionKey,
	}

	em, err := u.service.tpl.ExecuteHTML(forgotPassTemplate, emdata)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	subj, err := u.service.tpl.ExecuteHTML(forgotPassSubject, emdata)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	if err := mailer.Send(m.Email, subj, em); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

func (u *userRouter) forgotPasswordReset(c echo.Context, l *logrus.Logger) error {
	db := u.service.db.DB()
	ch := u.service.cache.Cache()
	mailer := u.service.mailer

	ruser := reqForgotPasswordReset{}
	if err := c.Bind(&ruser); err != nil {
		return governor.NewErrorUser(moduleIDUser, err.Error(), 0, http.StatusBadRequest)
	}
	if err := ruser.valid(u.service.passwordMinSize); err != nil {
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

	emdata := emailPassReset{
		FirstName: m.FirstName,
		Username:  m.Username,
	}

	em, err := u.service.tpl.ExecuteHTML(passResetTemplate, emdata)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	subj, err := u.service.tpl.ExecuteHTML(passResetSubject, emdata)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	if err := mailer.Send(m.Email, subj, em); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	if err := ch.Del(ruser.Key).Err(); err != nil {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}

	if err := m.Update(db); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

func (u *userRouter) patchRank(c echo.Context, l *logrus.Logger) error {
	db := u.service.db.DB()
	ch := u.service.cache.Cache()

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
		l.WithFields(logrus.Fields{
			"time":     string(t),
			"origin":   moduleIDUser,
			"userid":   reqid.Userid,
			"username": m.Username,
		}).Info("admin status added")
	}
	if editRemoveRank.Has("admin") {
		t, _ := time.Now().MarshalText()
		l.WithFields(logrus.Fields{
			"time":     string(t),
			"origin":   moduleIDUser,
			"userid":   reqid.Userid,
			"username": m.Username,
		}).Info("admin status removed")
	}

	diff := make(map[string]int)
	for k, v := range editAddRank {
		if v {
			diff[k] = usermodel.RoleAdd
		}
	}
	for k, v := range editRemoveRank {
		if v {
			diff[k] = usermodel.RoleRemove
		}
	}

	s := session.Session{
		Userid: reqid.Userid,
	}

	var sarr []string
	if sgobs, err := ch.HGetAll(s.UserKey()).Result(); err == nil {
		sarr = make([]string, 0, len(sgobs))
		for _, v := range sgobs {
			s := session.Session{}
			if err = gob.NewDecoder(bytes.NewBufferString(v)).Decode(&s); err != nil {
				return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
			}
			sarr = append(sarr, s.SessionID)
		}
	} else {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}

	if len(sarr) > 0 {
		if err := ch.Del(sarr...).Err(); err != nil {
			return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
		}
		if err := ch.HDel(s.UserKey(), sarr...).Err(); err != nil {
			return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
		}
	}

	if err := m.UpdateRoles(db, diff); err != nil {
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
