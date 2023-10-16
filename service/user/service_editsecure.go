package user

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	htmlTemplate "html/template"
	"net/http"
	"net/url"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/dbsql"
	"xorkevin.dev/governor/service/mail"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

const (
	kindResetEmail = "email"
	kindResetPass  = "pass"
)

type (
	emailEmailChange struct {
		Userid    string `json:"Userid"`
		Key       string `json:"Key"`
		URL       string `json:"URL"`
		FirstName string `json:"FirstName"`
		LastName  string `json:"LastName"`
		Username  string `json:"Username"`
	}

	queryEmailEmailChange struct {
		Userid    string `json:"Userid"`
		Key       string `json:"Key"`
		FirstName string `json:"FirstName"`
		LastName  string `json:"LastName"`
		Username  string `json:"Username"`
	}

	emailEmailChangeNotify struct {
		FirstName string `json:"FirstName"`
		LastName  string `json:"LastName"`
		Username  string `json:"Username"`
	}
)

func (e *emailEmailChange) query() queryEmailEmailChange {
	return queryEmailEmailChange{
		Userid:    url.QueryEscape(e.Userid),
		Key:       url.QueryEscape(e.Key),
		FirstName: url.QueryEscape(e.FirstName),
		LastName:  url.QueryEscape(e.LastName),
		Username:  url.QueryEscape(e.Username),
	}
}

func (e *emailEmailChange) computeURL(base string, tpl *htmlTemplate.Template) error {
	var b bytes.Buffer
	if err := tpl.Execute(&b, e.query()); err != nil {
		return kerrors.WithMsg(err, "Failed executing email change url template")
	}
	e.URL = base + b.String()
	return nil
}

func (s *Service) updateEmail(ctx context.Context, userid string, newEmail string) error {
	if _, err := s.users.GetByEmail(ctx, newEmail); err != nil {
		if !errors.Is(err, dbsql.ErrNotFound) {
			return kerrors.WithMsg(err, "Failed to get user")
		}
	} else {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Email is already in use")
	}
	m, err := s.users.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, dbsql.ErrNotFound) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "User not found")
		}
		return kerrors.WithMsg(err, "Failed to get user")
	}
	if m.Email == newEmail {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Emails cannot be the same")
	}

	needInsert := false
	mr, err := s.resets.GetByID(ctx, m.Userid, kindResetEmail)
	if err != nil {
		if !errors.Is(err, dbsql.ErrNotFound) {
			return kerrors.WithMsg(err, "Failed to get user")
		}
		needInsert = true
		mr = s.resets.New(m.Userid, kindResetEmail)
	}
	code, err := s.resets.RehashCode(mr)
	if err != nil {
		return kerrors.WithMsg(err, "Failed to generate email reset code")
	}
	mr.Params = newEmail
	if needInsert {
		if err := s.resets.Insert(ctx, mr); err != nil {
			return kerrors.WithMsg(err, "Failed to create email reset request")
		}
	} else if err := s.resets.Update(ctx, mr); err != nil {
		return kerrors.WithMsg(err, "Failed to update email reset request")
	}

	emdata := emailEmailChange{
		Userid:    userid,
		Key:       code,
		FirstName: m.FirstName,
		LastName:  m.LastName,
		Username:  m.Username,
	}
	if err := emdata.computeURL(s.emailSettings.urlTpl.base, s.emailSettings.urlTpl.emailChange); err != nil {
		return kerrors.WithMsg(err, "Failed to generate new email verification email")
	}
	if err := s.mailer.SendTpl(
		ctx,
		"",
		mail.Addr{},
		[]mail.Addr{{Address: newEmail, Name: m.FirstName}},
		mail.TplLocal(s.emailSettings.tplName.emailchange),
		emdata,
		true,
	); err != nil {
		return kerrors.WithMsg(err, "Failed to send new email verification email")
	}
	return nil
}

func (s *Service) commitEmail(ctx context.Context, userid string, key string) error {
	mr, err := s.resets.GetByID(ctx, userid, kindResetEmail)
	if err != nil {
		if errors.Is(err, dbsql.ErrNotFound) {
			return governor.ErrWithRes(err, http.StatusBadRequest, "", "New email verification expired")
		}
		return kerrors.WithMsg(err, "Failed to get email reset request")
	}

	if time.Now().Round(0).After(time.Unix(mr.CodeTime, 0).Add(s.editSettings.newEmailConfirmDuration)) {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "New email verification expired")
	}
	if ok, err := s.resets.ValidateCode(key, mr); err != nil {
		return kerrors.WithMsg(err, "Failed to validate email reset code")
	} else if !ok {
		return governor.ErrWithRes(nil, http.StatusUnauthorized, "", "Invalid code")
	}

	m, err := s.users.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, dbsql.ErrNotFound) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "User not found")
		}
		return kerrors.WithMsg(err, "Failed to get user")
	}

	oldEmail := m.Email
	m.Email = mr.Params

	if err := s.resets.Delete(ctx, userid, kindResetEmail); err != nil {
		return kerrors.WithMsg(err, "Failed to delete email reset request")
	}

	if err = s.users.UpdateEmail(ctx, m); err != nil {
		if errors.Is(err, dbsql.ErrUnique) {
			return governor.ErrWithRes(err, http.StatusBadRequest, "", "Email is already in use by another account")
		}
		return kerrors.WithMsg(err, "Failed to update email")
	}

	emdatanotify := emailEmailChangeNotify{
		FirstName: m.FirstName,
		LastName:  m.LastName,
		Username:  m.Username,
	}
	// must make a best effort attempt to send the email
	ctx = klog.ExtendCtx(context.Background(), ctx)
	if err := s.mailer.SendTpl(
		ctx,
		"",
		mail.Addr{},
		[]mail.Addr{{Address: oldEmail, Name: m.FirstName}},
		mail.TplLocal(s.emailSettings.tplName.emailchangenotify),
		emdatanotify,
		false,
	); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to send old email change notification"))
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

