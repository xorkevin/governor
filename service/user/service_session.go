package user

import (
	"xorkevin.dev/governor"
)

type (
	resSession struct {
		SessionID string `json:"session_id"`
		Userid    string `json:"userid"`
		Time      int64  `json:"time"`
		AuthTime  int64  `json:"auth_time"`
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
		return nil, governor.ErrWithMsg(err, "Failed to get user sessions")
	}
	res := make([]resSession, 0, len(m))
	for _, i := range m {
		res = append(res, resSession{
			SessionID: i.SessionID,
			Userid:    i.Userid,
			Time:      i.Time,
			AuthTime:  i.AuthTime,
			IPAddr:    i.IPAddr,
			UserAgent: i.UserAgent,
		})
	}
	return &resUserGetSessions{
		Sessions: res,
	}, nil
}

func (s *service) killCacheSessions(sessionids []string) {
	if err := s.kvsessions.Del(sessionids...); err != nil {
		s.logger.Error("Failed to delete session keys", map[string]string{
			"error":      err.Error(),
			"actiontype": "clearcachesessionids",
		})
	}
}

// KillSessions terminates user sessions
func (s *service) KillSessions(sessionids []string) error {
	if err := s.sessions.DeleteSessions(sessionids); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete user sessions")
	}
	s.killCacheSessions(sessionids)
	return nil
}

// KillAllSessions terminates all sessions of a user
func (s *service) KillAllSessions(userid string) error {
	if err := s.sessions.DeleteUserSessions(userid); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete user sessions")
	}
	return nil
}
