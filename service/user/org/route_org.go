package org

import (
	"net/http"
	"strings"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/service/user/token"
	"xorkevin.dev/governor/util/rank"
)

type (
	//forge:valid
	reqOrgGet struct {
		OrgID string `valid:"orgid,has" json:"-"`
	}

	//forge:valid
	reqOrgNameGet struct {
		Name string `valid:"name,has" json:"-"`
	}

	//forge:valid
	reqOrgsGet struct {
		OrgIDs []string `valid:"orgids,has" json:"-"`
	}

	//forge:valid
	reqOrgMembersSearch struct {
		OrgID  string `valid:"orgid,has" json:"-"`
		Mods   bool   `json:"-"`
		Prefix string `valid:"username,opt" json:"-"`
		Amount int    `valid:"amount" json:"-"`
		Offset int    `valid:"offset" json:"-"`
	}

	//forge:valid
	reqOrgsSearch struct {
		Userid string `valid:"userid,has" json:"-"`
		Mods   bool   `json:"-"`
		Prefix string `valid:"name,opt" json:"-"`
		Amount int    `valid:"amount" json:"-"`
		Offset int    `valid:"offset" json:"-"`
	}

	//forge:valid
	reqOrgsGetBulk struct {
		Amount int `valid:"amount" json:"-"`
		Offset int `valid:"offset" json:"-"`
	}
)

func (s *router) getOrg(c governor.Context) {
	req := reqOrgGet{
		OrgID: c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := s.s.GetByID(c.Ctx(), req.OrgID)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (s *router) getOrgByName(c governor.Context) {
	req := reqOrgNameGet{
		Name: c.Param("name"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := s.s.GetByName(c.Ctx(), req.Name)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (s *router) getOrgs(c governor.Context) {
	req := reqOrgsGet{
		OrgIDs: strings.Split(c.Query("ids"), ","),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := s.s.getOrgs(c.Ctx(), req.OrgIDs)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (s *router) getOrgMembers(c governor.Context) {
	req := reqOrgMembersSearch{
		OrgID:  c.Param("id"),
		Mods:   c.QueryBool("mod"),
		Prefix: c.Query("prefix"),
		Amount: c.QueryInt("amount", -1),
		Offset: c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if req.Mods {
		res, err := s.s.getOrgMods(c.Ctx(), req.OrgID, req.Prefix, req.Amount, req.Offset)
		if err != nil {
			c.WriteError(err)
			return
		}
		c.WriteJSON(http.StatusOK, res)
	} else {
		res, err := s.s.getOrgMembers(c.Ctx(), req.OrgID, req.Prefix, req.Amount, req.Offset)
		if err != nil {
			c.WriteError(err)
			return
		}
		c.WriteJSON(http.StatusOK, res)
	}
}

func (s *router) getUserOrgs(c governor.Context) {
	req := reqOrgsSearch{
		Userid: gate.GetCtxUserid(c),
		Mods:   c.QueryBool("mod"),
		Prefix: c.Query("prefix"),
		Amount: c.QueryInt("amount", -1),
		Offset: c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if req.Mods {
		res, err := s.s.getUserMods(c.Ctx(), req.Userid, req.Prefix, req.Amount, req.Offset)
		if err != nil {
			c.WriteError(err)
			return
		}
		c.WriteJSON(http.StatusOK, res)
	} else {
		res, err := s.s.getUserOrgs(c.Ctx(), req.Userid, req.Prefix, req.Amount, req.Offset)
		if err != nil {
			c.WriteError(err)
			return
		}
		c.WriteJSON(http.StatusOK, res)
	}
}

func (s *router) getAllOrgs(c governor.Context) {
	req := reqOrgsGetBulk{
		Amount: c.QueryInt("amount", -1),
		Offset: c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := s.s.getAllOrgs(c.Ctx(), req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqOrgPost struct {
		Display string `valid:"display" json:"display_name"`
		Desc    string `valid:"desc" json:"desc"`
		Userid  string `valid:"userid,has" json:"-"`
	}
)

func (s *router) createOrg(c governor.Context) {
	var req reqOrgPost
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := s.s.createOrg(c.Ctx(), req.Userid, req.Display, req.Desc)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusCreated, res)
}

type (
	//forge:valid
	reqOrgPut struct {
		OrgID   string `valid:"orgid,has" json:"-"`
		Name    string `valid:"name" json:"name"`
		Display string `valid:"display" json:"display_name"`
		Desc    string `valid:"desc" json:"desc"`
	}
)

func (s *router) updateOrg(c governor.Context) {
	var req reqOrgPut
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	req.OrgID = c.Param("id")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := s.s.updateOrg(c.Ctx(), req.OrgID, req.Name, req.Display, req.Desc); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (s *router) deleteOrg(c governor.Context) {
	req := reqOrgGet{
		OrgID: c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := s.s.deleteOrg(c.Ctx(), req.OrgID); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (s *router) orgMember(c governor.Context, _ string) (string, bool, bool) {
	orgid := c.Param("id")
	if err := validhasOrgid(orgid); err != nil {
		return "", false, false
	}
	return rank.ToOrgName(orgid), false, true
}

func (s *router) mountRoute(r governor.Router) {
	m := governor.NewMethodRouter(r)
	scopeOrgRead := s.s.scopens + ":read"
	scopeOrgWrite := s.s.scopens + ":write"
	m.GetCtx("/id/{id}", s.getOrg, s.rt)
	m.GetCtx("/name/{name}", s.getOrgByName)
	m.GetCtx("/ids", s.getOrgs, s.rt)
	m.GetCtx("/id/{id}/member", s.getOrgMembers, s.rt)
	m.GetCtx("/search", s.getUserOrgs, gate.User(s.s.gate, scopeOrgRead), s.rt)
	m.GetCtx("", s.getAllOrgs, s.rt)
	m.PostCtx("", s.createOrg, gate.User(s.s.gate, token.ScopeForbidden), s.rt)
	m.PutCtx("/id/{id}", s.updateOrg, gate.ModF(s.s.gate, s.orgMember, scopeOrgWrite), s.rt)
	m.DeleteCtx("/id/{id}", s.deleteOrg, gate.ModF(s.s.gate, s.orgMember, token.ScopeForbidden), s.rt)
}
