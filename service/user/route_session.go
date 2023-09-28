package user

import (
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/gate"
)

type (
	//forge:valid
	reqGetUserSessions struct {
		Userid string `valid:"userid,has" json:"-"`
		Amount int    `valid:"amount" json:"-"`
		Offset int    `valid:"offset" json:"-"`
	}
)

func (s *router) getSessions(c *governor.Context) {
	req := reqGetUserSessions{
		Userid: gate.GetCtxUserid(c),
		Amount: c.QueryInt("amount", -1),
		Offset: c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.getUserSessions(c.Ctx(), req.Userid, req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqUserRmSession struct {
		Userid    string `valid:"userid,has" json:"-"`
		SessionID string `valid:"sessionID,has" json:"session_id"`
	}
)

func (s *router) killSession(c *governor.Context) {
	var req reqUserRmSession
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := s.s.killSession(c.Ctx(), req.Userid, req.SessionID); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (s *router) mountSession(m *governor.MethodRouter) {
	scopeSessionRead := s.s.scopens + ".session:read"
	scopeSessionWrite := s.s.scopens + ".session:write"
	m.GetCtx("/sessions", s.getSessions, gate.AuthUser(s.s.gate, scopeSessionRead), s.rt)
	m.DeleteCtx("/session", s.killSession, gate.AuthUser(s.s.gate, scopeSessionWrite), s.rt)
}
