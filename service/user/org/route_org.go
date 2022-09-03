package org

import (
	"net/http"
	"strings"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/service/user/token"
	"xorkevin.dev/governor/util/rank"
)

//go:generate forge validation -o validation_org_gen.go reqOrgGet reqOrgNameGet reqOrgsGet reqOrgMembersSearch reqOrgsSearch reqOrgsGetBulk reqOrgPost reqOrgPut

type (
	reqOrgGet struct {
		OrgID string `valid:"orgid,has" json:"-"`
	}

	reqOrgNameGet struct {
		Name string `valid:"name,has" json:"-"`
	}

	reqOrgsGet struct {
		OrgIDs []string `valid:"orgids,has" json:"-"`
	}

	reqOrgMembersSearch struct {
		OrgID  string `valid:"orgid,has" json:"-"`
		Mods   bool   `json:"-"`
		Prefix string `valid:"username,opt" json:"-"`
		Amount int    `valid:"amount" json:"-"`
		Offset int    `valid:"offset" json:"-"`
	}

	reqOrgsSearch struct {
		Userid string `valid:"userid,has" json:"-"`
		Mods   bool   `json:"-"`
		Prefix string `valid:"name,opt" json:"-"`
		Amount int    `valid:"amount" json:"-"`
		Offset int    `valid:"offset" json:"-"`
	}

	reqOrgsGetBulk struct {
		Amount int `valid:"amount" json:"-"`
		Offset int `valid:"offset" json:"-"`
	}
)

func (m *router) getOrg(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqOrgGet{
		OrgID: c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.GetByID(c.Ctx(), req.OrgID)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (m *router) getOrgByName(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqOrgNameGet{
		Name: c.Param("name"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.GetByName(c.Ctx(), req.Name)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (m *router) getOrgs(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqOrgsGet{
		OrgIDs: strings.Split(c.Query("ids"), ","),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.GetOrgs(c.Ctx(), req.OrgIDs)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (m *router) getOrgMembers(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
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
		res, err := m.s.GetOrgMods(c.Ctx(), req.OrgID, req.Prefix, req.Amount, req.Offset)
		if err != nil {
			c.WriteError(err)
			return
		}
		c.WriteJSON(http.StatusOK, res)
	} else {
		res, err := m.s.GetOrgMembers(c.Ctx(), req.OrgID, req.Prefix, req.Amount, req.Offset)
		if err != nil {
			c.WriteError(err)
			return
		}
		c.WriteJSON(http.StatusOK, res)
	}
}

func (m *router) getUserOrgs(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
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
		res, err := m.s.GetUserMods(c.Ctx(), req.Userid, req.Prefix, req.Amount, req.Offset)
		if err != nil {
			c.WriteError(err)
			return
		}
		c.WriteJSON(http.StatusOK, res)
	} else {
		res, err := m.s.GetUserOrgs(c.Ctx(), req.Userid, req.Prefix, req.Amount, req.Offset)
		if err != nil {
			c.WriteError(err)
			return
		}
		c.WriteJSON(http.StatusOK, res)
	}
}

func (m *router) getAllOrgs(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqOrgsGetBulk{
		Amount: c.QueryInt("amount", -1),
		Offset: c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.GetAllOrgs(c.Ctx(), req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	reqOrgPost struct {
		Display string `valid:"display" json:"display_name"`
		Desc    string `valid:"desc" json:"desc"`
		Userid  string `valid:"userid,has" json:"-"`
	}
)

func (m *router) createOrg(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqOrgPost{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.CreateOrg(c.Ctx(), req.Userid, req.Display, req.Desc)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusCreated, res)
}

type (
	reqOrgPut struct {
		OrgID   string `valid:"orgid,has" json:"-"`
		Name    string `valid:"name" json:"name"`
		Display string `valid:"display" json:"display_name"`
		Desc    string `valid:"desc" json:"desc"`
	}
)

func (m *router) updateOrg(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqOrgPut{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	req.OrgID = c.Param("id")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := m.s.UpdateOrg(c.Ctx(), req.OrgID, req.Name, req.Display, req.Desc); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (m *router) deleteOrg(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqOrgGet{
		OrgID: c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := m.s.DeleteOrg(c.Ctx(), req.OrgID); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (m *router) orgMember(c governor.Context, _ string) (string, bool, bool) {
	orgid := c.Param("id")
	if err := validhasOrgid(orgid); err != nil {
		return "", false, false
	}
	return rank.ToOrgName(orgid), false, true
}

func (m *router) mountRoute(r governor.Router) {
	scopeOrgRead := m.s.scopens + ":read"
	scopeOrgWrite := m.s.scopens + ":write"
	r.Get("/id/{id}", m.getOrg, m.rt)
	r.Get("/name/{name}", m.getOrgByName)
	r.Get("/ids", m.getOrgs, m.rt)
	r.Get("/id/{id}/member", m.getOrgMembers, m.rt)
	r.Get("/search", m.getUserOrgs, gate.User(m.s.gate, scopeOrgRead), m.rt)
	r.Get("", m.getAllOrgs, m.rt)
	r.Post("", m.createOrg, gate.User(m.s.gate, token.ScopeForbidden), m.rt)
	r.Put("/id/{id}", m.updateOrg, gate.ModF(m.s.gate, m.orgMember, scopeOrgWrite), m.rt)
	r.Delete("/id/{id}", m.deleteOrg, gate.ModF(m.s.gate, m.orgMember, token.ScopeForbidden), m.rt)
}
