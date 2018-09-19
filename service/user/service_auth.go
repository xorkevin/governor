package user

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/model"
	"github.com/hackform/governor/service/user/session"
	"github.com/hackform/governor/service/user/token"
	"net/http"
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
