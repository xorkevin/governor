package user

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/model"
	"github.com/hackform/governor/service/user/session"
	"github.com/hackform/governor/service/user/token"
	"net/http"
	"strings"
	"time"
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
func (u *userService) Login(userid, password, sessionToken, ipAddress, userAgent string) (bool, *resUserAuth, *governor.Error) {
	m, err := usermodel.GetByIDB64(u.db.DB(), userid)
	if err != nil {
		err.AddTrace(moduleIDAuth)
		return false, nil, err
	}
	if !m.ValidatePass(password) {
		return false, &resUserAuth{
			Valid: false,
		}, nil
	}

	sessionID := ""
	isMember := false
	// if claims userid matches model, session_id is provided,
	// is in list of user sessions, set it as the sessionID
	// the session can be expired by time
	if ok, claims := u.tokenizer.GetClaims(sessionToken, sessionSubject); ok {
		if userid == claims.Userid {
			usersession := session.Session{
				Userid: claims.Userid,
			}
			userkey := usersession.UserKey()
			if isM, err := u.cache.Cache().HExists(userkey, claims.Id).Result(); err == nil && isM {
				sessionID = claims.Id
				isMember = isM
			}
		}
	}

	var s *session.Session
	if sessionID == "" {
		// otherwise, create a new sessionID
		if s, err = session.New(m, ipAddress, userAgent); err != nil {
			err.AddTrace(moduleIDAuth)
			return false, nil, err
		}
	} else {
		if s, err = session.FromSessionID(sessionID, userid, ipAddress, userAgent); err != nil {
			err.AddTrace(moduleIDAuth)
			return false, nil, err
		}
	}

	// generate an access token
	accessToken, claims, err := u.tokenizer.Generate(m, u.accessTime, authenticationSubject, "")
	if err != nil {
		err.AddTrace(moduleIDAuth)
		return false, nil, err
	}
	// generate a refresh token with the sessionKey
	refreshToken, _, err := u.tokenizer.Generate(m, u.refreshTime, refreshSubject, s.SessionID+":"+s.SessionKey)
	if err != nil {
		err.AddTrace(moduleIDAuth)
		return false, nil, err
	}
	// generate a session token
	newSessionToken, _, err := u.tokenizer.Generate(m, u.refreshTime, sessionSubject, s.SessionID)
	if err != nil {
		err.AddTrace(moduleIDAuth)
		return false, nil, err
	}

	// store the session in cache
	sessionGob, err := s.ToGob()
	if err != nil {
		err.AddTrace(moduleIDAuth)
		return false, nil, err
	}
	if u.newLoginEmail && !isMember {
		emdata := emailNewLogin{
			FirstName: m.FirstName,
			Username:  m.Username,
			SessionID: s.SessionID,
			IP:        s.IP,
			Time:      time.Unix(s.Time, 0).String(),
			UserAgent: s.UserAgent,
		}

		em, err := u.tpl.ExecuteHTML(newLoginTemplate, emdata)
		if err != nil {
			err.AddTrace(moduleIDAuth)
			return false, nil, err
		}
		subj, err := u.tpl.ExecuteHTML(newLoginSubject, emdata)
		if err != nil {
			err.AddTrace(moduleIDAuth)
			return false, nil, err
		}

		if err := u.mailer.Send(m.Email, subj, em); err != nil {
			err.AddTrace(moduleIDAuth)
			return false, nil, err
		}
	}

	// add to list of user sessions
	if err := u.cache.Cache().HSet(s.UserKey(), s.SessionID, sessionGob).Err(); err != nil {
		return false, nil, governor.NewError(moduleIDAuth, err.Error(), 0, http.StatusInternalServerError)
	}

	// set the session id and key into cache
	if err := u.cache.Cache().Set(s.SessionID, s.SessionKey, time.Duration(u.refreshTime*b1)).Err(); err != nil {
		return false, nil, governor.NewError(moduleIDAuth, err.Error(), 0, http.StatusInternalServerError)
	}

	return true, &resUserAuth{
		Valid:        true,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		SessionToken: newSessionToken,
		Claims:       claims,
	}, nil
}

// ExchangeToken validates a refresh token and returns an auth token
func (u *userService) ExchangeToken(refreshToken, ipAddress, userAgent string) (bool, *resUserAuth, *governor.Error) {
	sessionID := ""
	sessionKey := ""
	userid := ""
	// if session_id is provided, is in cache, and is valid, set it as the sessionID
	// the session cannot be expired
	if ok, claims := u.tokenizer.GetClaims(refreshToken, refreshSubject); ok {
		if s := strings.Split(claims.Id, ":"); len(s) == 2 {
			if key, err := u.cache.Cache().Get(s[0]).Result(); err == nil {
				sessionID = s[0]
				sessionKey = key
				userid = claims.Userid
			}
		}
	}

	if sessionID == "" {
		return false, nil, governor.NewErrorUser(moduleIDAuth, "malformed refresh token", 0, http.StatusUnauthorized)
	}

	// check the refresh token
	validToken, claims := u.tokenizer.Validate(refreshToken, refreshSubject, sessionID+":"+sessionKey)
	if !validToken {
		return false, &resUserAuth{
			Valid: false,
		}, nil
	}

	s, err := session.FromSessionID(sessionID, userid, ipAddress, userAgent)
	if err != nil {
		err.AddTrace(moduleIDAuth)
		return false, nil, err
	}
	sessionGob, err := s.ToGob()
	if err != nil {
		err.AddTrace(moduleIDAuth)
		return false, nil, err
	}
	if err := u.cache.Cache().HSet(s.UserKey(), s.SessionID, sessionGob).Err(); err != nil {
		return false, nil, governor.NewError(moduleIDAuth, err.Error(), 0, http.StatusInternalServerError)
	}

	// generate a new accessToken from the refreshToken claims
	accessToken, err := u.tokenizer.GenerateFromClaims(claims, u.accessTime, authenticationSubject, "")
	if err != nil {
		err.AddTrace(moduleIDAuth)
		return false, nil, err
	}

	return true, &resUserAuth{
		Valid:       true,
		AccessToken: accessToken,
		Claims:      claims,
	}, nil
}
