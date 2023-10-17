package user

import (
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/gate"
)

type (
	//forge:valid
	reqGetUserApikeys struct {
		Userid string `valid:"userid,has" json:"-"`
		Amount int    `valid:"amount" json:"-"`
		Offset int    `valid:"offset" json:"-"`
	}
)

func (s *router) getUserApikeys(c *governor.Context) {
	req := reqGetUserApikeys{
		Userid: gate.GetCtxUserid(c),
		Amount: c.QueryInt("amount", -1),
		Offset: c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.getUserApikeys(c.Ctx(), req.Userid, req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqApikeyPost struct {
		Userid string `valid:"userid,has" json:"-"`
		Scope  string `valid:"scope" json:"scope"`
		Name   string `valid:"apikeyName" json:"name"`
		Desc   string `valid:"apikeyDesc" json:"desc"`
	}
)

func (s *router) createApikey(c *governor.Context) {
	var req reqApikeyPost
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.createApikey(c.Ctx(), req.Userid, req.Scope, req.Name, req.Desc)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusCreated, res)
}

type (
	//forge:valid
	reqApikeyID struct {
		Userid string `valid:"userid,has" json:"-"`
		Keyid  string `valid:"apikeyid,has" json:"-"`
	}
)

func (s *router) deleteApikeySelf(c *governor.Context) {
	req := reqApikeyID{
		Userid: gate.GetCtxUserid(c),
		Keyid:  gate.GetCtxApikeyID(c),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := s.s.deleteApikey(c.Ctx(), req.Userid, req.Keyid); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (s *router) deleteApikey(c *governor.Context) {
	req := reqApikeyID{
		Userid: gate.GetCtxUserid(c),
		Keyid:  c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := s.s.deleteApikey(c.Ctx(), req.Userid, req.Keyid); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	//forge:valid
	reqApikeyUpdate struct {
		Userid string `valid:"userid,has" json:"-"`
		Keyid  string `valid:"apikeyid,has" json:"-"`
		Scope  string `valid:"scope" json:"scope"`
		Name   string `valid:"apikeyName" json:"name"`
		Desc   string `valid:"apikeyDesc" json:"desc"`
	}
)

func (s *router) updateApikey(c *governor.Context) {
	var req reqApikeyUpdate
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	req.Keyid = c.Param("id")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := s.s.updateApikey(c.Ctx(), req.Userid, req.Keyid, req.Scope, req.Name, req.Desc); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (s *router) rotateApikeySelf(c *governor.Context) {
	req := reqApikeyID{
		Userid: gate.GetCtxUserid(c),
		Keyid:  gate.GetCtxApikeyID(c),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.rotateApikey(c.Ctx(), req.Userid, req.Keyid)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (s *router) rotateApikey(c *governor.Context) {
	req := reqApikeyID{
		Userid: gate.GetCtxUserid(c),
		Keyid:  c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.rotateApikey(c.Ctx(), req.Userid, req.Keyid)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	resApikeyOK struct {
		Message string `json:"message"`
	}
)

func (s *router) checkApikey(c *governor.Context) {
	c.WriteJSON(http.StatusOK, resApikeyOK{
		Message: "OK",
	})
}

func (s *router) mountApikey(r governor.Router) {
	m := governor.NewMethodRouter(r)
	scopeApikeyRead := s.s.scopens + ".apikey:read"
	scopeApikeyWrite := s.s.scopens + ".apikey:write"
	m.GetCtx("", s.getUserApikeys, gate.AuthUser(s.s.gate, scopeApikeyRead), s.rt)
	m.PostCtx("", s.createApikey, gate.AuthUserSudo(s.s.gate, s.s.authSettings.sudoDuration, gate.ScopeNone), s.rt)
	m.PutCtx("/rotate", s.rotateApikeySelf, gate.AuthUser(s.s.gate, ""), s.rt)
	m.DeleteCtx("/discard", s.deleteApikeySelf, gate.AuthUser(s.s.gate, ""), s.rt)
	m.PutCtx("/id/{id}", s.updateApikey, gate.AuthUserSudo(s.s.gate, s.s.authSettings.sudoDuration, gate.ScopeNone), s.rt)
	m.PutCtx("/id/{id}/rotate", s.rotateApikey, gate.AuthUser(s.s.gate, scopeApikeyWrite), s.rt)
	m.DeleteCtx("/id/{id}", s.deleteApikey, gate.AuthUser(s.s.gate, scopeApikeyWrite), s.rt)
	m.AnyCtx("/check", s.checkApikey, gate.AuthUser(s.s.gate, ""), s.rt)
}
