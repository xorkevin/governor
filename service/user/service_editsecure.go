package user

import (
	"bytes"
	"errors"
	htmlTemplate "html/template"
	"net/http"
	"net/url"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/user/model"
)

const (
	kindResetEmail = "email"
	kindResetPass  = "pass"
)

type (
	emailEmailChange struct {
		Userid    string
		Key       string
		URL       string
		FirstName string
		LastName  string
		Username  string
	}

	queryEmailEmailChange struct {
		Userid    string
		Key       string
		FirstName string
		LastName  string
		Username  string
	}

	emailEmailChangeNotify struct {
		FirstName string
		LastName  string
		Username  string
	}
)

const (
	emailChangeTemplate       = "emailchange"
	emailChangeNotifyTemplate = "emailchangenotify"
)

func (e *emailEmailChange) Query() queryEmailEmailChange {
	return queryEmailEmailChange{
		Userid:    url.QueryEscape(e.Userid),
		Key:       url.QueryEscape(e.Key),
		FirstName: url.QueryEscape(e.FirstName),
		LastName:  url.QueryEscape(e.LastName),
		Username:  url.QueryEscape(e.Username),
	}
}

func (e *emailEmailChange) computeURL(base string, tpl *htmlTemplate.Template) error {
	b := &bytes.Buffer{}
	if err := tpl.Execute(b, e.Query()); err != nil {
		return governor.ErrWithMsg(err, "Failed executing email change url template")
	}
	e.URL = base + b.String()
	return nil
}

// UpdateEmail creates a pending user email update
func (s *service) UpdateEmail(userid string, newEmail string, password string) error {
	if _, err := s.users.GetByEmail(newEmail); err != nil {
		if !errors.Is(err, db.ErrNotFound{}) {
			return governor.ErrWithMsg(err, "Failed to get user")
		}
	} else {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Email is already in use",
		}))
	}
	m, err := s.users.GetByID(userid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "User not found",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to get user")
	}
	if m.Email == newEmail {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Emails cannot be the same",
		}), governor.ErrOptInner(err))
	}
	if ok, err := s.users.ValidatePass(password, m); err != nil {
		return governor.ErrWithMsg(err, "Failed to validate password")
	} else if !ok {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusUnauthorized,
			Message: "Incorrect password",
		}))
	}

	needInsert := false
	mr, err := s.resets.GetByID(m.Userid, kindResetEmail)
	if err != nil {
		if !errors.Is(err, db.ErrNotFound{}) {
			return governor.ErrWithMsg(err, "Failed to get user")
		}
		needInsert = true
		mr = s.resets.New(m.Userid, kindResetEmail)
	}
	code, err := s.resets.RehashCode(mr)
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to generate email reset code")
	}
	mr.Params = newEmail
	if needInsert {
		if err := s.resets.Insert(mr); err != nil {
			return governor.ErrWithMsg(err, "Failed to create email reset request")
		}
	} else if err := s.resets.Update(mr); err != nil {
		return governor.ErrWithMsg(err, "Failed to update email reset request")
	}

	emdata := emailEmailChange{
		Userid:    userid,
		Key:       code,
		FirstName: m.FirstName,
		LastName:  m.LastName,
		Username:  m.Username,
	}
	if err := emdata.computeURL(s.emailurlbase, s.tplemailchange); err != nil {
		return governor.ErrWithMsg(err, "Failed to generate new email verification email")
	}
	if err := s.mailer.Send("", "", []string{newEmail}, emailChangeTemplate, emdata); err != nil {
		return governor.ErrWithMsg(err, "Failed to send new email verification email")
	}
	return nil
}

