package user

import (
	"bytes"
	"encoding/gob"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/session"
	"net/http"
	"sort"
)

type (
	resUserGetSessions struct {
		Sessions []session.Session `json:"active_sessions"`
	}
)

// GetSessions retrieves a list of user sessions
func (u *userService) GetSessions(userid string) (*resUserGetSessions, *governor.Error) {
	ch := u.cache.Cache()

	s := session.Session{
		Userid: userid,
	}

	var sarr session.Slice
	if sgobs, err := ch.HGetAll(s.UserKey()).Result(); err == nil {
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
