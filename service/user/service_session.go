package user

import (
	"context"

	"xorkevin.dev/kerrors"
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

func (s *Service) getUserSessions(ctx context.Context, userid string, limit, offset int) (*resUserGetSessions, error) {
	m, err := s.sessions.GetUserSessions(ctx, userid, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get user sessions")
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

func (s *Service) killSessions(ctx context.Context, userid string, sessionids []string) error {
	if err := s.sessions.DeleteSessions(ctx, userid, sessionids); err != nil {
		return kerrors.WithMsg(err, "Failed to delete user sessions")
	}
	return nil
}

func (s *Service) killAllSessions(ctx context.Context, userid string) error {
	if err := s.sessions.DeleteUserSessions(ctx, userid); err != nil {
		return kerrors.WithMsg(err, "Failed to delete user sessions")
	}
	return nil
}
