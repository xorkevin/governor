package user

import (
	"net/http"

	"xorkevin.dev/governor"
	apikeymodel "xorkevin.dev/governor/service/user/apikey/model"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/util/rank"
)

//go:generate forge validation -o validation_apikey_gen.go reqGetUserApikeys reqApikeyPost reqApikeyID reqApikeyUpdate reqApikeyCheck

type (
	reqGetUserApikeys struct {
		Userid string `valid:"userid,has" json:"-"`
		Amount int    `valid:"amount" json:"-"`
		Offset int    `valid:"offset" json:"-"`
	}
)

func (m *router) getUserApikeys(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqGetUserApikeys{
		Userid: gate.GetCtxUserid(c),
		Amount: c.QueryInt("amount", -1),
		Offset: c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := m.s.GetUserApikeys(req.Userid, req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	reqApikeyPost struct {
		Userid string `valid:"userid,has" json:"-"`
		Scope  string `valid:"scope" json:"scope"`
		Name   string `valid:"apikeyName" json:"name"`
		Desc   string `valid:"apikeyDesc" json:"desc"`
	}
)

func (m *router) createApikey(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqApikeyPost{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := m.s.CreateApikey(req.Userid, req.Scope, req.Name, req.Desc)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusCreated, res)
}

type (
	reqApikeyID struct {
		Userid string `valid:"userid,has" json:"-"`
		Keyid  string `valid:"apikeyid,has" json:"-"`
	}
)

func (r *reqApikeyID) validUserid() error {
	userid, err := apikeymodel.ParseIDUserid(r.Keyid)
	if err != nil || r.Userid != userid {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Invalid apikey id",
		}))
	}
	return nil
}

func (m *router) deleteApikey(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqApikeyID{
		Userid: gate.GetCtxUserid(c),
		Keyid:  c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := req.validUserid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := m.s.DeleteApikey(req.Keyid); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	reqApikeyUpdate struct {
		Userid string `valid:"userid,has" json:"-"`
		Keyid  string `valid:"apikeyid,has" json:"-"`
		Scope  string `valid:"scope" json:"scope"`
		Name   string `valid:"apikeyName" json:"name"`
		Desc   string `valid:"apikeyDesc" json:"desc"`
	}
)

func (r *reqApikeyUpdate) validUserid() error {
	userid, err := apikeymodel.ParseIDUserid(r.Keyid)
	if err != nil || r.Userid != userid {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Invalid apikey id",
		}))
	}
	return nil
}

func (m *router) updateApikey(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqApikeyUpdate{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	req.Keyid = c.Param("id")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := req.validUserid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := m.s.UpdateApikey(req.Keyid, req.Scope, req.Name, req.Desc); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (m *router) rotateApikey(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqApikeyID{
		Userid: gate.GetCtxUserid(c),
		Keyid:  c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := req.validUserid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := m.s.RotateApikey(req.Keyid)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	reqApikeyCheck struct {
		Roles string `valid:"rankStr"`
		Scope string
	}
)

func (m *router) checkApikeyValidator(c gate.Context) bool {
	req := reqApikeyCheck{
		Roles: c.Ctx().Query("roles"),
		Scope: c.Ctx().Query("scope"),
	}
	if err := req.valid(); err != nil {
		return false
	}

	if !c.HasScope(req.Scope) {
		return false
	}
	expected, err := rank.FromString(req.Roles)
	if err != nil {
		return false
	}
	roles, err := c.Intersect(expected)
	if err != nil {
		return false
	}
	if roles.Len() != expected.Len() {
		return false
	}
	return true
}

type (
	resApikeyOK struct {
		Message string `json:"message"`
	}
)

func (m *router) checkApikey(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	c.WriteJSON(http.StatusOK, resApikeyOK{
		Message: "OK",
	})
}

func (m *router) mountApikey(r governor.Router) {
	scopeApikeyRead := m.s.scopens + ".apikey:read"
	scopeApikeyWrite := m.s.scopens + ".apikey:write"
	r.Get("", m.getUserApikeys, gate.User(m.s.gate, scopeApikeyRead), m.rt)
	r.Post("", m.createApikey, gate.User(m.s.gate, scopeApikeyWrite), m.rt)
	r.Put("/id/{id}", m.updateApikey, gate.User(m.s.gate, scopeApikeyWrite), m.rt)
	r.Put("/id/{id}/rotate", m.rotateApikey, gate.User(m.s.gate, scopeApikeyWrite), m.rt)
	r.Delete("/id/{id}", m.deleteApikey, gate.User(m.s.gate, scopeApikeyWrite), m.rt)
	r.Any("/check", m.checkApikey, m.s.gate.Authenticate(m.checkApikeyValidator, ""), m.rt)
}
