package user

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/dbsql"
	"xorkevin.dev/governor/service/gate"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/mail"
	"xorkevin.dev/governor/service/user/sessionmodel"
	"xorkevin.dev/governor/service/user/usermodel"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	emailNewLogin struct {
		FirstName string
		LastName  string
		Username  string
		SessionID string
		IP        string
		UserAgent string
		Time      string
	}
)

const (
	newLoginTemplate = "newlogin"
)

type (
	resUserAuth struct {
		Valid        bool         `json:"valid"`
		AccessToken  string       `json:"access_token,omitempty"`
		RefreshToken string       `json:"refresh_token,omitempty"`
		Claims       *gate.Claims `json:"claims,omitempty"`
	}
)

type (
	// errAuthenticate is returned when failing to authenticate
	errAuthenticate struct{}
	// errDiscardSession is returned when the login session should be discarded
	errDiscardSession struct{}
)

func (e errAuthenticate) Error() string {
	return "Failed authenticating"
}

func (e errDiscardSession) Error() string {
	return "Discard session"
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

func (s *Service) checkOTPCode(ctx context.Context, m *usermodel.Model, code string, backup string, ipaddr, useragent string) error {
	if code == "" {
		cipher, err := s.getCipher(ctx)
		if err != nil {
			return err
		}
		if ok, err := s.users.ValidateOTPBackup(cipher.keyring, m, backup); err != nil {
			return kerrors.WithMsg(err, "Failed to validate otp backup code")
		} else if !ok {
			return governor.ErrWithRes(kerrors.WithKind(nil, errAuthenticate{}, "Invalid otp backup code"), http.StatusUnauthorized, "", "Inalid otp backup code")
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
		ctx = klog.ExtendCtx(context.Background(), ctx)
		if err := s.mailer.SendTpl(ctx, "", mail.Addr{}, []mail.Addr{{Address: m.Email, Name: m.FirstName}}, mail.TplLocal(s.emailSettings.tplName.otpbackupused), emdata, false); err != nil {
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to send otp backup used email"))
		}
	} else {
		if _, err := s.kvotpcodes.Get(ctx, s.kvotpcodes.Subkey(m.Userid, code)); err != nil {
			if !errors.Is(err, kvstore.ErrNotFound) {
				s.log.Err(ctx, kerrors.WithMsg(err, "Failed to get user used otp code"))
			}
		} else {
			return governor.ErrWithRes(nil, http.StatusBadRequest, "", "OTP code already used")
		}
		cipher, err := s.getCipher(ctx)
		if err != nil {
			return err
		}
		if ok, err := s.users.ValidateOTPCode(cipher.keyring, m, code); err != nil {
			return kerrors.WithMsg(err, "Failed to validate otp code")
		} else if !ok {
			return governor.ErrWithRes(kerrors.WithKind(nil, errAuthenticate{}, "Invalid otp code"), http.StatusUnauthorized, "", "Invalid otp code")
		}
	}
	return nil
}

func (s *Service) markOTPCode(ctx context.Context, userid string, code string) {
	if err := s.kvotpcodes.Set(ctx, s.kvotpcodes.Subkey(userid, code), "-", 120); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to mark otp code as used"))
	}
}

func (s *Service) incrLoginFailCount(ctx context.Context, m *usermodel.Model, ipaddr, useragent string) {
	m.FailedLoginTime = time.Now().Round(0).Unix()
	if m.FailedLoginCount < 0 {
		m.FailedLoginCount = 0
	}
	m.FailedLoginCount += 1
	if err := s.users.UpdateLoginFailed(ctx, m); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to update login failure count"))
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
		if err := s.mailer.SendTpl(
			ctx,
			"",
			mail.Addr{},
			[]mail.Addr{{Address: m.Email, Name: m.FirstName}},
			mail.TplLocal(s.emailSettings.tplName.loginratelimit),
			emdata,
			false,
		); err != nil {
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to send otp ratelimit email"))
		}
	}
}

func (s *Service) resetLoginFailCount(ctx context.Context, m *usermodel.Model) {
	m.FailedLoginTime = 0
	m.FailedLoginCount = 0
	if err := s.users.UpdateLoginFailed(ctx, m); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to reset login failure count"))
	}
}

