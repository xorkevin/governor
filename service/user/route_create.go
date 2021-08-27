package user

import (
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/ratelimit"
	"xorkevin.dev/governor/service/user/gate"
)

//go:generate forge validation -o validation_create_gen.go reqUserPost reqUserPostConfirm reqUserDelete reqGetUserApprovals

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
	reqUserDelete struct {
		Userid   string `valid:"userid,has" json:"userid"`
		Username string `valid:"username,has" json:"username"`
		Password string `valid:"password,has" json:"password"`
	}
)

func (m *router) deleteUser(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqUserDelete{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if c.Param("id") != req.Userid {
		c.WriteError(governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Userid does not match",
		})))
		return
	}

	if err := m.s.DeleteUser(req.Userid, req.Username, req.Password); err != nil {
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

const (
	scopeApprovalRead  = "gov.user.approval:read"
	scopeApprovalWrite = "gov.user.approval:write"
	scopeAccountDelete = "gov.user.account:delete"
)

func (m *router) mountCreate(r governor.Router) {
	rt := ratelimit.Compose(m.s.ratelimiter, ratelimit.IPAddress("ip", 60, 15, 240))
	r.Post("", m.createUser, rt)
	r.Post("/confirm", m.commitUser, rt)
	r.Get("/approvals", m.getUserApprovals, gate.Member(m.s.gate, "gov.user", scopeApprovalRead))
	r.Post("/approvals/id/{id}", m.approveUser, gate.Member(m.s.gate, "gov.user", scopeApprovalWrite))
	r.Delete("/approvals/id/{id}", m.deleteUserApproval, gate.Member(m.s.gate, "gov.user", scopeApprovalWrite))
	r.Delete("/id/{id}", m.deleteUser, gate.OwnerParam(m.s.gate, "id", scopeAccountDelete))
}
