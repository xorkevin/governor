package user

import (
	"net/http"
	"strings"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/util/rank"
)

//go:generate forge validation -o validation_get_gen.go reqUserGetID reqUserGetUsername reqGetUserRoles reqGetUserRolesIntersect reqGetRoleUser reqGetUserBulk reqGetUsers

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
		Userid: gate.GetCtxUserid(c),
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
	reqGetUserRoles struct {
		Userid string `valid:"userid,has" json:"-"`
		Prefix string `valid:"rolePrefix,has" json:"-"`
		Amount int    `valid:"amount" json:"-"`
		Offset int    `valid:"offset" json:"-"`
	}
)

func (m *router) getUserRoles(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqGetUserRoles{
		Userid: c.Param("id"),
		Prefix: c.Query("prefix"),
		Amount: c.QueryInt("amount", -1),
		Offset: c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.GetUserRoles(req.Userid, req.Prefix, req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}

	c.WriteJSON(http.StatusOK, res)
}

func (m *router) getUserRolesPersonal(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqGetUserRoles{
		Userid: gate.GetCtxUserid(c),
		Prefix: c.Query("prefix"),
		Amount: c.QueryInt("amount", -1),
		Offset: c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.GetUserRoles(req.Userid, req.Prefix, req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}

	c.WriteJSON(http.StatusOK, res)
}

type (
	reqGetUserRolesIntersect struct {
		Userid string `valid:"userid,has" json:"-"`
		Roles  string `valid:"rankStr" json:"-"`
	}
)

func (m *router) getUserRolesIntersect(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqGetUserRolesIntersect{
		Userid: c.Param("id"),
		Roles:  c.Query("roles"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	roles, err := rank.FromString(req.Roles)
	if err != nil {
		c.WriteError(err)
		return
	}
	res, err := m.s.GetUserRolesIntersect(req.Userid, roles)
	if err != nil {
		c.WriteError(err)
		return
	}

	c.WriteJSON(http.StatusOK, res)
}

func (m *router) getUserRolesIntersectPersonal(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqGetUserRolesIntersect{
		Userid: gate.GetCtxUserid(c),
		Roles:  c.Query("roles"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	roles, err := rank.FromString(req.Roles)
	if err != nil {
		c.WriteError(err)
		return
	}
	res, err := m.s.GetUserRolesIntersect(req.Userid, roles)
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
	req := reqGetRoleUser{
		Role:   c.Param("role"),
		Amount: c.QueryInt("amount", -1),
		Offset: c.QueryInt("offset", -1),
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
	req := reqGetUserBulk{
		Amount: c.QueryInt("amount", -1),
		Offset: c.QueryInt("offset", -1),
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
	c.WriteJSON(http.StatusOK, res)
}

const (
	scopeAccountRead = "gov.user.account:read"
	scopeAdminRead   = "gov.user.admin:read"
)

func (m *router) mountGet(r governor.Router) {
	r.Get("/id/{id}", m.getByID)
	r.Get("", m.getByIDPersonal, gate.User(m.s.gate, scopeAccountRead))
	r.Get("/roles", m.getUserRolesPersonal, gate.User(m.s.gate, scopeAccountRead))
	r.Get("/roleint", m.getUserRolesIntersectPersonal, gate.User(m.s.gate, scopeAccountRead))
	r.Get("/id/{id}/private", m.getByIDPrivate, gate.Admin(m.s.gate, scopeAdminRead))
	r.Get("/id/{id}/roles", m.getUserRoles)
	r.Get("/id/{id}/roleint", m.getUserRolesIntersect)
	r.Get("/name/{username}", m.getByUsername)
	r.Get("/name/{username}/private", m.getByUsernamePrivate, gate.Admin(m.s.gate, scopeAdminRead))
	r.Get("/role/{role}", m.getUsersByRole)
	r.Get("/all", m.getAllUserInfo, gate.Admin(m.s.gate, scopeAdminRead))
	r.Get("/ids", m.getUserInfoBulkPublic)
}
