package user

import (
	"bytes"
	"context"
	"errors"
	htmlTemplate "html/template"
	"net/http"
	"net/url"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/mail"
	"xorkevin.dev/governor/service/user/usermodel"
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
	b := &bytes.Buffer{}
	if err := tpl.Execute(b, e.query()); err != nil {
		return kerrors.WithMsg(err, "Failed executing email change url template")
	}
	e.URL = base + b.String()
	return nil
}

func (s *Service) updateEmail(ctx context.Context, userid string, newEmail string, password string) error {
	if _, err := s.users.GetByEmail(ctx, newEmail); err != nil {
		if !errors.Is(err, db.ErrorNotFound) {
			return kerrors.WithMsg(err, "Failed to get user")
		}
	} else {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Email is already in use")
	}
	m, err := s.users.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "User not found")
		}
		return kerrors.WithMsg(err, "Failed to get user")
	}
	if m.Email == newEmail {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Emails cannot be the same")
	}
	if ok, err := s.users.ValidatePass(password, m); err != nil {
		return kerrors.WithMsg(err, "Failed to validate password")
	} else if !ok {
		return governor.ErrWithRes(nil, http.StatusUnauthorized, "", "Incorrect password")
	}

	needInsert := false
	mr, err := s.resets.GetByID(ctx, m.Userid, kindResetEmail)
	if err != nil {
		if !errors.Is(err, db.ErrorNotFound) {
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
	if err := emdata.computeURL(s.emailurl.base, s.emailurl.emailchange); err != nil {
		return kerrors.WithMsg(err, "Failed to generate new email verification email")
	}
	if err := s.mailer.SendTpl(ctx, "", mail.Addr{}, []mail.Addr{{Address: newEmail, Name: m.FirstName}}, mail.TplLocal(s.tplname.emailchange), emdata, true); err != nil {
		return kerrors.WithMsg(err, "Failed to send new email verification email")
	}
	return nil
}

func (s *Service) commitEmail(ctx context.Context, userid string, key string, password string) error {
	mr, err := s.resets.GetByID(ctx, userid, kindResetEmail)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			return governor.ErrWithRes(err, http.StatusBadRequest, "", "New email verification expired")
		}
		return kerrors.WithMsg(err, "Failed to get email reset request")
	}

	if time.Now().Round(0).After(time.Unix(mr.CodeTime, 0).Add(s.authsettings.emailConfirmDuration)) {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "New email verification expired")
	}
	if ok, err := s.resets.ValidateCode(key, mr); err != nil {
		return kerrors.WithMsg(err, "Failed to validate email reset code")
	} else if !ok {
		return governor.ErrWithRes(nil, http.StatusUnauthorized, "", "Invalid code")
	}

	m, err := s.users.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "User not found")
		}
		return kerrors.WithMsg(err, "Failed to get user")
	}

	if ok, err := s.users.ValidatePass(password, m); err != nil {
		return kerrors.WithMsg(err, "Failed to validate password")
	} else if !ok {
		return governor.ErrWithRes(nil, http.StatusUnauthorized, "", "Incorrect password")
	}

	oldEmail := m.Email
	m.Email = mr.Params

	if err := s.resets.Delete(ctx, userid, kindResetEmail); err != nil {
		return kerrors.WithMsg(err, "Failed to delete email reset request")
	}

	if err = s.users.UpdateEmail(ctx, m); err != nil {
		if errors.Is(err, db.ErrorUnique) {
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
	ctx = klog.ExtendCtx(context.Background(), ctx, nil)
	if err := s.mailer.SendTpl(ctx, "", mail.Addr{}, []mail.Addr{{Address: oldEmail, Name: m.FirstName}}, mail.TplLocal(s.tplname.emailchangenotify), emdatanotify, false); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to send old email change notification"), nil)
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
	m, err := s.users.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound) {
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
	ctx = klog.ExtendCtx(context.Background(), ctx, nil)
	if err := s.mailer.SendTpl(ctx, "", mail.Addr{}, []mail.Addr{{Address: m.Email, Name: m.FirstName}}, mail.TplLocal(s.tplname.passchange), emdata, false); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to send password change notification email"), nil)
	}
	return nil
}

