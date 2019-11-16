package user

import (
	"net/http"
	"strings"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/util/uid"
)

const (
	uidEmailSize = 16
	uidPassSize  = 16
)

type (
	emailEmailChange struct {
		Userid    string
		Key       string
		FirstName string
		Username  string
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
	emailChangeSeparator = "|email|"
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
	nonce := key.Base64()
	noncehash, err := s.hasher.Hash(nonce)
	if err != nil {
		return governor.NewError("Failed to hash email reset key", http.StatusInternalServerError, err)
	}

	if err := s.kvemailchange.Set(userid, noncehash+emailChangeSeparator+newEmail, s.passwordResetTime); err != nil {
		return governor.NewError("Failed to store new email info", http.StatusInternalServerError, err)
	}

	emdata := emailEmailChange{
		Userid:    userid,
		Key:       nonce,
		FirstName: m.FirstName,
		Username:  m.Username,
	}
	if err := s.mailer.Send("", "", newEmail, emailChangeSubject, emailChangeTemplate, emdata); err != nil {
		return governor.NewError("Failed to send new email verification", http.StatusInternalServerError, err)
	}
	return nil
}

// CommitEmail commits an email update from the cache
func (s *service) CommitEmail(userid string, key string, password string) error {
	result, err := s.kvemailchange.Get(userid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return governor.NewErrorUser("New email verification expired", http.StatusBadRequest, err)
		}
		return governor.NewError("Failed to get user email reset info", http.StatusInternalServerError, err)
	}

	k := strings.SplitN(result, emailChangeSeparator, 2)
	if len(k) != 2 {
		return governor.NewError("Failed to decode new email info", http.StatusInternalServerError, nil)
	}
	noncehash := k[0]
	newEmail := k[1]

	if ok, err := s.verifier.Verify(key, noncehash); err != nil {
		return governor.NewError("Failed to verify key", http.StatusInternalServerError, err)
	} else if !ok {
		return governor.NewErrorUser("Invalid key", http.StatusForbidden, nil)
	}

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

	m.Email = newEmail
	if err = s.users.Update(m); err != nil {
		return err
	}

	if err := s.kvemailchange.Del(userid); err != nil {
		s.logger.Error("failed to clean up new email info", map[string]string{
			"error":      err.Error(),
			"actiontype": "commitemailcleanup",
		})
	}

	emdatanotify := emailEmailChangeNotify{
		FirstName: m.FirstName,
		Username:  m.Username,
	}
	if err := s.mailer.Send("", "", m.Email, emailChangeNotifySubject, emailChangeNotifyTemplate, emdatanotify); err != nil {
		s.logger.Error("failed to send old email change notification", map[string]string{
			"error":      err.Error(),
			"actiontype": "commitemailoldmail",
		})
	}
	return nil
}

type (
	emailPassChange struct {
		FirstName string
		Username  string
	}

	emailForgotPass struct {
		Userid    string
		Key       string
		FirstName string
		Username  string
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

	emdata := emailPassChange{
		FirstName: m.FirstName,
		Username:  m.Username,
	}
	if err := s.mailer.Send("", "", m.Email, passChangeSubject, passChangeTemplate, emdata); err != nil {
		s.logger.Error("failed to send password change notification email", map[string]string{
			"error":      err.Error(),
			"actiontype": "updatepasswordmail",
		})
	}
	return nil
}

// ForgotPassword invokes the forgot password reset procedure
func (s *service) ForgotPassword(useroremail string) error {
	m := s.users.NewEmptyPtr()
	if isEmail(useroremail) {
		mu, err := s.users.GetByEmail(useroremail)
		if err != nil {
			if governor.ErrorStatus(err) == http.StatusNotFound {
				return governor.NewErrorUser("", 0, err)
			}
			return err
		}
		m = mu
	} else {
		mu, err := s.users.GetByUsername(useroremail)
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
	nonce := key.Base64()
	noncehash, err := s.hasher.Hash(nonce)
	if err != nil {
		return governor.NewError("Failed to hash password reset key", http.StatusInternalServerError, err)
	}

	if err := s.kvpassreset.Set(m.Userid, noncehash, s.passwordResetTime); err != nil {
		return governor.NewError("Failed to store password reset info", http.StatusInternalServerError, err)
	}

	emdata := emailForgotPass{
		Userid:    m.Userid,
		Key:       nonce,
		FirstName: m.FirstName,
		Username:  m.Username,
	}
	if err := s.mailer.Send("", "", m.Email, forgotPassSubject, forgotPassTemplate, emdata); err != nil {
		return governor.NewError("Failed to send password reset email", http.StatusInternalServerError, err)
	}
	return nil
}

// ResetPassword completes the forgot password procedure
func (s *service) ResetPassword(userid string, key string, newPassword string) error {
	noncehash, err := s.kvpassreset.Get(userid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return governor.NewErrorUser("Password reset expired", http.StatusBadRequest, err)
		}
		return governor.NewError("Failed to reset password", http.StatusInternalServerError, err)
	}

	if ok, err := s.verifier.Verify(key, noncehash); err != nil {
		return governor.NewError("Failed to verify key", http.StatusInternalServerError, err)
	} else if !ok {
		return governor.NewErrorUser("Invalid key", http.StatusForbidden, nil)
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

	if err := s.kvpassreset.Del(userid); err != nil {
		s.logger.Error("failed to clean up password reset info", map[string]string{
			"error":      err.Error(),
			"actiontype": "resetpasswordcleanup",
		})
	}

	emdata := emailPassReset{
		FirstName: m.FirstName,
		Username:  m.Username,
	}
	if err := s.mailer.Send("", "", m.Email, passResetSubject, passResetTemplate, emdata); err != nil {
		s.logger.Error("failed to send password change notification email", map[string]string{
			"error":      err.Error(),
			"actiontype": "resetpasswordmail",
		})
	}
	return nil
}
