package user

import (
	"net/http"
	"strings"

	"xorkevin.dev/governor"
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
	k := strings.SplitN(r.Keyid, "|", 2)
	if len(k) != 2 || r.Userid != k[0] {
		return governor.NewErrorUser("Invalid apikey id", http.StatusForbidden, nil)
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
	k := strings.SplitN(r.Keyid, "|", 2)
	if len(k) != 2 || r.Userid != k[0] {
		return governor.NewErrorUser("Invalid apikey id", http.StatusForbidden, nil)
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

func (m *router) checkApikeyValidator(t gate.Intersector) bool {
	c := t.Ctx()
	req := reqApikeyCheck{
		Roles: c.Query("roles"),
		Scope: c.Query("scope"),
	}
	if err := req.valid(); err != nil {
		return false
	}

	if !t.HasScope(req.Scope) {
		return false
	}
	expected, err := rank.FromString(req.Roles)
	if err != nil {
		return false
	}
	roles, ok := t.Intersect(expected)
	if !ok {
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

const (
	scopeApikeyRead  = "gov.user.apikey:read"
	scopeApikeyWrite = "gov.user.apikey:write"
)

func (m *router) mountApikey(r governor.Router) {
	r.Get("", m.getUserApikeys, gate.User(m.s.gate, scopeApikeyRead))
	r.Post("", m.createApikey, gate.User(m.s.gate, scopeApikeyWrite))
	r.Put("/id/{id}", m.updateApikey, gate.User(m.s.gate, scopeApikeyWrite))
	r.Put("/id/{id}/rotate", m.rotateApikey, gate.User(m.s.gate, scopeApikeyWrite))
	r.Delete("/id/{id}", m.deleteApikey, gate.User(m.s.gate, scopeApikeyWrite))
	r.Any("/check", m.checkApikey, m.s.gate.Authenticate(m.checkApikeyValidator, ""))
}
