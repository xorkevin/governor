package user

import (
	"net/http"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/util/rank"
)

//go:generate forge validation -o validation_edit_gen.go reqUserPut reqUserPutRank

type (
	reqUserPut struct {
		Username  string `valid:"username" json:"username"`
		FirstName string `valid:"firstName" json:"first_name"`
		LastName  string `valid:"lastName" json:"last_name"`
	}
)

func (m *router) putUser(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	userid := c.Get(gate.CtxUserid).(string)

	req := reqUserPut{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := m.s.UpdateUser(userid, req); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	reqUserPutRank struct {
		Userid string   `valid:"userid,has" json:"-"`
		Add    []string `valid:"rank" json:"add"`
		Remove []string `valid:"rank" json:"remove"`
	}
)

func (m *router) patchRank(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqUserPutRank{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = c.Param("id")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	updaterUserid := c.Get(gate.CtxUserid).(string)
	editAddRank := rank.FromSlice(req.Add)
	editRemoveRank := rank.FromSlice(req.Remove)

	if err := m.s.UpdateRank(req.Userid, updaterUserid, editAddRank, editRemoveRank); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (m *router) mountEdit(r governor.Router) {
	r.Put("", m.putUser, gate.User(m.s.gate))
	r.Patch("/id/{id}/rank", m.patchRank, gate.User(m.s.gate))
}
