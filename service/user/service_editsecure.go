package user

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/util/uid"
	"net/http"
	"strings"
	"time"
)

type (
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
	emailChangeTemplate       = "emailchange"
	emailChangeSubject        = "emailchange_subject"
	emailChangeNotifyTemplate = "emailchangenotify"
	emailChangeNotifySubject  = "emailchangenotify_subject"
)

const (
	emailChangeEscapeSequence = "%email%"
	cachePrefixEmailUpdate    = moduleID + ".updateemail:"
)

// UpdateEmail creates a pending user email update
func (u *userService) UpdateEmail(userid string, newEmail string, password string) *governor.Error {
	m, err := u.repo.GetByIDB64(userid)
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

	if err := u.cache.Cache().Set(cachePrefixEmailUpdate+sessionKey, userid+emailChangeEscapeSequence+newEmail, time.Duration(u.passwordResetTime*b1)).Err(); err != nil {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}

	emdatanotify := emailEmailChangeNotify{
		FirstName: m.FirstName,
		Username:  m.Username,
	}
	if err := u.mailer.Send(m.Email, emailChangeNotifySubject, emailChangeNotifyTemplate, emdatanotify); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	emdata := emailEmailChange{
		FirstName: m.FirstName,
		Username:  m.Username,
		Key:       sessionKey,
	}
	if err := u.mailer.Send(newEmail, emailChangeNotifySubject, emailChangeTemplate, emdata); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	return nil
}

// CommitEmail commits an email update from the cache
func (u *userService) CommitEmail(key string, password string) *governor.Error {
	var userid, email string

	if result, err := u.cache.Cache().Get(cachePrefixEmailUpdate + key).Result(); err == nil {
		k := strings.SplitN(result, emailChangeEscapeSequence, 2)
		if len(k) != 2 {
			return governor.NewError(moduleIDUser, "incorrect sessionKey value in cache during email verification", 0, http.StatusInternalServerError)
		}
		userid = k[0]
		email = k[1]
	} else {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}

	m, err := u.repo.GetByIDB64(userid)
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

	if err := u.cache.Cache().Del(cachePrefixEmailUpdate + key).Err(); err != nil {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}

	m.Email = email
	if err = u.repo.Update(m); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	return nil
}

type (
	emailPassChange struct {
		FirstName string
		Username  string
		Key       string
	}

	emailForgotPass struct {
		FirstName string
		Username  string
		Key       string
	}

	emailPassReset struct {
		FirstName string
		Username  string
	}
)

const (
	passChangeTemplate = "passchange"
	passChangeSubject  = "passchange_subject"
	forgotPassTemplate = "forgotpass"
	forgotPassSubject  = "forgotpass_subject"
	passResetTemplate  = "passreset"
	passResetSubject   = "passreset_subject"
)

const (
	cachePrefixPasswordUpdate = moduleID + ".updatepassword:"
)

// UpdatePassword updates the password
func (u *userService) UpdatePassword(userid string, newPassword string, oldPassword string) *governor.Error {
	m, err := u.repo.GetByIDB64(userid)
	if err != nil {
		if err.Code() == 2 {
			err.SetErrorUser()
		}
		err.AddTrace(moduleIDUser)
		return err
	}
	if !m.ValidatePass(oldPassword) {
		return governor.NewErrorUser(moduleIDUser, "incorrect password", 0, http.StatusForbidden)
	}
	if err = m.RehashPass(newPassword); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	key, err := uid.NewU(0, 16)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	sessionKey := key.Base64()

	if err := u.cache.Cache().Set(cachePrefixPasswordUpdate+sessionKey, userid, time.Duration(u.passwordResetTime*b1)).Err(); err != nil {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}

	emdata := emailPassChange{
		FirstName: m.FirstName,
		Username:  m.Username,
		Key:       sessionKey,
	}
	if err := u.mailer.Send(m.Email, passChangeSubject, passChangeTemplate, emdata); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	if err = u.repo.Update(m); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	return nil
}

// ForgotPassword invokes the forgot password reset procedure
func (u *userService) ForgotPassword(username string, isEmail bool) *governor.Error {
	m := u.repo.NewEmptyPtr()
	if isEmail {
		mu, err := u.repo.GetByEmail(username)
		if err != nil {
			if err.Code() == 2 {
				err.SetErrorUser()
			}
			err.AddTrace(moduleIDUser)
			return err
		}
		m = mu
	} else {
		mu, err := u.repo.GetByUsername(username)
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

	if err := u.cache.Cache().Set(cachePrefixPasswordUpdate+sessionKey, userid, time.Duration(u.passwordResetTime*b1)).Err(); err != nil {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}

	emdata := emailForgotPass{
		FirstName: m.FirstName,
		Username:  m.Username,
		Key:       sessionKey,
	}
	if err := u.mailer.Send(m.Email, forgotPassSubject, forgotPassTemplate, emdata); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	return nil
}

// ResetPassword completes the forgot password procedure
func (u *userService) ResetPassword(key string, newPassword string) *governor.Error {
	userid := ""
	if result, err := u.cache.Cache().Get(cachePrefixPasswordUpdate + key).Result(); err == nil {
		userid = result
	} else {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}

	m, err := u.repo.GetByIDB64(userid)
	if err != nil {
		if err.Code() == 2 {
			err.SetErrorUser()
		}
		err.AddTrace(moduleIDUser)
		return err
	}

	if err := m.RehashPass(newPassword); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	emdata := emailPassReset{
		FirstName: m.FirstName,
		Username:  m.Username,
	}
	if err := u.mailer.Send(m.Email, passResetSubject, passResetTemplate, emdata); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	if err := u.cache.Cache().Del(cachePrefixPasswordUpdate + key).Err(); err != nil {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}

	if err := u.repo.Update(m); err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	return nil
}
