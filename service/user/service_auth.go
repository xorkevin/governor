package user

import (
	"github.com/go-redis/redis"
	"net/http"
	"time"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/session/model"
	"xorkevin.dev/governor/service/user/token"
)

const (
	uidSize = 16
)

const (
	authenticationSubject = "authentication"
	refreshSubject        = "refresh"
	cachePrefixSession    = moduleID + ".session:"
)

type (
	emailNewLogin struct {
		FirstName string
		Username  string
		SessionID string
		IP        string
		UserAgent string
		Time      string
	}
)

const (
	newLoginTemplate = "newlogin"
	newLoginSubject  = "newlogin_subject"
)

type (
	resUserAuth struct {
		Valid        bool          `json:"valid"`
		AccessToken  string        `json:"access_token,omitempty"`
		RefreshToken string        `json:"refresh_token,omitempty"`
		SessionToken string        `json:"session_token,omitempty"`
		Claims       *token.Claims `json:"claims,omitempty"`
	}
)

// Login authenticates a user and returns auth tokens
func (u *userService) Login(userid, password, sessionID, ipaddr, useragent string) (*resUserAuth, error) {
	m, err := u.repo.GetByID(userid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return nil, governor.NewErrorUser("Invalid password", http.StatusUnauthorized, nil)
		}
		return nil, governor.NewError("Failed to get user", http.StatusInternalServerError, err)
	}
	if ok, err := u.repo.ValidatePass(password, m); err != nil {
		return nil, governor.NewError("Failed to validate password", http.StatusInternalServerError, err)
	} else if !ok {
		return nil, governor.NewErrorUser("Invalid password", http.StatusUnauthorized, nil)
	}

	sessionExists := false
	var sm *sessionmodel.Model
	if len(sessionID) > 0 {
		if m, err := u.sessionrepo.GetByID(sessionID); err != nil {
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
		m, key, err := u.sessionrepo.New(userid, ipaddr, useragent)
		if err != nil {
			return nil, governor.NewError("Failed to create user session", http.StatusInternalServerError, err)
		}
		sm = m
		sessionKey = key
	} else {
		key, err := u.sessionrepo.RehashKey(sm)
		if err != nil {
			return nil, governor.NewError("Failed to generate session key", http.StatusInternalServerError, err)
		}
		sessionKey = key
	}

	// generate an access token
	accessToken, claims, err := u.tokenizer.Generate(m, u.accessTime, authenticationSubject, "", "")
	if err != nil {
		return nil, governor.NewError("Failed to generate access token", http.StatusInternalServerError, err)
	}
	// generate a refresh token with the sessionKey
	refreshToken, _, err := u.tokenizer.Generate(m, u.refreshTime, refreshSubject, sm.SessionID, sessionKey)
	if err != nil {
		return nil, governor.NewError("Failed to generate refresh token", http.StatusInternalServerError, err)
	}

	if u.newLoginEmail && !sessionExists {
		emdata := emailNewLogin{
			FirstName: m.FirstName,
			Username:  m.Username,
			SessionID: sm.SessionID,
			IP:        sm.IPAddr,
			Time:      time.Unix(sm.Time, 0).String(),
			UserAgent: sm.UserAgent,
		}
		if err := u.mailer.Send(m.Email, newLoginSubject, newLoginTemplate, emdata); err != nil {
			u.logger.Error("fail send new login email", map[string]string{
				"err": err.Error(),
			})
		}
	}

	if err := u.cache.Cache().Set(cachePrefixSession+sm.SessionID, sm.KeyHash, time.Duration(u.refreshTime*b1)).Err(); err != nil {
		return nil, governor.NewError("Failed to save user session", http.StatusInternalServerError, err)
	}
	if !sessionExists {
		if err := u.sessionrepo.Insert(sm); err != nil {
			return nil, governor.NewError("Failed to save user session", http.StatusInternalServerError, err)
		}
	} else {
		if err := u.sessionrepo.Update(sm); err != nil {
			return nil, governor.NewError("Failed to save user session", http.StatusInternalServerError, err)
		}
	}

	return &resUserAuth{
		Valid:        true,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		SessionToken: sm.SessionID,
		Claims:       claims,
	}, nil
}

