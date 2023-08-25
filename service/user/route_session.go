package user

import (
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
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
	reqUserRmSessions struct {
		Userid     string   `valid:"userid,has" json:"-"`
		SessionIDs []string `valid:"sessionids" json:"session_ids"`
	}
)

func (s *router) killSessions(c *governor.Context) {
	var req reqUserRmSessions
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := s.s.killSessions(c.Ctx(), req.Userid, req.SessionIDs); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (s *router) mountSession(m *governor.MethodRouter) {
	scopeSessionRead := s.s.scopens + ".session:read"
	scopeSessionWrite := s.s.scopens + ".session:write"
	m.GetCtx("/sessions", s.getSessions, gate.User(s.s.gate, scopeSessionRead), s.rt)
	m.DeleteCtx("/sessions", s.killSessions, gate.User(s.s.gate, scopeSessionWrite), s.rt)
}
