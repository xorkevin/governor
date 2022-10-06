package user

import (
	"context"
	"errors"
	"net/http"
	"time"

	"gopkg.in/square/go-jose.v2/jwt"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/mail"
	sessionmodel "xorkevin.dev/governor/service/user/session/model"
	"xorkevin.dev/governor/service/user/token"
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
		Valid        bool          `json:"valid"`
		Refresh      bool          `json:"refresh"`
		AccessToken  string        `json:"access_token,omitempty"`
		RefreshToken string        `json:"refresh_token,omitempty"`
		SessionID    string        `json:"session_token,omitempty"`
		Claims       *token.Claims `json:"claims,omitempty"`
	}
)

type (
	// ErrorDiscardSession is returned when the login session should be discarded
	ErrorDiscardSession struct{}
)

func (e ErrorDiscardSession) Error() string {
	return "Discard session"
}

func (s *Service) login(ctx context.Context, userid, password, code, backup, sessionID, ipaddr, useragent string) (*resUserAuth, error) {
	m, err := s.users.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound{}) {
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
		s.incrLoginFailCount(klog.ExtendCtx(context.Background(), ctx, nil), m, ipaddr, useragent)
		return nil, governor.ErrWithRes(nil, http.StatusUnauthorized, "", "Invalid username or password")
	}

	if m.OTPEnabled {
		if len(code) == 0 && len(backup) == 0 {
			return nil, governor.ErrWithRes(nil, http.StatusBadRequest, "otp_required", "OTP code required")
		}

		if err := s.checkOTPCode(ctx, m, code, backup, ipaddr, useragent); err != nil {
			if errors.Is(err, ErrorAuthenticate{}) {
				// must make a best effort to increment login failures
				s.incrLoginFailCount(klog.ExtendCtx(context.Background(), ctx, nil), m, ipaddr, useragent)
			}
			return nil, err
		}
	}

	// must make a best effort to reset login failures
	s.resetLoginFailCount(klog.ExtendCtx(context.Background(), ctx, nil), m)

	sessionExists := false
	var sm *sessionmodel.Model
	if len(sessionID) > 0 {
		if m, err := s.sessions.GetByID(ctx, sessionID); err != nil {
			if !errors.Is(err, db.ErrorNotFound{}) {
				return nil, kerrors.WithMsg(err, "Failed to get user session")
			}
		} else {
			sm = m
			sessionExists = true
		}
	}

	sessionKey := ""
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
		sessionKey = key
	}

	// generate an access token
	accessToken, accessClaims, err := s.tokenizer.Generate(
		ctx,
		token.KindAccess,
		m.Userid,
		s.authsettings.accessDuration,
		sm.SessionID,
		sm.AuthTime,
		token.ScopeAll,
		"",
	)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to generate access token")
	}
	// generate a refresh token with the sessionKey
	refreshToken, _, err := s.tokenizer.Generate(
		ctx,
		token.KindRefresh,
		m.Userid,
		s.authsettings.refreshDuration,
		sm.SessionID,
		sm.AuthTime,
		token.ScopeAll,
		sessionKey,
	)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to generate refresh token")
	}

	if s.authsettings.newLoginEmail && !sessionExists {
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
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to send new login email"), nil)
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

	if err := s.kvsessions.Set(ctx, sm.SessionID, sm.KeyHash, s.authsettings.refreshCache); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to cache user session"), nil)
	}

	// must make a best effort to mark otp code as used
	s.markOTPCode(klog.ExtendCtx(context.Background(), ctx, nil), userid, code)

	return &resUserAuth{
		Valid:        true,
		Refresh:      true,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		SessionID:    sm.SessionID,
		Claims:       accessClaims,
	}, nil
}

func (s *Service) exchangeToken(ctx context.Context, refreshToken, ipaddr, useragent string) (*resUserAuth, error) {
	validToken, claims := s.tokenizer.Validate(ctx, token.KindRefresh, refreshToken)
	if !validToken {
		return nil, governor.ErrWithRes(nil, http.StatusUnauthorized, "", "Invalid token")
	}

	if ok, err := s.CheckUserExists(ctx, claims.Subject); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get user")
	} else if !ok {
		return nil, governor.ErrWithRes(nil, http.StatusNotFound, "", "Invalid user")
	}

	keyhash, err := s.kvsessions.Get(ctx, claims.ID)
	if err != nil {
		if !errors.Is(err, kvstore.ErrorNotFound{}) {
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to get cached session"), nil)
		}
		return s.refreshToken(ctx, refreshToken, ipaddr, useragent)
	}

	if ok, err := s.sessions.ValidateKey(claims.Key, &sessionmodel.Model{
		KeyHash: keyhash,
	}); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to validate key")
	} else if !ok {
		return nil, governor.ErrWithRes(nil, http.StatusUnauthorized, "", "Invalid token")
	}

	accessToken, accessClaims, err := s.tokenizer.Generate(
		ctx,
		token.KindAccess,
		claims.Subject,
		s.authsettings.accessDuration,
		claims.ID,
		claims.AuthTime,
		claims.Scope,
		"",
	)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to generate access token")
	}

	return &resUserAuth{
		Valid:        true,
		Refresh:      false,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		SessionID:    claims.ID,
		Claims:       accessClaims,
	}, nil
}

