package user

import (
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/service/user/token"
)

//go:generate forge validation -o validation_create_gen.go reqUserPost reqUserPostConfirm reqUserDeleteSelf reqUserDelete reqGetUserApprovals

type (
	reqUserPost struct {
		Username  string `valid:"username" json:"username"`
		Password  string `valid:"password" json:"password"`
		Email     string `valid:"email" json:"email"`
		FirstName string `valid:"firstName" json:"first_name"`
		LastName  string `valid:"lastName" json:"last_name"`
	}
)

func (s *router) createUser(c governor.Context) {
	var req reqUserPost
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := s.s.CreateUser(c.Ctx(), req)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusCreated, res)
}

type (
	reqUserPostConfirm struct {
		Userid string `valid:"userid,has" json:"userid"`
		Key    string `valid:"token,has" json:"key"`
	}
)

func (s *router) commitUser(c governor.Context) {
	var req reqUserPostConfirm
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := s.s.CommitUser(c.Ctx(), req.Userid, req.Key)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusCreated, res)
}

type (
	reqUserDeleteSelf struct {
		Userid   string `valid:"userid,has" json:"-"`
		Username string `valid:"username,has" json:"username"`
		Password string `valid:"password,has" json:"password"`
	}
)

func (s *router) deleteUserSelf(c governor.Context) {
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

	if err := s.s.DeleteUser(c.Ctx(), req.Userid, req.Username, false, req.Password); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	reqUserDelete struct {
		Userid   string `valid:"userid,has" json:"-"`
		Username string `valid:"username,has" json:"username"`
	}
)

func (s *router) deleteUser(c governor.Context) {
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

	if err := s.s.DeleteUser(c.Ctx(), req.Userid, req.Username, true, ""); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	reqGetUserApprovals struct {
		Amount int `valid:"amount" json:"-"`
		Offset int `valid:"offset" json:"-"`
	}
)

func (s *router) getUserApprovals(c governor.Context) {
	req := reqGetUserApprovals{
		Amount: c.QueryInt("amount", -1),
		Offset: c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := s.s.GetUserApprovals(c.Ctx(), req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}

	c.WriteJSON(http.StatusOK, res)
}

func (s *router) approveUser(c governor.Context) {
	req := reqUserGetID{
		Userid: c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := s.s.ApproveUser(c.Ctx(), req.Userid); err != nil {
		c.WriteError(err)
		return
	}

	c.WriteStatus(http.StatusNoContent)
}

func (s *router) deleteUserApproval(c governor.Context) {
	req := reqUserGetID{
		Userid: c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := s.s.DeleteUserApproval(c.Ctx(), req.Userid); err != nil {
		c.WriteError(err)
		return
	}

	c.WriteStatus(http.StatusNoContent)
}

func (s *router) mountCreate(m *governor.MethodRouter) {
	scopeApprovalRead := s.s.scopens + ".approval:read"
	scopeApprovalWrite := s.s.scopens + ".approval:write"
	scopeAdminAccount := s.s.scopens + ".admin.account:delete"
	m.PostCtx("", s.createUser, s.rt)
	m.PostCtx("/confirm", s.commitUser, s.rt)
	m.GetCtx("/approvals", s.getUserApprovals, gate.Member(s.s.gate, s.s.rolens, scopeApprovalRead), s.rt)
	m.PostCtx("/approvals/id/{id}", s.approveUser, gate.Member(s.s.gate, s.s.rolens, scopeApprovalWrite), s.rt)
	m.DeleteCtx("/approvals/id/{id}", s.deleteUserApproval, gate.Member(s.s.gate, s.s.rolens, scopeApprovalWrite), s.rt)
	m.DeleteCtx("", s.deleteUserSelf, gate.User(s.s.gate, token.ScopeForbidden), s.rt)
	m.DeleteCtx("/id/{id}", s.deleteUser, gate.Admin(s.s.gate, scopeAdminAccount), s.rt)
}