func (e *emailForgotPass) query() queryEmailForgotPass {
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
	if err := tpl.Execute(b, e.query()); err != nil {
		return kerrors.WithMsg(err, "Failed executing forgot pass url template")
	}
	e.URL = base + b.String()
	return nil
}

func (s *Service) updatePassword(ctx context.Context, userid string, newPassword string, oldPassword string) error {
	if len(newPassword) < s.authSettings.passMinSize {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", fmt.Sprintf("Password must be at least %d chars", s.authSettings.passMinSize))
	}

	m, err := s.users.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, dbsql.ErrNotFound) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "User not found")
		}
		return kerrors.WithMsg(err, "Failed to get user")
	}
	if ok, err := s.users.ValidatePass(oldPassword, m); err != nil {
		return kerrors.WithMsg(err, "Failed to validate password")
	} else if !ok {
		return governor.ErrWithRes(nil, http.StatusUnauthorized, "", "Incorrect password")
	}
	if err := s.users.RehashPass(ctx, m, newPassword); err != nil {
		return kerrors.WithMsg(err, "Failed updating password")
	}

	emdata := emailPassChange{
		FirstName: m.FirstName,
		LastName:  m.LastName,
		Username:  m.Username,
	}
	// must make best effort attempt to send the email
	ctx = klog.ExtendCtx(context.Background(), ctx)
	if err := s.mailer.SendTpl(ctx,
		"",
		mail.Addr{},
		[]mail.Addr{{Address: m.Email, Name: m.FirstName}},
		mail.TplLocal(s.emailSettings.tplName.passchange),
		emdata,
		false,
	); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to send password change notification email"))
	}
	return nil
}

func (s *Service) forgotPassword(ctx context.Context, userid string) error {
	if !s.editSettings.passReset {
		return governor.ErrWithRes(nil, http.StatusConflict, "", "Password reset not enabled")
	}

	m, err := s.users.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, dbsql.ErrNotFound) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "User not found")
		}
		return kerrors.WithMsg(err, "Failed to get user")
	}

	needInsert := false
	mr, err := s.resets.GetByID(ctx, userid, kindResetPass)
	if err != nil {
		if !errors.Is(err, dbsql.ErrNotFound) {
			return kerrors.WithMsg(err, "Failed to get password reset")
		}
		needInsert = true
		mr = s.resets.New(userid, kindResetPass)
	} else {
		cliff := time.Unix(mr.CodeTime, 0).Add(s.editSettings.passResetDelay)
		if time.Now().Round(0).Before(cliff) {
			s.log.Warn(ctx, "Forgot password called prior to delay end",
				klog.AString("userid", userid),
			)
			return governor.ErrWithTooManyRequests(nil, cliff, "", "Failed login too many times")
		}
	}
	code, err := s.resets.RehashCode(mr)
	if err != nil {
		return kerrors.WithMsg(err, "Failed to generate password reset code")
	}
	if needInsert {
		if err := s.resets.Insert(ctx, mr); err != nil {
			return kerrors.WithMsg(err, "Failed to create password reset request")
		}
	} else {
		if err := s.resets.Update(ctx, mr); err != nil {
			return kerrors.WithMsg(err, "Failed to update password reset request")
		}
	}

	emdata := emailForgotPass{
		Userid:    m.Userid,
		Key:       code,
		FirstName: m.FirstName,
		LastName:  m.LastName,
		Username:  m.Username,
	}
	if err := emdata.computeURL(s.emailSettings.urlTpl.base, s.emailSettings.urlTpl.forgotPass); err != nil {
		return kerrors.WithMsg(err, "Failed to generate password reset email")
	}
	if err := s.mailer.SendTpl(
		ctx,
		"",
		mail.Addr{},
		[]mail.Addr{{Address: m.Email, Name: m.FirstName}},
		mail.TplLocal(s.emailSettings.tplName.forgotpass),
		emdata,
		true,
	); err != nil {
		return kerrors.WithMsg(err, "Failed to send password reset email")
	}
	return nil
}

