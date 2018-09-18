package user

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/model"
	"github.com/hackform/governor/util/uid"
	"net/http"
	"strings"
	"time"
)

// UpdateEmail creates a pending user email update
func (u *userService) UpdateEmail(userid string, newEmail string, password string) *governor.Error {
	m, err := usermodel.GetByIDB64(u.db.DB(), userid)
	if err != nil {
		if err.Code() == 2 {
			err.SetErrorUser()
		}
		err.AddTrace(moduleIDUser)
		return err
	}
	if m.Email == newEmail {
		return governor.NewErrorUser(moduleIDUser, "emails cannot be the same", 0, http.StatusBadRequest)
	}
	if !m.ValidatePass(password) {
		return governor.NewErrorUser(moduleIDUser, "incorrect password", 0, http.StatusForbidden)
	}

	key, err := uid.NewU(0, 16)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	sessionKey := key.Base64()

	if err := u.cache.Cache().Set(sessionKey, userid+emailChangeEscapeSequence+newEmail, time.Duration(u.passwordResetTime*b1)).Err(); err != nil {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}

	emdata := emailEmailChange{
		FirstName: m.FirstName,
		Username:  m.Username,
		Key:       sessionKey,
	}

	em, err := u.tpl.ExecuteHTML(emailChangeTemplate, emdata)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	subj, err := u.tpl.ExecuteHTML(emailChangeSubject, emdata)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	emdatanotify := emailEmailChangeNotify{
		FirstName: m.FirstName,
		Username:  m.Username,
	}

	emnotify, err := u.tpl.ExecuteHTML(emailChangeNotifyTemplate, emdatanotify)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	subjnotify, err := u.tpl.ExecuteHTML(emailChangeNotifySubject, emdatanotify)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	if err := u.mailer.Send(m.Email, subjnotify, emnotify); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	if err := u.mailer.Send(newEmail, subj, em); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	return nil
}

// CommitEmail commits an email update from the cache
func (u *userService) CommitEmail(key string, password string) *governor.Error {
	var userid, email string

	if result, err := u.cache.Cache().Get(key).Result(); err == nil {
		k := strings.SplitN(result, emailChangeEscapeSequence, 2)
		if len(k) != 2 {
			return governor.NewError(moduleIDUser, "incorrect sessionKey value in cache during email verification", 0, http.StatusInternalServerError)
		}
		userid = k[0]
		email = k[1]
	} else {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}

	m, err := usermodel.GetByIDB64(u.db.DB(), userid)
	if err != nil {
		if err.Code() == 2 {
			err.SetErrorUser()
		}
		err.AddTrace(moduleIDUser)
		return err
	}

	if !m.ValidatePass(password) {
		return governor.NewErrorUser(moduleIDUser, "incorrect password", 0, http.StatusForbidden)
	}

	if err := u.cache.Cache().Del(key).Err(); err != nil {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}

	m.Email = email
	if err = m.Update(u.db.DB()); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	return nil
}
