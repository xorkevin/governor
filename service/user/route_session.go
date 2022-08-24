package user

import (
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/service/user/session/model"
)

//go:generate forge validation -o validation_session_gen.go reqGetUserSessions reqUserRmSessions

type (
	reqGetUserSessions struct {
		Userid string `valid:"userid,has" json:"-"`
		Amount int    `valid:"amount" json:"-"`
		Offset int    `valid:"offset" json:"-"`
	}
)

func (m *router) getSessions(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqGetUserSessions{
		Userid: gate.GetCtxUserid(c),
		Amount: c.QueryInt("amount", -1),
		Offset: c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := m.s.GetUserSessions(req.Userid, req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	reqUserRmSessions struct {
		Userid     string   `valid:"userid,has" json:"-"`
		SessionIDs []string `valid:"sessionIDs" json:"session_ids"`
	}
)

func (r *reqUserRmSessions) validUserid() error {
	for _, i := range r.SessionIDs {
		userid, err := model.ParseIDUserid(i)
		if err != nil {
			return governor.ErrWithRes(err, http.StatusBadRequest, "", "Invalid session ids")
		}
		if r.Userid != userid {
			return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Invalid session ids")
		}
	}
	return nil
}

func (m *router) killSessions(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqUserRmSessions{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := req.validUserid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := m.s.KillSessions(req.SessionIDs); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (m *router) mountSession(r governor.Router) {
	scopeSessionRead := m.s.scopens + ".session:read"
	scopeSessionWrite := m.s.scopens + ".session:write"
	r.Get("/sessions", m.getSessions, gate.User(m.s.gate, scopeSessionRead), m.rt)
	r.Delete("/sessions", m.killSessions, gate.User(m.s.gate, scopeSessionWrite), m.rt)
}