func (s *Service) resetPassword(ctx context.Context, userid string, key string, newPassword string) error {
	if len(newPassword) < s.authSettings.passMinSize {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", fmt.Sprintf("Password must be at least %d chars", s.authSettings.passMinSize))
	}

	mr, err := s.resets.GetByID(ctx, userid, kindResetPass)
	if err != nil {
		if errors.Is(err, dbsql.ErrNotFound) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "Password reset expired")
		}
		return kerrors.WithMsg(err, "Failed to get password reset request")
	}

	if time.Now().Round(0).After(time.Unix(mr.CodeTime, 0).Add(s.editSettings.passResetDuration)) {
		return governor.ErrWithRes(nil, http.StatusNotFound, "", "Password reset expired")
	}
	if ok, err := s.resets.ValidateCode(key, mr); err != nil {
		return kerrors.WithMsg(err, "Failed to validate password reset code")
	} else if !ok {
		return governor.ErrWithRes(nil, http.StatusUnauthorized, "", "Invalid code")
	}

	m, err := s.users.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, dbsql.ErrNotFound) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "User not found")
		}
		return kerrors.WithMsg(err, "Failed to get user")
	}

	if err := s.resets.Delete(ctx, userid, kindResetPass); err != nil {
		return kerrors.WithMsg(err, "Failed to delete password reset request")
	}

	if err := s.users.RehashPass(ctx, m, newPassword); err != nil {
		return kerrors.WithMsg(err, "Failed hashing password")
	}

	emdata := emailPassReset{
		FirstName: m.FirstName,
		LastName:  m.LastName,
		Username:  m.Username,
	}
	// must make best effort attempt to send the email
	ctx = klog.ExtendCtx(context.Background(), ctx)
	if err := s.mailer.SendTpl(
		ctx,
		"",
		mail.Addr{},
		[]mail.Addr{{Address: m.Email, Name: m.FirstName}},
		mail.TplLocal(s.emailSettings.tplName.passreset),
		emdata,
		false,
	); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to send password change notification email"))
	}
	return nil
}

type (
	emailLoginRatelimit struct {
		FirstName string `json:"FirstName"`
		LastName  string `json:"LastName"`
		Username  string `json:"Username"`
		IP        string `json:"IP"`
		UserAgent string `json:"UserAgent"`
		Time      string `json:"Time"`
	}

	emailOTPBackupUsed struct {
		FirstName string `json:"FirstName"`
		LastName  string `json:"LastName"`
		Username  string `json:"Username"`
		IP        string `json:"IP"`
		UserAgent string `json:"UserAgent"`
		Time      string `json:"Time"`
	}
)

type (
	resAddOTP struct {
		URI    string `json:"uri"`
		Backup string `json:"backup"`
	}
)

func (s *Service) addOTP(ctx context.Context, userid string, alg string, digits int) (*resAddOTP, error) {
	m, err := s.users.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, dbsql.ErrNotFound) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "User not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get user")
	}
	if m.OTPEnabled {
		return nil, governor.ErrWithRes(nil, http.StatusBadRequest, "", "OTP already enabled")
	}

	cipher, err := s.getCipher(ctx)
	if err != nil {
		return nil, err
	}
	uri, backup, err := s.users.GenerateOTPSecret(ctx, cipher.cipher, m, s.authSettings.otpIssuer, alg, digits)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to generate otp secret")
	}
	return &resAddOTP{
		URI:    uri,
		Backup: backup,
	}, nil
}

func (s *Service) commitOTP(ctx context.Context, userid string, code string) error {
	m, err := s.users.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, dbsql.ErrNotFound) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "User not found")
		}
		return kerrors.WithMsg(err, "Failed to get user")
	}
	if m.OTPEnabled {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "OTP already enabled")
	}
	if m.OTPSecret == "" {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "OTP secret not yet added")
	}
	cipher, err := s.getCipher(ctx)
	if err != nil {
		return err
	}
	if ok, err := s.users.ValidateOTPCode(cipher.keyring, m, code); err != nil {
		return kerrors.WithMsg(err, "Failed to validate otp code")
	} else if !ok {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Incorrect otp code")
	}
	if err := s.users.EnableOTP(ctx, m); err != nil {
		return kerrors.WithMsg(err, "Failed to enable otp")
	}
	return nil
}

func (s *Service) removeOTP(ctx context.Context, userid string, code string, backup string, ipaddr, useragent string) error {
	m, err := s.users.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, dbsql.ErrNotFound) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "User not found")
		}
		return kerrors.WithMsg(err, "Failed to get user")
	}
	if !m.OTPEnabled {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "OTP already disabled")
	}
	if err := s.checkOTPCode(ctx, m, code, backup, ipaddr, useragent); err != nil {
		return err
	}
	if err := s.users.DisableOTP(ctx, m); err != nil {
		return kerrors.WithMsg(err, "Failed to disable otp")
	}
	return nil
}
