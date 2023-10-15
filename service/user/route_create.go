package user

import (
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/gate"
)

type (
	//forge:valid
	reqUserPost struct {
		Username  string `valid:"username" json:"username"`
		Password  string `valid:"password" json:"password"`
		Email     string `valid:"email" json:"email"`
		FirstName string `valid:"firstName" json:"first_name"`
		LastName  string `valid:"lastName" json:"last_name"`
	}
)

func (s *router) createUser(c *governor.Context) {
	var req reqUserPost
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := s.s.createUser(c.Ctx(), req)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusCreated, res)
}

type (
	//forge:valid
	reqUserPostConfirm struct {
		Userid string `valid:"userid,has" json:"userid"`
		Key    string `valid:"token,has" json:"key"`
	}
)

func (s *router) commitUser(c *governor.Context) {
	var req reqUserPostConfirm
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := s.s.commitUser(c.Ctx(), req.Userid, req.Key)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusCreated, res)
}

type (
	//forge:valid
	reqUserDeleteSelf struct {
		Userid   string `valid:"userid,has" json:"-"`
		Username string `valid:"username,has" json:"username"`
	}
)

func (s *router) deleteUserSelf(c *governor.Context) {
	var req reqUserDeleteSelf
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := s.s.deleteUser(c.Ctx(), req.Userid, req.Username); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	//forge:valid
	reqUserDelete struct {
		Userid   string `valid:"userid,has" json:"-"`
		Username string `valid:"username,has" json:"username"`
	}
)

func (s *router) deleteUser(c *governor.Context) {
	var req reqUserDelete
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = c.Param("id")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := s.s.deleteUser(c.Ctx(), req.Userid, req.Username); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (s *router) addAdmin(c *governor.Context) {
	var req reqUserPost
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := s.s.addAdmin(c.Ctx(), req)
	if err != nil {
		c.WriteError(err)
		return
	}

	c.WriteJSON(http.StatusCreated, res)
}

type (
	//forge:valid
	reqGetUserApprovals struct {
		Amount int `valid:"amount" json:"-"`
		Offset int `valid:"offset" json:"-"`
	}
)

func (s *router) getUserApprovals(c *governor.Context) {
	req := reqGetUserApprovals{
		Amount: c.QueryInt("amount", -1),
		Offset: c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := s.s.getUserApprovals(c.Ctx(), req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}

	c.WriteJSON(http.StatusOK, res)
}

func (s *router) approveUser(c *governor.Context) {
	req := reqUserGetID{
		Userid: c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := s.s.approveUser(c.Ctx(), req.Userid); err != nil {
		c.WriteError(err)
		return
	}

	c.WriteStatus(http.StatusNoContent)
}

func (s *router) deleteUserApproval(c *governor.Context) {
	req := reqUserGetID{
		Userid: c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := s.s.deleteUserApproval(c.Ctx(), req.Userid); err != nil {
		c.WriteError(err)
		return
	}

	c.WriteStatus(http.StatusNoContent)
}

func (s *router) mountCreate(m *governor.MethodRouter) {
	scopeApprovalRead := s.s.scopens + ".approval:read"
	scopeApprovalWrite := s.s.scopens + ".approval:write"
	scopeAdminAccount := s.s.scopens + ".admin.account:delete"
	scopeAddAdmin := s.s.scopens + ".add.admin:write"
	m.PostCtx("", s.createUser, s.rt)
	m.PostCtx("/confirm", s.commitUser, s.rt)
	m.PostCtx("/admin", s.addAdmin, gate.AuthSystem(s.s.gate, scopeAddAdmin), s.rt)
	m.GetCtx("/approvals", s.getUserApprovals, gate.AuthMember(s.s.gate, s.s.rolens, scopeApprovalRead), s.rt)
	m.PostCtx("/approvals/id/{id}", s.approveUser, gate.AuthMember(s.s.gate, s.s.rolens, scopeApprovalWrite), s.rt)
	m.DeleteCtx("/approvals/id/{id}", s.deleteUserApproval, gate.AuthMember(s.s.gate, s.s.rolens, scopeApprovalWrite), s.rt)
	m.DeleteCtx("", s.deleteUserSelf, gate.AuthUserSudo(s.s.gate, s.s.authSettings.sudoDuration, gate.ScopeNone), s.rt)
	m.DeleteCtx("/id/{id}", s.deleteUser, gate.AuthAdminSudo(s.s.gate, s.s.authSettings.sudoDuration, scopeAdminAccount), s.rt)
}