// CommitEmail commits an email update from the cache
func (s *service) CommitEmail(userid string, key string, password string) error {
	mr, err := s.resets.GetByID(userid, kindResetEmail)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusBadRequest,
				Message: "New email verification expired",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to get email reset request")
	}

	if time.Now().Round(0).Unix() > mr.CodeTime+s.passwordResetTime {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "New email verification expired",
		}))
	}
	if ok, err := s.resets.ValidateCode(key, mr); err != nil {
		return governor.ErrWithMsg(err, "Failed to validate email reset code")
	} else if !ok {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusUnauthorized,
			Message: "Invalid code",
		}))
	}

	m, err := s.users.GetByID(userid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "User not found",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to get user")
	}

	if ok, err := s.users.ValidatePass(password, m); err != nil {
		return governor.ErrWithMsg(err, "Failed to validate password")
	} else if !ok {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusUnauthorized,
			Message: "Incorrect password",
		}))
	}

	m.Email = mr.Params

	if err := s.resets.Delete(userid, kindResetEmail); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete email reset request")
	}

	if err = s.users.Update(m); err != nil {
		if errors.Is(err, db.ErrUnique{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusBadRequest,
				Message: "Email is already in use by another account",
			}))
		}
		return governor.ErrWithMsg(err, "Failed to update email")
	}

	emdatanotify := emailEmailChangeNotify{
		FirstName: m.FirstName,
		LastName:  m.LastName,
		Username:  m.Username,
	}
	if err := s.mailer.Send("", "", []string{m.Email}, emailChangeNotifyTemplate, emdatanotify); err != nil {
		s.logger.Error("Failed to send old email change notification", map[string]string{
			"error":      err.Error(),
			"actiontype": "commitemailoldmail",
		})
	}
	return nil
}

type (
	emailPassChange struct {
		FirstName string
		LastName  string
		Username  string
	}

	emailForgotPass struct {
		Userid    string
		Key       string
		URL       string
		FirstName string
		LastName  string
		Username  string
	}

	queryEmailForgotPass struct {
		Userid    string
		Key       string
		FirstName string
		LastName  string
		Username  string
	}

	emailPassReset struct {
		FirstName string
		LastName  string
		Username  string
	}
)

func (e *emailForgotPass) Query() queryEmailForgotPass {
	return queryEmailForgotPass{
		Userid:    url.QueryEscape(e.Userid),
		Key:       url.QueryEscape(e.Key),
		FirstName: url.QueryEscape(e.FirstName),
		LastName:  url.QueryEscape(e.LastName),
		Username:  url.QueryEscape(e.Username),
	}
}

func (e *emailForgotPass) computeURL(base string, tpl *htmlTemplate.Template) error {
	b := &bytes.Buffer{}
	if err := tpl.Execute(b, e.Query()); err != nil {
		return governor.ErrWithMsg(err, "Failed executing forgot pass url template")
	}
	e.URL = base + b.String()
	return nil
}

const (
	passChangeTemplate = "passchange"
	forgotPassTemplate = "forgotpass"
	passResetTemplate  = "passreset"
)

// UpdatePassword updates the password
func (s *service) UpdatePassword(userid string, newPassword string, oldPassword string) error {
	m, err := s.users.GetByID(userid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "User not found",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to get user")
	}
	if ok, err := s.users.ValidatePass(oldPassword, m); err != nil {
		return governor.ErrWithMsg(err, "Failed to validate password")
	} else if !ok {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusUnauthorized,
			Message: "Incorrect password",
		}))
	}
	if err := s.users.RehashPass(m, newPassword); err != nil {
		return governor.ErrWithMsg(err, "Failed hashing password")
	}

	if err = s.users.Update(m); err != nil {
		return governor.ErrWithMsg(err, "Failed to update user")
	}

	emdata := emailPassChange{
		FirstName: m.FirstName,
		LastName:  m.LastName,
		Username:  m.Username,
	}
	if err := s.mailer.Send("", "", []string{m.Email}, passChangeTemplate, emdata); err != nil {
		s.logger.Error("Failed to send password change notification email", map[string]string{
			"error":      err.Error(),
			"actiontype": "updatepasswordmail",
		})
	}
	return nil
}