// ExchangeToken validates a refresh token and returns an auth token
func (u *userService) ExchangeToken(refreshToken, ipaddr, useragent string) (*resUserAuth, error) {
	validToken, claims := u.tokenizer.Validate(refreshToken, refreshSubject)
	if !validToken {
		return nil, governor.NewErrorUser("Invalid token", http.StatusUnauthorized, nil)
	}

	keyhash, err := u.cache.Cache().Get(cachePrefixSession + claims.ID).Result()
	if err != nil {
		if err == redis.Nil {
			return u.RefreshToken(refreshToken, ipaddr, useragent)
		}
		return nil, governor.NewError("Failed to get session key", http.StatusInternalServerError, err)
	}

	k := sessionmodel.Model{
		KeyHash: keyhash,
	}
	if ok, err := u.sessionrepo.ValidateKey(claims.Key, &k); err != nil || !ok {
		return nil, governor.NewErrorUser("Invalid token", http.StatusUnauthorized, nil)
	}

	accessToken, newClaims, err := u.tokenizer.GenerateFromClaims(claims, u.accessTime, authenticationSubject, "")
	if err != nil {
		return nil, governor.NewError("Failed to generate access token", http.StatusInternalServerError, err)
	}

	return &resUserAuth{
		Valid:       true,
		AccessToken: accessToken,
		Claims:      newClaims,
	}, nil
}

// RefreshToken invalidates the previous refresh token and creates a new one
func (u *userService) RefreshToken(refreshToken, ipaddr, useragent string) (*resUserAuth, error) {
	validToken, claims := u.tokenizer.Validate(refreshToken, refreshSubject)
	if !validToken {
		return nil, governor.NewErrorUser("Invalid token", http.StatusUnauthorized, nil)
	}

	sm, err := u.sessionrepo.GetByID(claims.ID)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return nil, governor.NewErrorUser("Invalid token", http.StatusUnauthorized, nil)
		}
		return nil, governor.NewError("Failed to get session", http.StatusInternalServerError, err)
	}
	if ok, err := u.sessionrepo.ValidateKey(claims.Key, sm); err != nil || !ok {
		return nil, governor.NewErrorUser("Invalid token", http.StatusUnauthorized, nil)
	}
	m, err := u.repo.GetByID(claims.Userid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return nil, governor.NewErrorUser("Invalid token", http.StatusUnauthorized, nil)
		}
		return nil, governor.NewError("Failed to get user", http.StatusInternalServerError, err)
	}

	sessionKey, err := u.sessionrepo.RehashKey(sm)
	if err != nil {
		return nil, governor.NewError("Failed to generate session key", http.StatusInternalServerError, err)
	}

	accessToken, newClaims, err := u.tokenizer.Generate(m, u.accessTime, authenticationSubject, "", "")
	if err != nil {
		return nil, governor.NewError("Failed to generate access token", http.StatusInternalServerError, err)
	}
	newRefreshToken, _, err := u.tokenizer.Generate(m, u.refreshTime, refreshSubject, sm.SessionID, sessionKey)
	if err != nil {
		return nil, governor.NewError("Failed to generate refresh token", http.StatusInternalServerError, err)
	}

	if err := u.cache.Cache().Set(cachePrefixSession+sm.SessionID, sm.KeyHash, time.Duration(u.refreshTime*b1)).Err(); err != nil {
		return nil, governor.NewError("Failed to save user session", http.StatusInternalServerError, err)
	}
	if err := u.sessionrepo.Update(sm); err != nil {
		return nil, governor.NewError("Failed to save user session", http.StatusInternalServerError, err)
	}

	return &resUserAuth{
		Valid:        true,
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		SessionToken: sm.SessionID,
		Claims:       newClaims,
	}, nil
}

// Logout removes the user session in cache
func (u *userService) Logout(refreshToken string) error {
	// if session_id is provided, is in cache, and is valid, set it as the sessionID
	// the session can be expired by time
	okToken, claims := u.tokenizer.GetClaims(refreshToken, refreshSubject)
	if !okToken {
		return governor.NewErrorUser("Invalid token", http.StatusUnauthorized, nil)
	}

	sm, err := u.sessionrepo.GetByID(claims.ID)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return governor.NewErrorUser("Invalid token", http.StatusUnauthorized, nil)
		}
		return governor.NewError("Failed to get session", http.StatusInternalServerError, err)
	}
	if ok, err := u.sessionrepo.ValidateKey(claims.Key, sm); err != nil || !ok {
		return governor.NewErrorUser("Invalid token", http.StatusUnauthorized, nil)
	}

	if err := u.cache.Cache().Del(cachePrefixSession + claims.ID).Err(); err != nil {
		return governor.NewError("Failed to delete session", http.StatusInternalServerError, err)
	}
	if err := u.sessionrepo.Delete(sm); err != nil {
		return governor.NewError("Failed to delete session", http.StatusInternalServerError, err)
	}

	return nil
}
