package user

import (
	"errors"
	"net/http"
	"strings"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/util/rank"
)

type (
	//forge:valid
	reqUserGetID struct {
		Userid string `valid:"userid,has" json:"-"`
	}
)

func (s *router) getByID(c governor.Context) {
	req := reqUserGetID{
		Userid: c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := s.s.getByIDPublic(c.Ctx(), req.Userid)
	if err != nil {
		c.WriteError(err)
		return
	}

	c.WriteJSON(http.StatusOK, res)
}

func (s *router) getByIDPersonal(c governor.Context) {
	req := reqUserGetID{
		Userid: gate.GetCtxUserid(c),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := s.s.GetByID(c.Ctx(), req.Userid)
	if err != nil {
		c.WriteError(err)
		return
	}

	c.WriteJSON(http.StatusOK, res)
}

func (s *router) getByIDPrivate(c governor.Context) {
	req := reqUserGetID{
		Userid: c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := s.s.GetByID(c.Ctx(), req.Userid)
	if err != nil {
		c.WriteError(err)
		return
	}

	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqUserGetUsername struct {
		Username string `valid:"username,has" json:"-"`
	}
)

func (s *router) getByUsername(c governor.Context) {
	req := reqUserGetUsername{
		Username: c.Param("username"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := s.s.getByUsernamePublic(c.Ctx(), req.Username)
	if err != nil {
		c.WriteError(err)
		return
	}

	c.WriteJSON(http.StatusOK, res)
}

func (s *router) getByUsernamePrivate(c governor.Context) {
	req := reqUserGetUsername{
		Username: c.Param("username"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := s.s.GetByUsername(c.Ctx(), req.Username)
	if err != nil {
		c.WriteError(err)
		return
	}

	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqGetUserRoles struct {
		Userid string `valid:"userid,has" json:"-"`
		Prefix string `valid:"rolePrefix,has" json:"-"`
		Amount int    `valid:"amount" json:"-"`
		Offset int    `valid:"offset" json:"-"`
	}
)

func (s *router) getUserRoles(c governor.Context) {
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

	res, err := s.s.getUserRoles(c.Ctx(), req.Userid, req.Prefix, req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}

	c.WriteJSON(http.StatusOK, res)
}

func (s *router) getUserRolesPersonal(c governor.Context) {
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

	res, err := s.s.getUserRoles(c.Ctx(), req.Userid, req.Prefix, req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}

	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqGetUserRolesIntersect struct {
		Userid string   `valid:"userid,has" json:"-"`
		Roles  []string `valid:"rank" json:"-"`
	}
)

func (s *router) getUserRolesIntersect(c governor.Context) {
	req := reqGetUserRolesIntersect{
		Userid: c.Param("id"),
		Roles:  rank.SplitString(c.Query("roles")),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	roles, err := rank.FromSlice(req.Roles)
	if err != nil {
		if errors.Is(err, rank.ErrorInvalidRank{}) {
			c.WriteError(governor.ErrWithRes(err, http.StatusBadRequest, "", "Invalid rank string"))
			return
		}
		c.WriteError(err)
		return
	}
	res, err := s.s.getUserRolesIntersect(c.Ctx(), req.Userid, roles)
	if err != nil {
		c.WriteError(err)
		return
	}

	c.WriteJSON(http.StatusOK, res)
}

func (s *router) getUserRolesIntersectPersonal(c governor.Context) {
	req := reqGetUserRolesIntersect{
		Userid: gate.GetCtxUserid(c),
		Roles:  rank.SplitString(c.Query("roles")),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	roles, err := rank.FromSlice(req.Roles)
	if err != nil {
		if errors.Is(err, rank.ErrorInvalidRank{}) {
			c.WriteError(governor.ErrWithRes(err, http.StatusBadRequest, "", "Invalid rank string"))
			return
		}
		c.WriteError(err)
		return
	}
	res, err := s.s.getUserRolesIntersect(c.Ctx(), req.Userid, roles)
	if err != nil {
		c.WriteError(err)
		return
	}

	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqGetRoleUser struct {
		Role   string `valid:"role,has" json:"-"`
		Amount int    `valid:"amount" json:"-"`
		Offset int    `valid:"offset" json:"-"`
	}
)

func (s *router) getUsersByRole(c governor.Context) {
	req := reqGetRoleUser{
		Role:   c.Param("role"),
		Amount: c.QueryInt("amount", -1),
		Offset: c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := s.s.getIDsByRole(c.Ctx(), req.Role, req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqGetUserBulk struct {
		Amount int `valid:"amount" json:"-"`
		Offset int `valid:"offset" json:"-"`
	}
)

func (s *router) getAllUserInfo(c governor.Context) {
	req := reqGetUserBulk{
		Amount: c.QueryInt("amount", -1),
		Offset: c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := s.s.getInfoAll(c.Ctx(), req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqGetUsers struct {
		Userids []string `valid:"userids,has" json:"-"`
	}
)

func (s *router) getUserInfoBulkPublic(c governor.Context) {
	req := reqGetUsers{
		Userids: strings.Split(c.Query("ids"), ","),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := s.s.getInfoBulkPublic(c.Ctx(), req.Userids)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqSearchUsers struct {
		Prefix string `valid:"username,opt" json:"-"`
		Amount int    `valid:"amount" json:"-"`
	}
)

func (s *router) searchUsers(c governor.Context) {
	req := reqSearchUsers{
		Prefix: c.Query("prefix"),
		Amount: c.QueryInt("amount", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := s.s.getInfoUsernamePrefix(c.Ctx(), req.Prefix, req.Amount)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (s *router) mountGet(m *governor.MethodRouter) {
	scopeAccountRead := s.s.scopens + ".account:read"
	scopeAdminRead := s.s.scopens + ".admin:read"
	m.GetCtx("/id/{id}", s.getByID, s.rt)
	m.GetCtx("", s.getByIDPersonal, gate.User(s.s.gate, scopeAccountRead), s.rt)
	m.GetCtx("/roles", s.getUserRolesPersonal, gate.User(s.s.gate, scopeAccountRead), s.rt)
	m.GetCtx("/roleint", s.getUserRolesIntersectPersonal, gate.User(s.s.gate, scopeAccountRead), s.rt)
	m.GetCtx("/id/{id}/private", s.getByIDPrivate, gate.Admin(s.s.gate, scopeAdminRead), s.rt)
	m.GetCtx("/id/{id}/roles", s.getUserRoles, s.rt)
	m.GetCtx("/id/{id}/roleint", s.getUserRolesIntersect, s.rt)
	m.GetCtx("/name/{username}", s.getByUsername, s.rt)
	m.GetCtx("/name/{username}/private", s.getByUsernamePrivate, gate.Admin(s.s.gate, scopeAdminRead), s.rt)
	m.GetCtx("/role/{role}", s.getUsersByRole, s.rt)
	m.GetCtx("/all", s.getAllUserInfo, gate.Admin(s.s.gate, scopeAdminRead), s.rt)
	m.GetCtx("/ids", s.getUserInfoBulkPublic, s.rt)
	m.GetCtx("/search", s.searchUsers, s.rt)
}
