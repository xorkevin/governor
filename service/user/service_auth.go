package user

import (
	"net/http"
	"time"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/session/model"
	"xorkevin.dev/governor/service/user/token"
)

const (
	uidSize = 16
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
		SessionToken string        `json:"session_token,omitempty"`
		Claims       *token.Claims `json:"claims,omitempty"`
	}
)

// Login authenticates a user and returns auth tokens
func (s *service) Login(userid, password, sessionID, ipaddr, useragent string) (*resUserAuth, error) {
	m, err := s.users.GetByID(userid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return nil, governor.NewErrorUser("Invalid username or password", http.StatusUnauthorized, nil)
		}
		return nil, governor.NewError("Failed to get user", http.StatusInternalServerError, err)
	}
	if ok, err := s.users.ValidatePass(password, m); err != nil || !ok {
		return nil, governor.NewErrorUser("Invalid username or password", http.StatusUnauthorized, nil)
	}

	sessionExists := false
	var sm *sessionmodel.Model
	if len(sessionID) > 0 {
		if m, err := s.sessions.GetByID(sessionID); err != nil {
			if governor.ErrorStatus(err) == http.StatusNotFound {
			} else {
				return nil, governor.NewError("Failed to get user session", http.StatusInternalServerError, err)
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
			return nil, governor.NewError("Failed to create user session", http.StatusInternalServerError, err)
		}
		sm = m
		sessionKey = key
	} else {
		key, err := s.sessions.RehashKey(sm)
		if err != nil {
			return nil, governor.NewError("Failed to generate session key", http.StatusInternalServerError, err)
		}
		sm.AuthTime = sm.Time
		sessionKey = key
	}

	// generate an access token
	accessToken, accessClaims, err := s.tokenizer.Generate(m.Userid, s.accessTime, token.ScopeAll, "", "")
	if err != nil {
		return nil, governor.NewError("Failed to generate access token", http.StatusInternalServerError, err)
	}
	// generate a refresh token with the sessionKey
	refreshToken, _, err := s.tokenizer.Generate(m.Userid, s.refreshTime, token.ScopeAll, sm.SessionID, sessionKey)
	if err != nil {
		return nil, governor.NewError("Failed to generate refresh token", http.StatusInternalServerError, err)
	}

	if s.newLoginEmail && !sessionExists {
		emdata := emailNewLogin{
			FirstName: m.FirstName,
			LastName:  m.LastName,
			Username:  m.Username,
			SessionID: sm.SessionID,
			IP:        sm.IPAddr,
			Time:      time.Unix(sm.Time, 0).Format(time.RFC3339),
			UserAgent: sm.UserAgent,
		}
		if err := s.mailer.Send("", "", []string{m.Email}, newLoginTemplate, emdata); err != nil {
			s.logger.Error("fail send new login email", map[string]string{
				"error":      err.Error(),
				"actiontype": "newloginemail",
			})
		}
	}

	if !sessionExists {
		if err := s.sessions.Insert(sm); err != nil {
			return nil, governor.NewError("Failed to save user session", http.StatusInternalServerError, err)
		}
	} else {
		if err := s.sessions.Update(sm); err != nil {
			return nil, governor.NewError("Failed to save user session", http.StatusInternalServerError, err)
		}
	}

	if err := s.kvsessions.Set(sm.SessionID, sm.KeyHash, s.refreshCacheTime); err != nil {
		s.logger.Error("Failed to cache user session", map[string]string{
			"error":      err.Error(),
			"actiontype": "cachesession",
		})
	}

	return &resUserAuth{
		Valid:        true,
		Refresh:      true,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		SessionToken: sm.SessionID,
		Claims:       accessClaims,
	}, nil
}

// ValidRefreshToken validates a refresh token
func (s *service) ValidRefreshToken(refreshToken string) (string, error) {
	validToken, claims := s.tokenizer.Validate(refreshToken, "")
	if !validToken {
		return "", governor.NewErrorUser("Invalid token", http.StatusUnauthorized, nil)
	}

	keyhash, err := s.kvsessions.Get(claims.ID)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			sm, err := s.sessions.GetByID(claims.ID)
			if err != nil {
				if governor.ErrorStatus(err) == http.StatusNotFound {
					return "", governor.NewErrorUser("Invalid token", http.StatusUnauthorized, nil)
				}
				return "", governor.NewError("Failed to get session", http.StatusInternalServerError, err)
			}
			if ok, err := s.sessions.ValidateKey(claims.Key, sm); err != nil || !ok {
				return "", governor.NewErrorUser("Invalid token", http.StatusUnauthorized, nil)
			}
			return claims.Subject, nil
		}
		return "", governor.NewError("Failed to get session key", http.StatusInternalServerError, err)
	}
	if ok, err := s.sessions.ValidateKey(claims.Key, &sessionmodel.Model{
		KeyHash: keyhash,
	}); err != nil || !ok {
		return "", governor.NewErrorUser("Invalid token", http.StatusUnauthorized, nil)
	}
	return claims.Subject, nil
}

