package session

import (
	"bytes"
	"encoding/gob"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/model"
	"github.com/hackform/governor/util/uid"
	"net/http"
	"time"
)

const (
	moduleID = "session"
)

type (
	// Session is a user session
	Session struct {
		Userid     string `json:"userid"`
		SessionID  string `json:"session_id"`
		SessionKey string `json:"-"`
		Time       int64  `json:"time"`
		IP         string `json:"ip"`
		UserAgent  string `json:"user_agent"`
	}

	// Slice is a session slice
	Slice []Session
)

// New creates a new session
func New(m *usermodel.Model, ipAddress, userAgent string) (*Session, error) {
	userid, err := usermodel.ParseB64ID(m.Userid)
	if err != nil {
		return nil, governor.NewError("Failed to parse userid", http.StatusBadRequest, err)
	}
	id, err := uid.New(4, 8, 4, userid.Bytes())
	if err != nil {
		return nil, governor.NewError("Failed to create new uid", http.StatusInternalServerError, err)
	}
	return FromSessionID(id.Base64(), m.Userid, ipAddress, userAgent)
}

// FromSessionID creates a new session from an existing sessionID
func FromSessionID(sessionID, userid, ipAddress, userAgent string) (*Session, error) {
	key, err := uid.NewU(0, 16)
	if err != nil {
		return nil, governor.NewError("Failed to create new uid", http.StatusInternalServerError, err)
	}

	return &Session{
		Userid:     userid,
		SessionID:  sessionID,
		SessionKey: key.Base64(),
		Time:       time.Now().Unix(),
		IP:         ipAddress,
		UserAgent:  userAgent,
	}, nil
}

// ToGob returns the session object as a gob
func (s *Session) ToGob() (string, error) {
	b := bytes.Buffer{}
	if err := gob.NewEncoder(&b).Encode(s); err != nil {
		return "", governor.NewError("Failed to encode session to gob", http.StatusInternalServerError, err)
	}
	return b.String(), nil
}

// UserKey returns the session key of the user
func (s *Session) UserKey() string {
	return "usersession:" + s.Userid
}

// Len is the number of elements in the collection.
func (s Slice) Len() int {
	return len(s)
}

// Less reports whether the element with
// index i should sort before the element with index j.
func (s Slice) Less(i, j int) bool {
	return s[i].Time < s[j].Time
}

// Swap swaps the elements with indexes i and j.
func (s Slice) Swap(i, j int) {
	k := s[i]
	s[i] = s[j]
	s[j] = k
}
