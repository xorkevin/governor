package user

import (
	"net/http"
	"strconv"
	"strings"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
)

//go:generate forge validation -o validation_get_gen.go reqUserGetID reqUserGetUsername reqGetRoleUser reqGetUserBulk reqGetUsers

type (
	reqUserGetID struct {
		Userid string `valid:"userid,has" json:"-"`
	}
)

func (m *router) getByID(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqUserGetID{
		Userid: c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.GetByIDPublic(req.Userid)
	if err != nil {
		c.WriteError(err)
		return
	}

	c.WriteJSON(http.StatusOK, res)
}

func (m *router) getByIDPersonal(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqUserGetID{
		Userid: c.Get(gate.CtxUserid).(string),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.GetByID(req.Userid)
	if err != nil {
		c.WriteError(err)
		return
	}

	c.WriteJSON(http.StatusOK, res)
}

func (m *router) getByIDPrivate(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqUserGetID{
		Userid: c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.GetByID(req.Userid)
	if err != nil {
		c.WriteError(err)
		return
	}

	c.WriteJSON(http.StatusOK, res)
}

type (
	reqUserGetUsername struct {
		Username string `valid:"username,has" json:"-"`
	}
)

func (m *router) getByUsername(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqUserGetUsername{
		Username: c.Param("username"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.GetByUsernamePublic(req.Username)
	if err != nil {
		c.WriteError(err)
		return
	}

	c.WriteJSON(http.StatusOK, res)
}

func (m *router) getByUsernamePrivate(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqUserGetUsername{
		Username: c.Param("username"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.GetByUsername(req.Username)
	if err != nil {
		c.WriteError(err)
		return
	}

	c.WriteJSON(http.StatusOK, res)
}

type (
	reqGetRoleUser struct {
		Role   string `valid:"role,has" json:"-"`
		Amount int    `valid:"amount" json:"-"`
		Offset int    `valid:"offset" json:"-"`
	}
)

func (m *router) getUsersByRole(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	amount, err := strconv.Atoi(c.Query("amount"))
	if err != nil {
		c.WriteError(governor.NewErrorUser("amount invalid", http.StatusBadRequest, nil))
		return
	}
	offset, err := strconv.Atoi(c.Query("offset"))
	if err != nil {
		c.WriteError(governor.NewErrorUser("offset invalid", http.StatusBadRequest, nil))
		return
	}

	req := reqGetRoleUser{
		Role:   c.Param("role"),
		Amount: amount,
		Offset: offset,
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.GetIDsByRole(req.Role, req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}

	if len(res.Users) == 0 {
		c.WriteStatus(http.StatusNotFound)
		return
	}

	c.WriteJSON(http.StatusOK, res)
}

type (
	reqGetUserBulk struct {
		Amount int `valid:"amount" json:"-"`
		Offset int `valid:"offset" json:"-"`
	}
)

func (m *router) getAllUserInfo(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	amount, err := strconv.Atoi(c.Query("amount"))
	if err != nil {
		c.WriteError(governor.NewErrorUser("amount invalid", http.StatusBadRequest, nil))
		return
	}
	offset, err := strconv.Atoi(c.Query("offset"))
	if err != nil {
		c.WriteError(governor.NewErrorUser("offset invalid", http.StatusBadRequest, nil))
		return
	}

	req := reqGetUserBulk{
		Amount: amount,
		Offset: offset,
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.GetInfoAll(req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}

	if len(res.Users) == 0 {
		c.WriteStatus(http.StatusNotFound)
		return
	}

	c.WriteJSON(http.StatusOK, res)
}

type (
	reqGetUsers struct {
		Userids string `valid:"userids,has" json:"-"`
	}
)

func (m *router) getUserInfoBulkPublic(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqGetUsers{
		Userids: c.Query("ids"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.GetInfoBulkPublic(strings.Split(req.Userids, ","))
	if err != nil {
		c.WriteError(err)
		return
	}

	if len(res.Users) == 0 {
		c.WriteStatus(http.StatusNotFound)
		return
	}

	c.WriteJSON(http.StatusOK, res)
}

func (m *router) mountGet(r governor.Router) {
	r.Get("/id/{id}", m.getByID)
	r.Get("", m.getByIDPersonal, gate.User(m.s.gate))
	r.Get("/id/{id}/private", m.getByIDPrivate, gate.Admin(m.s.gate))
	r.Get("/name/{username}", m.getByUsername)
	r.Get("/name/{username}/private", m.getByUsernamePrivate, gate.Admin(m.s.gate))
	r.Get("/role/{role}", m.getUsersByRole)
	r.Get("/all", m.getAllUserInfo, gate.Admin(m.s.gate))
	r.Get("/ids", m.getUserInfoBulkPublic)
}
