package user

import (
	"bytes"
	"encoding/gob"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/session"
	"net/http"
	"sort"
	"time"
)

// GetSessions retrieves a list of user sessions
func (u *userService) GetSessions(userid string) (*resUserGetSessions, *governor.Error) {
	s := session.Session{
		Userid: userid,
	}

	var sarr session.Slice
	if sgobs, err := u.cache.Cache().HGetAll(s.UserKey()).Result(); err == nil {
		sarr = make(session.Slice, 0, len(sgobs))
		for _, v := range sgobs {
			s := session.Session{}
			if err = gob.NewDecoder(bytes.NewBufferString(v)).Decode(&s); err != nil {
				return nil, governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
			}
			sarr = append(sarr, s)
		}
	} else {
		return nil, governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}
	sort.Sort(sort.Reverse(sarr))

	return &resUserGetSessions{
		Sessions: sarr,
	}, nil
}

// GetSessionKey retrieves the key of a session
func (u *userService) GetSessionKey(sessionID string) (string, *governor.Error) {
	key, err := u.cache.Cache().Get(sessionID).Result()
	if err != nil {
		return "", governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}
	return key, nil
}

// EndSession ends the session of a user
func (u *userService) EndSession(sessionID string) *governor.Error {
	if err := u.cache.Cache().Del(sessionID).Err(); err != nil {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

// KillSessions terminates sessions of a user
func (u *userService) KillSessions(userid string, sessionIDs []string) *governor.Error {
	s := session.Session{
		Userid: userid,
	}
	if err := u.cache.Cache().Del(sessionIDs...).Err(); err != nil {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}
	if err := u.cache.Cache().HDel(s.UserKey(), sessionIDs...).Err(); err != nil {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

// KillAllSessions terminates all sessions of a user
func (u *userService) KillAllSessions(userid string) *governor.Error {
	s := session.Session{
		Userid: userid,
	}

	var sessionIDs []string
	if smap, err := u.cache.Cache().HGetAll(s.UserKey()).Result(); err == nil {
		sessionIDs = make([]string, 0, len(smap))
		for k := range smap {
			sessionIDs = append(sessionIDs, k)
		}
	} else {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}

	if len(sessionIDs) == 0 {
		return nil
	}

	return u.KillSessions(userid, sessionIDs)
}

// SessionExists checks if a session of a user exists
func (u *userService) SessionExists(userid, sessionID string) (bool, *governor.Error) {
	usersession := session.Session{
		Userid: userid,
	}
	ok, err := u.cache.Cache().HExists(usersession.UserKey(), sessionID).Result()
	if err != nil {
		return false, governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}
	return ok, nil
}

// UpdateUserSession updates a user session
func (u *userService) UpdateUserSession(s *session.Session) *governor.Error {
	sessionGob, err := s.ToGob()
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}
	if err := u.cache.Cache().HSet(s.UserKey(), s.SessionID, sessionGob).Err(); err != nil {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

// UpdateSessionKey updates the key of a session
func (u *userService) UpdateSessionKey(sessionID string, sessionKey string, cacheDuration time.Duration) *governor.Error {
	if err := u.cache.Cache().Set(sessionID, sessionKey, cacheDuration).Err(); err != nil {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

// AddSession adds a session to the cache
func (u *userService) AddSession(s *session.Session, cacheDuration time.Duration) *governor.Error {
	// add to list of user sessions
	if err := u.UpdateUserSession(s); err != nil {
		return err
	}
	// set the session id and key into cache
	if err := u.UpdateSessionKey(s.SessionID, s.SessionKey, cacheDuration); err != nil {
		return err
	}
	return nil
}