func (s *Service) forgotPassword(ctx context.Context, useroremail string) error {
	if !s.authsettings.passwordReset {
		return governor.ErrWithRes(nil, http.StatusConflict, "", "Password reset not enabled")
	}

	var m *usermodel.Model
	if isEmail(useroremail) {
		mu, err := s.users.GetByEmail(ctx, useroremail)
		if err != nil {
			if errors.Is(err, db.ErrorNotFound) {
				// prevent email scanning for unauthorized users
				return nil
			}
			return kerrors.WithMsg(err, "Failed to get user")
		}
		m = mu
	} else {
		mu, err := s.users.GetByUsername(ctx, useroremail)
		if err != nil {
			if errors.Is(err, db.ErrorNotFound) {
				return governor.ErrWithRes(err, http.StatusNotFound, "", "User not found")
			}
			return kerrors.WithMsg(err, "Failed to get user")
		}
		m = mu
	}

	needInsert := false
	mr, err := s.resets.GetByID(ctx, m.Userid, kindResetPass)
	if err != nil {
		if !errors.Is(err, db.ErrorNotFound) {
			return kerrors.WithMsg(err, "Failed to get user")
		}
		needInsert = true
		mr = s.resets.New(m.Userid, kindResetPass)
	} else {
		if time.Now().Round(0).Before(time.Unix(mr.CodeTime, 0).Add(s.authsettings.passResetDelay)) {
			s.log.Warn(ctx, "Forgot password called prior to delay end", klog.Fields{
				"userid": m.Userid,
			})
			return nil
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
	if err := emdata.computeURL(s.emailurl.base, s.emailurl.forgotpass); err != nil {
		return kerrors.WithMsg(err, "Failed to generate password reset email")
	}
	if err := s.mailer.SendTpl(ctx, "", mail.Addr{}, []mail.Addr{{Address: m.Email, Name: m.FirstName}}, mail.TplLocal(s.tplname.forgotpass), emdata, true); err != nil {
		return kerrors.WithMsg(err, "Failed to send password reset email")
	}
	return nil
}

func (s *Service) resetPassword(ctx context.Context, userid string, key string, newPassword string) error {
	mr, err := s.resets.GetByID(ctx, userid, kindResetPass)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "Password reset expired")
		}
		return kerrors.WithMsg(err, "Failed to get password reset request")
	}

	if time.Now().Round(0).After(time.Unix(mr.CodeTime, 0).Add(s.authsettings.passwordResetDuration)) {
		return governor.ErrWithRes(nil, http.StatusNotFound, "", "Password reset expired")
	}
	if ok, err := s.resets.ValidateCode(key, mr); err != nil {
		return kerrors.WithMsg(err, "Failed to validate password reset code")
	} else if !ok {
		return governor.ErrWithRes(nil, http.StatusUnauthorized, "", "Invalid code")
	}

	m, err := s.users.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound) {
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
	ctx = klog.ExtendCtx(context.Background(), ctx, nil)
	if err := s.mailer.SendTpl(ctx, "", mail.Addr{}, []mail.Addr{{Address: m.Email, Name: m.FirstName}}, mail.TplLocal(s.tplname.passreset), emdata, false); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to send password change notification email"), nil)
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

func (s *Service) addOTP(ctx context.Context, userid string, alg string, digits int, password string) (*resAddOTP, error) {
	m, err := s.users.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "User not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get user")
	}
	if m.OTPEnabled {
		return nil, governor.ErrWithRes(nil, http.StatusBadRequest, "", "OTP already enabled")
	}
	if ok, err := s.users.ValidatePass(password, m); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to validate password")
	} else if !ok {
		return nil, governor.ErrWithRes(nil, http.StatusUnauthorized, "", "Incorrect password")
	}

	cipher, err := s.getCipher(ctx)
	if err != nil {
		return nil, err
	}
	uri, backup, err := s.users.GenerateOTPSecret(ctx, cipher.cipher, m, s.otpIssuer, alg, digits)
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
		if errors.Is(err, db.ErrorNotFound) {
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
	if ok, err := s.users.ValidateOTPCode(cipher.decrypter, m, code); err != nil {
		return kerrors.WithMsg(err, "Failed to validate otp code")
	} else if !ok {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Incorrect otp code")
	}
	if err := s.users.EnableOTP(ctx, m); err != nil {
		return kerrors.WithMsg(err, "Failed to enable otp")
	}
	return nil
}

func (s *Service) checkLoginRatelimit(ctx context.Context, m *usermodel.Model) error {
	var k time.Duration
	if m.FailedLoginCount > 293 || m.FailedLoginCount < 0 {
		k = 24 * time.Hour
	} else {
		k = time.Duration(m.FailedLoginCount*m.FailedLoginCount) * time.Second
	}
	cliff := time.Unix(m.FailedLoginTime, 0).Add(k).UTC()
	if time.Now().Round(0).Before(cliff) {
		return governor.ErrWithTooManyRequests(nil, cliff, "", "Failed login too many times")
	}
	return nil
}

type (
	// errorAuthenticate is returned when failing to authenticate
	errorAuthenticate struct{}
)

func (e errorAuthenticate) Error() string {
	return "Failed authenticating"
}

