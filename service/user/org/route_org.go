package org

import (
	"net/http"
	"strings"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/util/rank"
)

//go:generate forge validation -o validation_org_gen.go reqOrgGet reqOrgNameGet reqOrgsGet reqOrgsGetBulk reqOrgPost reqOrgPut

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

	res, err := m.s.GetByID(req.OrgID)
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

	res, err := m.s.GetByName(req.Name)
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

	res, err := m.s.GetOrgs(req.OrgIDs)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
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

	res, err := m.s.GetAllOrgs(req.Amount, req.Offset)
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

	res, err := m.s.CreateOrg(req.Userid, req.Display, req.Desc)
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

	if err := m.s.UpdateOrg(req.OrgID, req.Name, req.Display, req.Desc); err != nil {
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

	if err := m.s.DeleteOrg(req.OrgID); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (m *router) orgMember(c governor.Context, _ string) (string, error) {
	orgid := c.Param("id")
	if err := validhasOrgid(orgid); err != nil {
		return "", err
	}
	return rank.ToOrgName(orgid), nil
}

const (
	// scopeOrgRead  = "gov.user.org:read"

	scopeOrgWrite = "gov.user.org:write"
)

func (m *router) mountRoute(r governor.Router) {
	r.Get("/id/{id}", m.getOrg)
	r.Get("/name/{name}", m.getOrgByName)
	r.Get("/ids", m.getOrgs)
	r.Get("", m.getAllOrgs)
	r.Post("", m.createOrg, gate.User(m.s.gate, scopeOrgWrite))
	r.Put("/id/{id}", m.updateOrg, gate.ModF(m.s.gate, m.orgMember, scopeOrgWrite))
	r.Delete("/id/{id}", m.deleteOrg, gate.ModF(m.s.gate, m.orgMember, scopeOrgWrite))
}
