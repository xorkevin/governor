package user

import (
	"net/http"
	"strings"
	"time"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/util/uid"
)

const (
	uidEmailSize = 16
	uidPassSize  = 16
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
func (s *service) UpdateEmail(userid string, newEmail string, password string) error {
	m, err := s.users.GetByID(userid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return governor.NewErrorUser("", 0, err)
		}
		return err
	}
	if m.Email == newEmail {
		return governor.NewErrorUser("Emails cannot be the same", http.StatusBadRequest, err)
	}
	if ok, err := s.users.ValidatePass(password, m); err != nil {
		return err
	} else if !ok {
		return governor.NewErrorUser("Incorrect password", http.StatusForbidden, nil)
	}

	key, err := uid.New(uidEmailSize)
	if err != nil {
		return governor.NewError("Failed to create new uid", http.StatusInternalServerError, err)
	}
	sessionKey := key.Base64()

	if err := s.kv.KVStore().Set(cachePrefixEmailUpdate+sessionKey, userid+emailChangeEscapeSequence+newEmail, time.Duration(s.passwordResetTime)*time.Second).Err(); err != nil {
		return governor.NewError("Failed to store new email info", http.StatusInternalServerError, err)
	}

	emdatanotify := emailEmailChangeNotify{
		FirstName: m.FirstName,
		Username:  m.Username,
	}
	if err := s.mailer.Send(m.Email, emailChangeNotifySubject, emailChangeNotifyTemplate, emdatanotify); err != nil {
		s.logger.Error("fail to send old email change notification", map[string]string{
			"err": err.Error(),
		})
	}

	emdata := emailEmailChange{
		FirstName: m.FirstName,
		Username:  m.Username,
		Key:       sessionKey,
	}
	if err := s.mailer.Send(newEmail, emailChangeNotifySubject, emailChangeTemplate, emdata); err != nil {
		return governor.NewError("Failed to send new email verification", http.StatusInternalServerError, err)
	}
	return nil
}

// CommitEmail commits an email update from the cache
func (s *service) CommitEmail(key string, password string) error {
	result, err := s.kv.KVStore().Get(cachePrefixEmailUpdate + key).Result()
	if err != nil {
		return governor.NewErrorUser("New email verification expired", http.StatusBadRequest, err)
	}

	k := strings.SplitN(result, emailChangeEscapeSequence, 2)
	if len(k) != 2 {
		return governor.NewError("Failed to decode new email info", http.StatusInternalServerError, nil)
	}
	userid := k[0]
	email := k[1]

	m, err := s.users.GetByID(userid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return governor.NewErrorUser("", 0, err)
		}
		return err
	}

	if ok, err := s.users.ValidatePass(password, m); err != nil {
		return err
	} else if !ok {
		return governor.NewErrorUser("Incorrect password", http.StatusForbidden, nil)
	}

	m.Email = email
	if err = s.users.Update(m); err != nil {
		return err
	}

	if err := s.kv.KVStore().Del(cachePrefixEmailUpdate + key).Err(); err != nil {
		s.logger.Error("fail to clean up new email cache data", map[string]string{
			"err": err.Error(),
		})
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
func (s *service) UpdatePassword(userid string, newPassword string, oldPassword string) error {
	m, err := s.users.GetByID(userid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return governor.NewErrorUser("", 0, err)
		}
		return err
	}
	if ok, err := s.users.ValidatePass(oldPassword, m); err != nil {
		return err
	} else if !ok {
		return governor.NewErrorUser("Incorrect password", http.StatusForbidden, err)
	}
	if err := s.users.RehashPass(m, newPassword); err != nil {
		return err
	}

	if err = s.users.Update(m); err != nil {
		return err
	}

	if key, err := uid.New(uidPassSize); err != nil {
		s.logger.Error("fail to create new uid", map[string]string{
			"err": err.Error(),
		})
	} else {
		sessionKey := key.Base64()
		if err := s.kv.KVStore().Set(cachePrefixPasswordUpdate+sessionKey, userid, time.Duration(s.passwordResetTime)*time.Second).Err(); err != nil {
			s.logger.Error("fail to cache undo password change key", map[string]string{
				"err": err.Error(),
			})
		} else {
			emdata := emailPassChange{
				FirstName: m.FirstName,
				Username:  m.Username,
				Key:       sessionKey,
			}
			if err := s.mailer.Send(m.Email, passChangeSubject, passChangeTemplate, emdata); err != nil {
				s.logger.Error("fail to send password change notification email", map[string]string{
					"err": err.Error(),
				})
			}
		}
	}
	return nil
}

// ForgotPassword invokes the forgot password reset procedure
func (s *service) ForgotPassword(username string, isEmail bool) error {
	m := s.users.NewEmptyPtr()
	if isEmail {
		mu, err := s.users.GetByEmail(username)
		if err != nil {
			if governor.ErrorStatus(err) == http.StatusNotFound {
				return governor.NewErrorUser("", 0, err)
			}
			return err
		}
		m = mu
	} else {
		mu, err := s.users.GetByUsername(username)
		if err != nil {
			if governor.ErrorStatus(err) == http.StatusNotFound {
				return governor.NewErrorUser("", 0, err)
			}
			return err
		}
		m = mu
	}

	key, err := uid.New(uidPassSize)
	if err != nil {
		return governor.NewError("Failed to create new uid", http.StatusInternalServerError, err)
	}
	sessionKey := key.Base64()

	if err := s.kv.KVStore().Set(cachePrefixPasswordUpdate+sessionKey, m.Userid, time.Duration(s.passwordResetTime)*time.Second).Err(); err != nil {
		return governor.NewError("Failed to store password reset info", http.StatusInternalServerError, err)
	}

	emdata := emailForgotPass{
		FirstName: m.FirstName,
		Username:  m.Username,
		Key:       sessionKey,
	}
	if err := s.mailer.Send(m.Email, forgotPassSubject, forgotPassTemplate, emdata); err != nil {
		return governor.NewError("Failed to send password reset email", http.StatusInternalServerError, err)
	}
	return nil
}

// ResetPassword completes the forgot password procedure
func (s *service) ResetPassword(key string, newPassword string) error {
	userid, err := s.kv.KVStore().Get(cachePrefixPasswordUpdate + key).Result()
	if err != nil {
		return governor.NewError("Password reset expired", http.StatusBadRequest, err)
	}

	m, err := s.users.GetByID(userid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return governor.NewErrorUser("", 0, err)
		}
		return err
	}

	if err := s.users.RehashPass(m, newPassword); err != nil {
		return err
	}

	if err := s.users.Update(m); err != nil {
		return err
	}

	emdata := emailPassReset{
		FirstName: m.FirstName,
		Username:  m.Username,
	}
	if err := s.mailer.Send(m.Email, passResetSubject, passResetTemplate, emdata); err != nil {
		s.logger.Error("fail to send password change notification email", map[string]string{
			"err": err.Error(),
		})
	}

	if err := s.kv.KVStore().Del(cachePrefixPasswordUpdate + key).Err(); err != nil {
		s.logger.Error("fail to clean up password reset cache data", map[string]string{
			"err": err.Error(),
		})
	}
	return nil
}
