package user

import (
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
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

func (m *router) createUser(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqUserPost{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.CreateUser(req)
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

func (m *router) commitUser(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqUserPostConfirm{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.CommitUser(req.Userid, req.Key)
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

func (m *router) deleteUserSelf(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqUserDeleteSelf{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := m.s.DeleteUser(req.Userid, req.Username, false, req.Password); err != nil {
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

func (m *router) deleteUser(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqUserDelete{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = c.Param("id")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := m.s.DeleteUser(req.Userid, req.Username, true, ""); err != nil {
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

func (m *router) getUserApprovals(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqGetUserApprovals{
		Amount: c.QueryInt("amount", -1),
		Offset: c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.GetUserApprovals(req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}

	c.WriteJSON(http.StatusOK, res)
}

func (m *router) approveUser(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqUserGetID{
		Userid: c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := m.s.ApproveUser(req.Userid); err != nil {
		c.WriteError(err)
		return
	}

	c.WriteStatus(http.StatusNoContent)
}

func (m *router) deleteUserApproval(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqUserGetID{
		Userid: c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := m.s.DeleteUserApproval(req.Userid); err != nil {
		c.WriteError(err)
		return
	}

	c.WriteStatus(http.StatusNoContent)
}

func (m *router) mountCreate(r governor.Router) {
	scopeApprovalRead := m.s.scopens + ".approval:read"
	scopeApprovalWrite := m.s.scopens + ".approval:write"
	scopeAccountDelete := m.s.scopens + ".account:delete"
	scopeAdminAccount := m.s.scopens + ".admin.account:delete"
	r.Post("", m.createUser, m.rt)
	r.Post("/confirm", m.commitUser, m.rt)
	r.Get("/approvals", m.getUserApprovals, gate.Member(m.s.gate, m.s.rolens, scopeApprovalRead), m.rt)
	r.Post("/approvals/id/{id}", m.approveUser, gate.Member(m.s.gate, m.s.rolens, scopeApprovalWrite), m.rt)
	r.Delete("/approvals/id/{id}", m.deleteUserApproval, gate.Member(m.s.gate, m.s.rolens, scopeApprovalWrite), m.rt)
	r.Delete("", m.deleteUserSelf, gate.User(m.s.gate, scopeAccountDelete), m.rt)
	r.Delete("/id/{id}", m.deleteUser, gate.Admin(m.s.gate, scopeAdminAccount), m.rt)
}