func (s *Service) refreshToken(ctx context.Context, refreshToken, ipaddr, useragent string) (*resUserAuth, error) {
	validToken, claims := s.tokenizer.Validate(ctx, token.KindRefresh, refreshToken)
	if !validToken {
		return nil, governor.ErrWithRes(nil, http.StatusUnauthorized, "", "Invalid token")
	}

	sm, err := s.sessions.GetByID(ctx, claims.ID)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound{}) {
			return &resUserAuth{
				Valid: false,
				Claims: &token.Claims{
					Claims: jwt.Claims{
						Subject: claims.Subject,
					},
				},
			}, governor.ErrWithRes(kerrors.WithKind(err, ErrorDiscardSession{}, "No session"), http.StatusUnauthorized, "", "Invalid token")
		}
		return nil, kerrors.WithMsg(err, "Failed to get session")
	}
	if ok, err := s.sessions.ValidateKey(claims.Key, sm); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to validate key")
	} else if !ok {
		return &resUserAuth{
			Valid: false,
			Claims: &token.Claims{
				Claims: jwt.Claims{
					Subject: claims.Subject,
				},
			},
		}, governor.ErrWithRes(kerrors.WithKind(err, ErrorDiscardSession{}, "Invalid session key"), http.StatusUnauthorized, "", "Invalid token")
	}

	sessionKey, err := s.sessions.RehashKey(sm)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to generate session key")
	}

	accessToken, accessClaims, err := s.tokenizer.Generate(
		ctx,
		token.KindAccess,
		claims.Subject,
		s.authsettings.accessDuration,
		sm.SessionID,
		sm.AuthTime,
		claims.Scope,
		"",
	)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to generate access token")
	}
	newRefreshToken, _, err := s.tokenizer.Generate(
		ctx,
		token.KindRefresh,
		claims.Subject,
		s.authsettings.refreshDuration,
		sm.SessionID,
		sm.AuthTime,
		claims.Scope,
		sessionKey,
	)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to generate refresh token")
	}

	sm.IPAddr = ipaddr
	sm.UserAgent = useragent
	if err := s.sessions.Update(ctx, sm); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to save user session")
	}

	if err := s.kvsessions.Set(ctx, sm.SessionID, sm.KeyHash, s.authsettings.refreshCache); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to cache user session"), nil)
	}

	return &resUserAuth{
		Valid:        true,
		Refresh:      true,
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		SessionID:    sm.SessionID,
		Claims:       accessClaims,
	}, nil
}

func (s *Service) logout(ctx context.Context, refreshToken string) (string, error) {
	// if session_id is provided, is in cache, and is valid, set it as the sessionID
	// the session can be expired by time
	ok, claims := s.tokenizer.GetClaims(ctx, token.KindRefresh, refreshToken)
	if !ok {
		return "", governor.ErrWithRes(nil, http.StatusUnauthorized, "", "Invalid token")
	}

	sm, err := s.sessions.GetByID(ctx, claims.ID)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound{}) {
			return "", governor.ErrWithRes(err, http.StatusUnauthorized, "", "Invalid token")
		}
		return "", kerrors.WithMsg(err, "Failed to get session")
	}
	if ok, err := s.sessions.ValidateKey(claims.Key, sm); err != nil {
		return "", kerrors.WithMsg(err, "Failed to validate key")
	} else if !ok {
		return "", governor.ErrWithRes(nil, http.StatusUnauthorized, "", "Invalid token")
	}

	if err := s.sessions.Delete(ctx, sm); err != nil {
		return "", kerrors.WithMsg(err, "Failed to delete session")
	}
	// must make a best effort to remove cached sessions
	ctx = klog.ExtendCtx(context.Background(), ctx, nil)
	s.killCacheSessions(ctx, []string{claims.ID})

	return claims.Subject, nil
}