// ExchangeToken validates a refresh token and returns an auth token
func (s *service) ExchangeToken(refreshToken, ipaddr, useragent string) (*resUserAuth, error) {
	validToken, claims := s.tokenizer.Validate(refreshToken, "")
	if !validToken {
		return nil, governor.NewErrorUser("Invalid token", http.StatusUnauthorized, nil)
	}

	keyhash, err := s.kvsessions.Get(claims.ID)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return s.RefreshToken(refreshToken, ipaddr, useragent)
		}
		return nil, governor.NewError("Failed to get session key", http.StatusInternalServerError, err)
	}

	if ok, err := s.sessions.ValidateKey(claims.Key, &sessionmodel.Model{
		KeyHash: keyhash,
	}); err != nil || !ok {
		return nil, governor.NewErrorUser("Invalid token", http.StatusUnauthorized, nil)
	}

	accessToken, accessClaims, err := s.tokenizer.Generate(claims.Subject, s.accessTime, claims.Scope, "", "")
	if err != nil {
		return nil, governor.NewError("Failed to generate access token", http.StatusInternalServerError, err)
	}

	return &resUserAuth{
		Valid:        true,
		Refresh:      false,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		SessionToken: claims.ID,
		Claims:       accessClaims,
	}, nil
}

// RefreshToken invalidates the previous refresh token and creates a new one
func (s *service) RefreshToken(refreshToken, ipaddr, useragent string) (*resUserAuth, error) {
	validToken, claims := s.tokenizer.Validate(refreshToken, "")
	if !validToken {
		return nil, governor.NewErrorUser("Invalid token", http.StatusUnauthorized, nil)
	}

	sm, err := s.sessions.GetByID(claims.ID)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return nil, governor.NewErrorUser("Invalid token", http.StatusUnauthorized, nil)
		}
		return nil, governor.NewError("Failed to get session", http.StatusInternalServerError, err)
	}
	if ok, err := s.sessions.ValidateKey(claims.Key, sm); err != nil || !ok {
		return nil, governor.NewErrorUser("Invalid token", http.StatusUnauthorized, nil)
	}

	sessionKey, err := s.sessions.RehashKey(sm)
	if err != nil {
		return nil, governor.NewError("Failed to generate session key", http.StatusInternalServerError, err)
	}

	accessToken, accessClaims, err := s.tokenizer.Generate(claims.Subject, s.accessTime, claims.Scope, "", "")
	if err != nil {
		return nil, governor.NewError("Failed to generate access token", http.StatusInternalServerError, err)
	}
	newRefreshToken, _, err := s.tokenizer.Generate(claims.Subject, s.refreshTime, claims.Scope, sm.SessionID, sessionKey)
	if err != nil {
		return nil, governor.NewError("Failed to generate refresh token", http.StatusInternalServerError, err)
	}

	if err := s.sessions.Update(sm); err != nil {
		return nil, governor.NewError("Failed to save user session", http.StatusInternalServerError, err)
	}

	if err := s.kvsessions.Set(sm.SessionID, sm.KeyHash, s.refreshCacheTime); err != nil {
		s.logger.Error("Failed to cache user session", map[string]string{
			"error":      err.Error(),
			"actiontype": "cachesession",
		})
	}

	return &resUserAuth{
		Valid:        true,
		Refresh:      true,
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		SessionToken: sm.SessionID,
		Claims:       accessClaims,
	}, nil
}

// Logout removes the user session in cache
func (s *service) Logout(refreshToken string) error {
	// if session_id is provided, is in cache, and is valid, set it as the sessionID
	// the session can be expired by time
	ok, claims := s.tokenizer.GetClaims(refreshToken, "")
	if !ok {
		return governor.NewErrorUser("Invalid token", http.StatusUnauthorized, nil)
	}

	sm, err := s.sessions.GetByID(claims.ID)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return governor.NewErrorUser("Invalid token", http.StatusUnauthorized, nil)
		}
		return governor.NewError("Failed to get session", http.StatusInternalServerError, err)
	}
	if ok, err := s.sessions.ValidateKey(claims.Key, sm); err != nil || !ok {
		return governor.NewErrorUser("Invalid token", http.StatusUnauthorized, nil)
	}

	if err := s.sessions.Delete(sm); err != nil {
		return governor.NewError("Failed to delete session", http.StatusInternalServerError, err)
	}
	s.killCacheSessions([]string{claims.ID})

	return nil
}