func (s *Service) checkOTPCode(ctx context.Context, m *usermodel.Model, code string, backup string, ipaddr, useragent string) error {
	if code == "" {
		cipher, err := s.getCipher(ctx)
		if err != nil {
			return err
		}
		if ok, err := s.users.ValidateOTPBackup(cipher.decrypter, m, backup); err != nil {
			return kerrors.WithMsg(err, "Failed to validate otp backup code")
		} else if !ok {
			return governor.ErrWithRes(kerrors.WithKind(nil, errorAuthenticate{}, "Failed to authenticate"), http.StatusUnauthorized, "", "Inalid otp backup code")
		}

		emdata := emailOTPBackupUsed{
			FirstName: m.FirstName,
			LastName:  m.LastName,
			Username:  m.Username,
			IP:        ipaddr,
			Time:      time.Now().Round(0).UTC().Format(time.RFC1123Z),
			UserAgent: useragent,
		}
		// must make best effort attempt to send the email
		ctx = klog.ExtendCtx(context.Background(), ctx, nil)
		if err := s.mailer.SendTpl(ctx, "", mail.Addr{}, []mail.Addr{{Address: m.Email, Name: m.FirstName}}, mail.TplLocal(s.tplname.otpbackupused), emdata, false); err != nil {
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to send otp backup used email"), nil)
		}
	} else {
		if _, err := s.kvotpcodes.Get(ctx, s.kvotpcodes.Subkey(m.Userid, code)); err != nil {
			if !errors.Is(err, kvstore.ErrorNotFound) {
				s.log.Err(ctx, kerrors.WithMsg(err, "Failed to get user used otp code"), nil)
			}
		} else {
			return governor.ErrWithRes(nil, http.StatusBadRequest, "", "OTP code already used")
		}
		cipher, err := s.getCipher(ctx)
		if err != nil {
			return err
		}
		if ok, err := s.users.ValidateOTPCode(cipher.decrypter, m, code); err != nil {
			return kerrors.WithMsg(err, "Failed to validate otp code")
		} else if !ok {
			return governor.ErrWithRes(kerrors.WithKind(nil, errorAuthenticate{}, "Failed to authenticate"), http.StatusUnauthorized, "", "Invalid otp code")
		}
	}
	return nil
}

func (s *Service) markOTPCode(ctx context.Context, userid string, code string) {
	if err := s.kvotpcodes.Set(ctx, s.kvotpcodes.Subkey(userid, code), "-", 120); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to mark otp code as used"), nil)
	}
}

func (s *Service) incrLoginFailCount(ctx context.Context, m *usermodel.Model, ipaddr, useragent string) {
	m.FailedLoginTime = time.Now().Round(0).Unix()
	if m.FailedLoginCount < 0 {
		m.FailedLoginCount = 0
	}
	m.FailedLoginCount += 1
	if err := s.users.UpdateLoginFailed(ctx, m); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to update login failure count"), nil)
	}

	if m.FailedLoginCount%8 == 0 {
		emdata := emailLoginRatelimit{
			FirstName: m.FirstName,
			LastName:  m.LastName,
			Username:  m.Username,
			IP:        ipaddr,
			Time:      time.Unix(m.FailedLoginTime, 0).UTC().Format(time.RFC1123Z),
			UserAgent: useragent,
		}
		if err := s.mailer.SendTpl(ctx, "", mail.Addr{}, []mail.Addr{{Address: m.Email, Name: m.FirstName}}, mail.TplLocal(s.tplname.loginratelimit), emdata, false); err != nil {
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to send otp ratelimit email"), nil)
		}
	}
}

func (s *Service) resetLoginFailCount(ctx context.Context, m *usermodel.Model) {
	m.FailedLoginTime = 0
	m.FailedLoginCount = 0
	if err := s.users.UpdateLoginFailed(ctx, m); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to reset login failure count"), nil)
	}
}

func (s *Service) removeOTP(ctx context.Context, userid string, code string, backup string, password string, ipaddr, useragent string) error {
	m, err := s.users.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "User not found")
		}
		return kerrors.WithMsg(err, "Failed to get user")
	}
	if !m.OTPEnabled {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "OTP already disabled")
	}
	if ok, err := s.users.ValidatePass(password, m); err != nil {
		return kerrors.WithMsg(err, "Failed to validate password")
	} else if !ok {
		return governor.ErrWithRes(nil, http.StatusUnauthorized, "", "Invalid password")
	}
	if err := s.checkOTPCode(ctx, m, code, backup, ipaddr, useragent); err != nil {
		return err
	}
	if err := s.users.DisableOTP(ctx, m); err != nil {
		return kerrors.WithMsg(err, "Failed to disable otp")
	}
	return nil
}