// ForgotPassword invokes the forgot password reset procedure
func (s *service) ForgotPassword(useroremail string) error {
	var m *model.Model
	if isEmail(useroremail) {
		mu, err := s.users.GetByEmail(useroremail)
		if err != nil {
			if errors.Is(err, db.ErrNotFound{}) {
				// prevent email scanning
				return nil
			}
			return governor.ErrWithMsg(err, "Failed to get user")
		}
		m = mu
	} else {
		mu, err := s.users.GetByUsername(useroremail)
		if err != nil {
			if errors.Is(err, db.ErrNotFound{}) {
				return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
					Status:  http.StatusNotFound,
					Message: "User not found",
				}), governor.ErrOptInner(err))
			}
			return governor.ErrWithMsg(err, "Failed to get user")
		}
		m = mu
	}

	needInsert := false
	mr, err := s.resets.GetByID(m.Userid, kindResetPass)
	if err != nil {
		if !errors.Is(err, db.ErrNotFound{}) {
			return governor.ErrWithMsg(err, "Failed to get user")
		}
		needInsert = true
		mr = s.resets.New(m.Userid, kindResetPass)
	}
	code, err := s.resets.RehashCode(mr)
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to generate password reset code")
	}
	if needInsert {
		if err := s.resets.Insert(mr); err != nil {
			return governor.ErrWithMsg(err, "Failed to create password reset request")
		}
	} else {
		if err := s.resets.Update(mr); err != nil {
			return governor.ErrWithMsg(err, "Failed to update password reset request")
		}
	}

	emdata := emailForgotPass{
		Userid:    m.Userid,
		Key:       code,
		FirstName: m.FirstName,
		LastName:  m.LastName,
		Username:  m.Username,
	}
	if err := emdata.computeURL(s.emailurlbase, s.tplforgotpass); err != nil {
		return governor.ErrWithMsg(err, "Failed to generate password reset email")
	}
	if err := s.mailer.Send("", "", []string{m.Email}, forgotPassTemplate, emdata); err != nil {
		return governor.ErrWithMsg(err, "Failed to send password reset email")
	}
	return nil
}

// ResetPassword completes the forgot password procedure
func (s *service) ResetPassword(userid string, key string, newPassword string) error {
	mr, err := s.resets.GetByID(userid, kindResetPass)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "Password reset expired",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to get password reset request")
	}

	if time.Now().Round(0).Unix() > mr.CodeTime+s.passwordResetTime {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusNotFound,
			Message: "Password reset expired",
		}))
	}
	if ok, err := s.resets.ValidateCode(key, mr); err != nil {
		return governor.ErrWithMsg(err, "Failed to validate password reset code")
	} else if !ok {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusUnauthorized,
			Message: "Invalid code",
		}))
	}

	m, err := s.users.GetByID(userid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "User not found",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to get user")
	}

	if err := s.users.RehashPass(m, newPassword); err != nil {
		return governor.ErrWithMsg(err, "Failed hashing password")
	}

	if err := s.resets.Delete(userid, kindResetPass); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete password reset request")
	}

	if err := s.users.Update(m); err != nil {
		return governor.ErrWithMsg(err, "Failed to update password")
	}

	emdata := emailPassReset{
		FirstName: m.FirstName,
		LastName:  m.LastName,
		Username:  m.Username,
	}
	if err := s.mailer.Send("", "", []string{m.Email}, passResetTemplate, emdata); err != nil {
		s.logger.Error("Failed to send password change notification email", map[string]string{
			"error":      err.Error(),
			"actiontype": "resetpasswordmail",
		})
	}
	return nil
}

type (
	resAddOTP struct {
		URI    string `json:"uri"`
		Backup string `json:"backup"`
	}
)

