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
	// ErrDiscardSession is returned when the login session should be discarded
	ErrDiscardSession struct{}
)

func (e ErrDiscardSession) Error() string {
	return "Discard session"
}

// Login authenticates a user and returns auth tokens
func (s *service) Login(ctx context.Context, userid, password, code, backup, sessionID, ipaddr, useragent string) (*resUserAuth, error) {
	m, err := s.users.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
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
		s.incrLoginFailCount(m, ipaddr, useragent)
		return nil, governor.ErrWithRes(nil, http.StatusUnauthorized, "", "Invalid username or password")
	}

	if m.OTPEnabled {
		if len(code) == 0 && len(backup) == 0 {
			return nil, governor.ErrWithRes(nil, http.StatusBadRequest, "otp_required", "OTP code required")
		}

		if err := s.checkOTPCode(ctx, m, code, backup, ipaddr, useragent); err != nil {
			if errors.Is(err, ErrAuthenticate{}) {
				s.incrLoginFailCount(m, ipaddr, useragent)
			}
			return nil, err
		}
	}

	s.resetLoginFailCount(m)

	sessionExists := false
	var sm *sessionmodel.Model
	if len(sessionID) > 0 {
		if m, err := s.sessions.GetByID(ctx, sessionID); err != nil {
			if !errors.Is(err, db.ErrNotFound{}) {
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
	accessToken, accessClaims, err := s.tokenizer.Generate(ctx, token.KindAccess, m.Userid, s.accessTime, sm.SessionID, sm.AuthTime, token.ScopeAll, "")
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to generate access token")
	}
	// generate a refresh token with the sessionKey
	refreshToken, _, err := s.tokenizer.Generate(ctx, token.KindRefresh, m.Userid, s.refreshTime, sm.SessionID, sm.AuthTime, token.ScopeAll, sessionKey)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to generate refresh token")
	}

	if s.newLoginEmail && !sessionExists {
		emdata := emailNewLogin{
			FirstName: m.FirstName,
			LastName:  m.LastName,
			Username:  m.Username,
			SessionID: sm.SessionID,
			IP:        sm.IPAddr,
			Time:      time.Unix(sm.Time, 0).UTC().Format(time.RFC3339),
			UserAgent: sm.UserAgent,
		}
		if err := s.mailer.SendTpl(ctx, "", mail.Addr{}, []mail.Addr{{Address: m.Email, Name: m.FirstName}}, mail.TplLocal(newLoginTemplate), emdata, false); err != nil {
			s.logger.Error("Failed to send new login email", map[string]string{
				"error":      err.Error(),
				"actiontype": "user_send_new_login_email",
			})
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

	if err := s.kvsessions.Set(ctx, sm.SessionID, sm.KeyHash, s.refreshCacheTime); err != nil {
		s.logger.Error("Failed to cache user session", map[string]string{
			"error":      err.Error(),
			"actiontype": "user_set_cache_session",
		})
	}

	s.markOTPCode(userid, code)

	return &resUserAuth{
		Valid:        true,
		Refresh:      true,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		SessionID:    sm.SessionID,
		Claims:       accessClaims,
	}, nil
}

// ExchangeToken validates a refresh token and returns an auth token
func (s *service) ExchangeToken(ctx context.Context, refreshToken, ipaddr, useragent string) (*resUserAuth, error) {
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
		if !errors.Is(err, kvstore.ErrNotFound{}) {
			s.logger.Error("Failed to get cached session", map[string]string{
				"error":      err.Error(),
				"actiontype": "user_get_cache_session",
			})
		}
		return s.RefreshToken(ctx, refreshToken, ipaddr, useragent)
	}

	if ok, err := s.sessions.ValidateKey(claims.Key, &sessionmodel.Model{
		KeyHash: keyhash,
	}); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to validate key")
	} else if !ok {
		return nil, governor.ErrWithRes(nil, http.StatusUnauthorized, "", "Invalid token")
	}

	accessToken, accessClaims, err := s.tokenizer.Generate(ctx, token.KindAccess, claims.Subject, s.accessTime, claims.ID, claims.AuthTime, claims.Scope, "")
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

// RefreshToken invalidates the previous refresh token and creates a new one
func (s *service) RefreshToken(ctx context.Context, refreshToken, ipaddr, useragent string) (*resUserAuth, error) {
	validToken, claims := s.tokenizer.Validate(ctx, token.KindRefresh, refreshToken)
	if !validToken {
		return nil, governor.ErrWithRes(nil, http.StatusUnauthorized, "", "Invalid token")
	}

	sm, err := s.sessions.GetByID(ctx, claims.ID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return &resUserAuth{
				Valid: false,
				Claims: &token.Claims{
					Claims: jwt.Claims{
						Subject: claims.Subject,
					},
				},
			}, governor.ErrWithRes(kerrors.WithKind(err, ErrDiscardSession{}, "No session"), http.StatusUnauthorized, "", "Invalid token")
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
		}, governor.ErrWithRes(kerrors.WithKind(err, ErrDiscardSession{}, "Invalid session key"), http.StatusUnauthorized, "", "Invalid token")
	}

	sessionKey, err := s.sessions.RehashKey(sm)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to generate session key")
	}

	accessToken, accessClaims, err := s.tokenizer.Generate(ctx, token.KindAccess, claims.Subject, s.accessTime, sm.SessionID, sm.AuthTime, claims.Scope, "")
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to generate access token")
	}
	newRefreshToken, _, err := s.tokenizer.Generate(ctx, token.KindRefresh, claims.Subject, s.refreshTime, sm.SessionID, sm.AuthTime, claims.Scope, sessionKey)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to generate refresh token")
	}

	sm.IPAddr = ipaddr
	sm.UserAgent = useragent
	if err := s.sessions.Update(ctx, sm); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to save user session")
	}

	if err := s.kvsessions.Set(ctx, sm.SessionID, sm.KeyHash, s.refreshCacheTime); err != nil {
		s.logger.Error("Failed to cache user session", map[string]string{
			"error":      err.Error(),
			"actiontype": "user_set_cache_session",
		})
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

// Logout removes the user session in cache
func (s *service) Logout(ctx context.Context, refreshToken string) (string, error) {
	// if session_id is provided, is in cache, and is valid, set it as the sessionID
	// the session can be expired by time
	ok, claims := s.tokenizer.GetClaims(ctx, token.KindRefresh, refreshToken)
	if !ok {
		return "", governor.ErrWithRes(nil, http.StatusUnauthorized, "", "Invalid token")
	}

	sm, err := s.sessions.GetByID(ctx, claims.ID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
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
	s.killCacheSessions([]string{claims.ID})

	return claims.Subject, nil
}