func (s *Service) login(ctx context.Context, userid, password, code, backup, sessionID, ipaddr, useragent string) (*resUserAuth, error) {
	m, err := s.users.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, dbsql.ErrNotFound) {
			return nil, governor.ErrWithRes(err, http.StatusUnauthorized, "", "Invalid username or password")
		}
		return nil, kerrors.WithMsg(err, "Failed to get user")
	}
	if err := s.checkLoginRatelimit(ctx, m); err != nil {
		return nil, err
	}

	if ok, err := s.users.ValidatePass(password, m); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to validate password")
	} else if !ok {
		// must make a best effort to increment login failures
		s.incrLoginFailCount(klog.ExtendCtx(context.Background(), ctx), m, ipaddr, useragent)
		return nil, governor.ErrWithRes(nil, http.StatusUnauthorized, "", "Invalid username or password")
	}

	if m.OTPEnabled {
		if code == "" && backup == "" {
			// must make a best effort to increment login failures
			s.incrLoginFailCount(klog.ExtendCtx(context.Background(), ctx), m, ipaddr, useragent)
			return nil, governor.ErrWithRes(nil, http.StatusBadRequest, "otp_required", "OTP code required")
		}

		if err := s.checkOTPCode(ctx, m, code, backup, ipaddr, useragent); err != nil {
			if errors.Is(err, errAuthenticate{}) {
				// must make a best effort to increment login failures
				s.incrLoginFailCount(klog.ExtendCtx(context.Background(), ctx), m, ipaddr, useragent)
			}
			return nil, err
		}
	}

	// must make a best effort to reset login failures
	s.resetLoginFailCount(klog.ExtendCtx(context.Background(), ctx), m)

	sessionExists := false
	var sm *sessionmodel.Model
	if sessionID != "" {
		if m, err := s.sessions.GetByID(ctx, userid, sessionID); err != nil {
			if !errors.Is(err, dbsql.ErrNotFound) {
				return nil, kerrors.WithMsg(err, "Failed to get user session")
			}
		} else {
			sm = m
			sessionExists = true
		}
	}

	var sessionKey string
	if sm == nil {
		m, key, err := s.sessions.New(userid, ipaddr, useragent)
		if err != nil {
			return nil, kerrors.WithMsg(err, "Failed to create user session")
		}
		sm = m
		sessionKey = key
	} else {
		key, err := s.sessions.RehashKey(sm)
		if err != nil {
			return nil, kerrors.WithMsg(err, "Failed to generate session key")
		}
		sm.AuthTime = sm.Time
		sm.IPAddr = ipaddr
		sm.UserAgent = useragent
		sessionKey = key
	}
	refreshToken := fmt.Sprintf("gr.%s.%s.%s", sm.Userid, sm.SessionID, sessionKey)

	// generate an access token
	accessToken, accessClaims, err := s.gate.Generate(
		ctx,
		gate.Claims{
			Subject:   sm.Userid,
			SessionID: sm.SessionID,
			AuthAt:    sm.AuthTime,
		},
		s.authSettings.accessDuration,
	)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to generate access token")
	}

	if s.authSettings.newLoginEmail && !sessionExists {
		emdata := emailNewLogin{
			FirstName: m.FirstName,
			LastName:  m.LastName,
			Username:  m.Username,
			SessionID: sm.SessionID,
			IP:        sm.IPAddr,
			Time:      time.Unix(sm.Time, 0).UTC().Format(time.RFC1123Z),
			UserAgent: sm.UserAgent,
		}
		if err := s.mailer.SendTpl(ctx, "", mail.Addr{}, []mail.Addr{{Address: m.Email, Name: m.FirstName}}, mail.TplLocal(newLoginTemplate), emdata, false); err != nil {
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to send new login email"))
		}
	}

	if !sessionExists {
		if err := s.sessions.Insert(ctx, sm); err != nil {
			return nil, kerrors.WithMsg(err, "Failed to save user session")
		}
	} else {
		if err := s.sessions.Update(ctx, sm); err != nil {
			return nil, kerrors.WithMsg(err, "Failed to save user session")
		}
	}

	if m.OTPEnabled {
		// must make a best effort to mark otp code as used
		s.markOTPCode(klog.ExtendCtx(context.Background(), ctx), userid, code)
	}

	return &resUserAuth{
		Valid:        true,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		Claims:       accessClaims,
	}, nil
}

func (s *Service) validateRefreshToken(ctx context.Context, refreshToken string) (*sessionmodel.Model, string, error) {
	tokenParts := strings.Split(refreshToken, ".")
	if len(tokenParts) != 4 || tokenParts[0] != "gr" || tokenParts[1] == "" || tokenParts[2] == "" || tokenParts[3] == "" {
		return nil, "", governor.ErrWithRes(nil, http.StatusUnauthorized, "", "Invalid token")
	}
	userid := tokenParts[1]
	sessionID := tokenParts[2]
	sessionKey := tokenParts[3]

	sm, err := s.sessions.GetByID(ctx, userid, sessionID)
	if err != nil {
		if errors.Is(err, dbsql.ErrNotFound) {
			return nil, userid, governor.ErrWithRes(kerrors.WithKind(err, errDiscardSession{}, "No session"), http.StatusUnauthorized, "", "Invalid token")
		}
		return nil, "", kerrors.WithMsg(err, "Failed to get session")
	}
	if ok, err := s.sessions.ValidateKey(sessionKey, sm); err != nil {
		return nil, "", kerrors.WithMsg(err, "Failed to validate key")
	} else if !ok {
		return nil, userid, governor.ErrWithRes(kerrors.WithKind(err, errDiscardSession{}, "Invalid session key"), http.StatusUnauthorized, "", "Invalid token")
	}
	return sm, "", nil
}

func (s *Service) refreshToken(ctx context.Context, refreshToken, ipaddr, useragent string) (*resUserAuth, error) {
	sm, erruserid, err := s.validateRefreshToken(ctx, refreshToken)
	if err != nil {
		return &resUserAuth{
			Valid: false,
			Claims: &gate.Claims{
				Subject: erruserid,
			},
		}, err
	}

	sessionKey, err := s.sessions.RehashKey(sm)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to generate session key")
	}
	sm.IPAddr = ipaddr
	sm.UserAgent = useragent
	refreshToken = fmt.Sprintf("gr.%s.%s.%s", sm.Userid, sm.SessionID, sessionKey)

	accessToken, accessClaims, err := s.gate.Generate(
		ctx,
		gate.Claims{
			Subject:   sm.Userid,
			SessionID: sm.SessionID,
			AuthAt:    sm.AuthTime,
		},
		s.authSettings.accessDuration,
	)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to generate access token")
	}

	if err := s.sessions.Update(ctx, sm); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to save user session")
	}

	return &resUserAuth{
		Valid:        true,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		Claims:       accessClaims,
	}, nil
}

func (s *Service) logout(ctx context.Context, refreshToken string) (string, error) {
	sm, erruserid, err := s.validateRefreshToken(ctx, refreshToken)
	if err != nil {
		return erruserid, err
	}
	if err := s.sessions.DeleteSession(ctx, sm.Userid, sm.SessionID); err != nil {
		return "", kerrors.WithMsg(err, "Failed to delete session")
	}
	return sm.Userid, nil
}
