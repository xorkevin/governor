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

func (u *userService) GetUserSessions(userid string) (*resUserGetSessions, error) {
	m, err := u.sessionrepo.GetUserSessions(userid, 256, 0)
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
func (u *userService) KillCacheSessions(sessionids []string) error {
	ids := make([]string, 0, len(sessionids))
	for _, i := range sessionids {
		ids = append(ids, cachePrefixSession+i)
	}
	if err := u.cache.Cache().Del(ids...).Err(); err != nil {
		return governor.NewError("Failed to delete session keys", http.StatusInternalServerError, err)
	}
	return nil
}

// KillSessions terminates user sessions
func (u *userService) KillSessions(sessionids []string) error {
	if err := u.KillCacheSessions(sessionids); err != nil {
		return err
	}
	if err := u.sessionrepo.DeleteSessions(sessionids); err != nil {
		return governor.NewError("Failed to delete user sessions", http.StatusInternalServerError, err)
	}
	return nil
}

// KillAllCacheSessions terminates all sessions of a user in cache
func (u *userService) KillAllCacheSessions(userid string) error {
	sessionids, err := u.sessionrepo.GetUserSessionIDs(userid, 65536, 0)
	if err != nil {
		return governor.NewError("Failed to get user session ids", http.StatusInternalServerError, err)
	}
	if err := u.KillCacheSessions(sessionids); err != nil {
		return err
	}
	return nil
}

// KillAllSessions terminates all sessions of a user
func (u *userService) KillAllSessions(userid string) error {
	if err := u.KillAllCacheSessions(userid); err != nil {
		return err
	}
	if err := u.sessionrepo.DeleteUserSessions(userid); err != nil {
		return governor.NewError("Failed to delete user sessions", http.StatusInternalServerError, err)
	}
	return nil
}
