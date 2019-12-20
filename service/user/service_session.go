package user

import (
	"net/http"
	"xorkevin.dev/governor"
)

type (
	resSession struct {
		SessionID string `json:"session_id"`
		Userid    string `json:"userid"`
		Time      int64  `json:"time"`
		IPAddr    string `json:"ip"`
		UserAgent string `json:"user_agent"`
	}

	resUserGetSessions struct {
		Sessions []resSession `json:"active_sessions"`
	}
)

func (s *service) GetUserSessions(userid string, limit, offset int) (*resUserGetSessions, error) {
	m, err := s.sessions.GetUserSessions(userid, limit, offset)
	if err != nil {
		return nil, governor.NewError("Failed to get user sessions", http.StatusInternalServerError, err)
	}
	res := make([]resSession, 0, len(m))
	for _, i := range m {
		res = append(res, resSession{
			SessionID: i.SessionID,
			Userid:    i.Userid,
			Time:      i.Time,
			IPAddr:    i.IPAddr,
			UserAgent: i.UserAgent,
		})
	}
	return &resUserGetSessions{
		Sessions: res,
	}, nil
}

// KillCacheSessions terminates user sessions in cache
func (s *service) KillCacheSessions(sessionids []string) error {
	if err := s.kvsessions.Del(sessionids...); err != nil {
		return governor.NewError("Failed to delete session keys", http.StatusInternalServerError, err)
	}
	return nil
}

// KillSessions terminates user sessions
func (s *service) KillSessions(sessionids []string) error {
	if err := s.KillCacheSessions(sessionids); err != nil {
		return err
	}
	if err := s.sessions.DeleteSessions(sessionids); err != nil {
		return governor.NewError("Failed to delete user sessions", http.StatusInternalServerError, err)
	}
	return nil
}

// KillAllCacheSessions terminates all sessions of a user in cache
func (s *service) KillAllCacheSessions(userid string) error {
	sessionids, err := s.sessions.GetUserSessionIDs(userid, 65536, 0)
	if err != nil {
		return governor.NewError("Failed to get user session ids", http.StatusInternalServerError, err)
	}
	if err := s.KillCacheSessions(sessionids); err != nil {
		return err
	}
	return nil
}

// KillAllSessions terminates all sessions of a user
func (s *service) KillAllSessions(userid string) error {
	if err := s.KillAllCacheSessions(userid); err != nil {
		return err
	}
	if err := s.sessions.DeleteUserSessions(userid); err != nil {
		return governor.NewError("Failed to delete user sessions", http.StatusInternalServerError, err)
	}
	return nil
}
