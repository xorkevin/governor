package user

import (
	"errors"
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

func (s *router) putUser(c governor.Context) {
	userid := gate.GetCtxUserid(c)

	var req reqUserPut
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := s.s.updateUser(c.Ctx(), userid, req); err != nil {
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

func (s *router) patchRank(c governor.Context) {
	var req reqUserPutRank
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = c.Param("id")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	updaterUserid := gate.GetCtxUserid(c)
	editAddRank, err := rank.FromSlice(req.Add)
	if err != nil {
		if errors.Is(err, rank.ErrorInvalidRank{}) {
			c.WriteError(governor.ErrWithRes(err, http.StatusBadRequest, "", "Invalid rank string"))
			return
		}
		c.WriteError(err)
		return
	}
	editRemoveRank, err := rank.FromSlice(req.Remove)
	if err != nil {
		if errors.Is(err, rank.ErrorInvalidRank{}) {
			c.WriteError(governor.ErrWithRes(err, http.StatusBadRequest, "", "Invalid rank string"))
			return
		}
		c.WriteError(err)
		return
	}

	if err := s.s.updateRank(c.Ctx(), req.Userid, updaterUserid, editAddRank, editRemoveRank); err != nil {
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

func (s *router) postAcceptRoleInvitation(c governor.Context) {
	req := reqAcceptRoleInvitation{
		Userid: gate.GetCtxUserid(c),
		Role:   c.Param("role"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := s.s.acceptRoleInvitation(c.Ctx(), req.Userid, req.Role); err != nil {
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

func (s *router) getRoleInvitations(c governor.Context) {
	req := reqGetRoleInvitations{
		Role:   c.Param("role"),
		Amount: c.QueryInt("amount", -1),
		Offset: c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := s.s.getRoleInvitations(c.Ctx(), req.Role, req.Amount, req.Offset)
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

func (s *router) getUserRoleInvitations(c governor.Context) {
	req := reqGetUserRoleInvitations{
		Userid: gate.GetCtxUserid(c),
		Amount: c.QueryInt("amount", -1),
		Offset: c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := s.s.getUserRoleInvitations(c.Ctx(), req.Userid, req.Amount, req.Offset)
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

func (s *router) deleteRoleInvitation(c governor.Context) {
	req := reqDelRoleInvitation{
		Userid: c.Param("id"),
		Role:   c.Param("role"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := s.s.deleteRoleInvitation(c.Ctx(), req.Userid, req.Role); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (s *router) deleteUserRoleInvitation(c governor.Context) {
	req := reqDelRoleInvitation{
		Userid: gate.GetCtxUserid(c),
		Role:   c.Param("role"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := s.s.deleteRoleInvitation(c.Ctx(), req.Userid, req.Role); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (s *router) roleMod(c governor.Context, _ string) (string, bool, bool) {
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

func (s *router) mountEdit(m *governor.MethodRouter) {
	scopeAccountRead := s.s.scopens + ".account:read"
	scopeAccountWrite := s.s.scopens + ".account:write"
	scopeAdminRead := s.s.scopens + ".admin:read"
	scopeAdminWrite := s.s.scopens + ".admin:write"
	m.PutCtx("", s.putUser, gate.User(s.s.gate, token.ScopeForbidden), s.rt)
	m.PatchCtx("/id/{id}/rank", s.patchRank, gate.User(s.s.gate, scopeAdminWrite), s.rt)
	m.GetCtx("/roles/invitation", s.getUserRoleInvitations, gate.User(s.s.gate, scopeAccountRead), s.rt)
	m.PostCtx("/roles/invitation/{role}/accept", s.postAcceptRoleInvitation, gate.User(s.s.gate, scopeAccountWrite), s.rt)
	m.DeleteCtx("/roles/invitation/{role}", s.deleteUserRoleInvitation, gate.User(s.s.gate, scopeAccountWrite), s.rt)
	m.GetCtx("/role/{role}/invitation", s.getRoleInvitations, gate.ModF(s.s.gate, s.roleMod, scopeAdminRead), s.rt)
	m.DeleteCtx("/role/{role}/invitation/id/{id}", s.deleteRoleInvitation, gate.ModF(s.s.gate, s.roleMod, scopeAdminWrite), s.rt)
}
