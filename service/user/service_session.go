package user

import (
	"bytes"
	"encoding/gob"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/session"
	"net/http"
	"sort"
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

func (u *userService) KillAllSessions(userid string) *governor.Error {
	sessions, err := u.GetSessions(userid)
	if err != nil {
		return err
	}

	if len(sessions.Sessions) == 0 {
		return nil
	}

	sessionIDs := make([]string, 0, len(sessions.Sessions))
	for _, v := range sessions.Sessions {
		sessionIDs = append(sessionIDs, v.SessionID)
	}

	return u.KillSessions(userid, sessionIDs)
}
