package user

import (
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/service/user/token"
	"xorkevin.dev/governor/util/rank"
)

//go:generate forge validation -o validation_edit_gen.go reqUserPut reqUserPutRank reqAcceptRoleInvitation reqGetRoleInvitations reqGetUserRoleInvitations reqDelRoleInvitation

type (
	reqUserPut struct {
		Username  string `valid:"username" json:"username"`
		FirstName string `valid:"firstName" json:"first_name"`
		LastName  string `valid:"lastName" json:"last_name"`
	}
)

func (m *router) putUser(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	userid := gate.GetCtxUserid(c)

	req := reqUserPut{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := m.s.UpdateUser(c.Ctx(), userid, req); err != nil {
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

	updaterUserid := gate.GetCtxUserid(c)
	editAddRank := rank.FromSlice(req.Add)
	editRemoveRank := rank.FromSlice(req.Remove)

	if err := m.s.UpdateRank(c.Ctx(), req.Userid, updaterUserid, editAddRank, editRemoveRank); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	reqAcceptRoleInvitation struct {
		Userid string `valid:"userid,has" json:"-"`
		Role   string `valid:"role,has" json:"-"`
	}
)

func (m *router) postAcceptRoleInvitation(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqAcceptRoleInvitation{
		Userid: gate.GetCtxUserid(c),
		Role:   c.Param("role"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := m.s.AcceptRoleInvitation(c.Ctx(), req.Userid, req.Role); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	reqGetRoleInvitations struct {
		Role   string `valid:"role,has" json:"-"`
		Amount int    `valid:"amount" json:"-"`
		Offset int    `valid:"offset" json:"-"`
	}
)

func (m *router) getRoleInvitations(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqGetRoleInvitations{
		Role:   c.Param("role"),
		Amount: c.QueryInt("amount", -1),
		Offset: c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.GetRoleInvitations(c.Ctx(), req.Role, req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	reqGetUserRoleInvitations struct {
		Userid string `valid:"userid,has" json:"-"`
		Amount int    `valid:"amount" json:"-"`
		Offset int    `valid:"offset" json:"-"`
	}
)

func (m *router) getUserRoleInvitations(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqGetUserRoleInvitations{
		Userid: gate.GetCtxUserid(c),
		Amount: c.QueryInt("amount", -1),
		Offset: c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.GetUserRoleInvitations(c.Ctx(), req.Userid, req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	reqDelRoleInvitation struct {
		Userid string `valid:"userid,has" json:"-"`
		Role   string `valid:"role,has" json:"-"`
	}
)

func (m *router) deleteRoleInvitation(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqDelRoleInvitation{
		Userid: c.Param("id"),
		Role:   c.Param("role"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := m.s.DeleteRoleInvitation(c.Ctx(), req.Userid, req.Role); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (m *router) deleteUserRoleInvitation(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqDelRoleInvitation{
		Userid: gate.GetCtxUserid(c),
		Role:   c.Param("role"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := m.s.DeleteRoleInvitation(c.Ctx(), req.Userid, req.Role); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (m *router) roleMod(c governor.Context, _ string) (string, bool, bool) {
	role := c.Param("role")
	if err := validhasRole(role); err != nil {
		return "", false, false
	}
	if role == rank.TagAdmin {
		return "", false, true
	}
	_, tag, err := rank.SplitTag(role)
	if err != nil {
		return "", false, false
	}
	return tag, false, true
}

func (m *router) mountEdit(r governor.Router) {
	scopeAccountRead := m.s.scopens + ".account:read"
	scopeAccountWrite := m.s.scopens + ".account:write"
	scopeAdminRead := m.s.scopens + ".admin:read"
	scopeAdminWrite := m.s.scopens + ".admin:write"
	r.Put("", m.putUser, gate.User(m.s.gate, token.ScopeForbidden), m.rt)
	r.Patch("/id/{id}/rank", m.patchRank, gate.User(m.s.gate, scopeAdminWrite), m.rt)
	r.Get("/roles/invitation", m.getUserRoleInvitations, gate.User(m.s.gate, scopeAccountRead), m.rt)
	r.Post("/roles/invitation/{role}/accept", m.postAcceptRoleInvitation, gate.User(m.s.gate, scopeAccountWrite), m.rt)
	r.Delete("/roles/invitation/{role}", m.deleteUserRoleInvitation, gate.User(m.s.gate, scopeAccountWrite), m.rt)
	r.Get("/role/{role}/invitation", m.getRoleInvitations, gate.ModF(m.s.gate, m.roleMod, scopeAdminRead), m.rt)
	r.Delete("/role/{role}/invitation/id/{id}", m.deleteRoleInvitation, gate.ModF(m.s.gate, m.roleMod, scopeAdminWrite), m.rt)
}
