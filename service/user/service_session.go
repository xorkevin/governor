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

const (
	cachePrefixUserSession = moduleID + ".usersession:"
	cachePrefixSession     = moduleID + ".session:"
)

type (
	resUserGetSessions struct {
		Sessions []session.Session `json:"active_sessions"`
	}
)

// GetSessions retrieves a list of user sessions
func (u *userService) GetSessions(userid string) (*resUserGetSessions, error) {
	s := session.Session{
		Userid: userid,
	}

	var sarr session.Slice
	if sgobs, err := u.cache.Cache().HGetAll(cachePrefixUserSession + s.UserKey()).Result(); err != nil {
		return nil, governor.NewError("Failed to get user sessions", http.StatusInternalServerError, err)
	} else {
		sarr = make(session.Slice, 0, len(sgobs))
		for _, v := range sgobs {
			s := session.Session{}
			if err = gob.NewDecoder(bytes.NewBufferString(v)).Decode(&s); err != nil {
				return nil, governor.NewError("Failed to decode user session", http.StatusInternalServerError, err)
			}
			sarr = append(sarr, s)
		}
	}
	sort.Sort(sort.Reverse(sarr))

	return &resUserGetSessions{
		Sessions: sarr,
	}, nil
}

// GetSessionKey retrieves the key of a session
func (u *userService) GetSessionKey(sessionID string) (string, error) {
	key, err := u.cache.Cache().Get(cachePrefixSession + sessionID).Result()
	if err != nil {
		return "", governor.NewError("Failed to get session key", http.StatusInternalServerError, err)
	}
	return key, nil
}

// EndSession ends the session of a user
func (u *userService) EndSession(sessionID string) error {
	if err := u.cache.Cache().Del(cachePrefixSession + sessionID).Err(); err != nil {
		return governor.NewError("Failed to kill session", http.StatusInternalServerError, err)
	}
	return nil
}

// KillSessions terminates sessions of a user
func (u *userService) KillSessions(userid string, sessionIDs []string) error {
	s := session.Session{
		Userid: userid,
	}
	ids := make([]string, 0, len(sessionIDs))
	for _, i := range sessionIDs {
		ids = append(ids, cachePrefixSession+i)
	}
	if err := u.cache.Cache().Del(ids...).Err(); err != nil {
		return governor.NewError("Failed to delete session keys", http.StatusInternalServerError, err)
	}
	if err := u.cache.Cache().HDel(cachePrefixUserSession+s.UserKey(), sessionIDs...).Err(); err != nil {
		return governor.NewError("Failed to delete sessions map", http.StatusInternalServerError, err)
	}
	return nil
}

// KillAllSessions terminates all sessions of a user
func (u *userService) KillAllSessions(userid string) error {
	s := session.Session{
		Userid: userid,
	}

	var sessionIDs []string
	if smap, err := u.cache.Cache().HGetAll(cachePrefixUserSession + s.UserKey()).Result(); err != nil {
		return governor.NewError("Failed to get sessions map", http.StatusInternalServerError, err)
	} else {
		sessionIDs = make([]string, 0, len(smap))
		for k := range smap {
			sessionIDs = append(sessionIDs, k)
		}
	}

	if len(sessionIDs) == 0 {
		return nil
	}

	return u.KillSessions(userid, sessionIDs)
}

// SessionExists checks if a session of a user exists
func (u *userService) SessionExists(userid, sessionID string) (bool, error) {
	s := session.Session{
		Userid: userid,
	}
	ok, err := u.cache.Cache().HExists(cachePrefixUserSession+s.UserKey(), sessionID).Result()
	if err != nil {
		return false, governor.NewError("Failed to check if session exists", http.StatusInternalServerError, err)
	}
	return ok, nil
}

// UpdateUserSession updates a user session
func (u *userService) UpdateUserSession(s *session.Session) error {
	sessionGob, err := s.ToGob()
	if err != nil {
		return governor.NewError("Failed to encode user session", http.StatusInternalServerError, err)
	}
	if err := u.cache.Cache().HSet(cachePrefixUserSession+s.UserKey(), s.SessionID, sessionGob).Err(); err != nil {
		return governor.NewError("Failed to store user session", http.StatusInternalServerError, err)
	}
	return nil
}

// UpdateSessionKey updates the key of a session
func (u *userService) UpdateSessionKey(sessionID string, sessionKey string, cacheDuration time.Duration) error {
	if err := u.cache.Cache().Set(cachePrefixSession+sessionID, sessionKey, cacheDuration).Err(); err != nil {
		return governor.NewError("Failed to update user session", http.StatusInternalServerError, err)
	}
	return nil
}

// AddSession adds a session to the cache
func (u *userService) AddSession(s *session.Session, cacheDuration time.Duration) error {
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