// AddOTP adds an otp secret
func (s *service) AddOTP(userid string, alg string, digits int) (*resAddOTP, error) {
	m, err := s.users.GetByID(userid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "User not found",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to get user")
	}
	if m.OTPEnabled {
		return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "OTP already enabled",
		}))
	}
	uri, backup, err := s.users.GenerateOTPSecret(s.otpCipher, m, s.otpIssuer, alg, digits)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to generate otp secret")
	}
	m.FailedLoginTime = 0
	m.FailedLoginCount = 0
	if err := s.users.Update(m); err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to update otp secret")
	}
	return &resAddOTP{
		URI:    uri,
		Backup: backup,
	}, nil
}

// CommitOTP commits to using an otp
func (s *service) CommitOTP(userid string, code string) error {
	m, err := s.users.GetByID(userid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "User not found",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to get user")
	}
	if m.OTPEnabled {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "OTP already enabled",
		}))
	}
	if m.OTPSecret == "" {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "OTP secret not yet added",
		}))
	}
	if ok, err := s.users.ValidateOTPCode(s.otpDecrypter, m, code); err != nil {
		return governor.ErrWithMsg(err, "Failed to validate otp code")
	} else if !ok {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Incorrect otp code",
		}))
	}
	m.OTPEnabled = true
	m.FailedLoginTime = 0
	m.FailedLoginCount = 0
	if err := s.users.Update(m); err != nil {
		return governor.ErrWithMsg(err, "Failed to update otp secret")
	}
	return nil
}

func (s *service) checkOTPCode(m *model.Model, code string, backup string) error {
	var k int64
	if m.FailedLoginCount > 293 || k < 0 {
		k = time24h
	} else {
		k = int64(m.FailedLoginCount) * int64(m.FailedLoginCount)
	}
	if time.Now().Round(0).Unix() < m.FailedLoginTime+k {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Rate limit",
		}))
	}
	if code == "" {
		if ok, err := s.users.ValidateOTPBackup(s.otpDecrypter, m, backup); err != nil {
			return governor.ErrWithMsg(err, "Failed to validate otp backup code")
		} else if !ok {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusBadRequest,
				Message: "Incorrect otp backup code",
			}))
		}
	} else {
		if ok, err := s.users.ValidateOTPCode(s.otpDecrypter, m, code); err != nil {
			return governor.ErrWithMsg(err, "Failed to validate otp code")
		} else if !ok {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusBadRequest,
				Message: "Incorrect otp code",
			}))
		}
	}
	return nil
}

func (s *service) incrOTPFailCount(m *model.Model) {
	m.FailedLoginTime = time.Now().Round(0).Unix()
	m.FailedLoginCount += 1
	if err := s.users.Update(m); err != nil {
		s.logger.Error("Failed to update otp fail count", map[string]string{
			"error":      err.Error(),
			"actiontype": "incrotpfailcount",
		})
	}
}

func (s *service) resetOTPFailCount(m *model.Model) {
	m.FailedLoginTime = 0
	m.FailedLoginCount = 0
	if err := s.users.Update(m); err != nil {
		s.logger.Error("Failed to reset otp fail count", map[string]string{
			"error":      err.Error(),
			"actiontype": "resetotpfailcount",
		})
	}
}

// RemoveOTP removes using otp
func (s *service) RemoveOTP(userid string, code string, backup string) error {
	m, err := s.users.GetByID(userid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "User not found",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to get user")
	}
	if !m.OTPEnabled {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "OTP already disabled",
		}))
	}
	if err := s.checkOTPCode(m, code, backup); err != nil {
		s.incrOTPFailCount(m)
		return err
	}
	m.OTPEnabled = false
	m.OTPSecret = ""
	m.OTPBackup = ""
	m.FailedLoginTime = 0
	m.FailedLoginCount = 0
	if err := s.users.Update(m); err != nil {
		return governor.ErrWithMsg(err, "Failed to update otp secret")
	}
	return nil
}
