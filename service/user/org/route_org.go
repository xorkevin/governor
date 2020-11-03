package org

import (
	"net/http"
	"strconv"
	"strings"
	"xorkevin.dev/governor"
)

//go:generate forge validation -o validation_org_gen.go reqOrgGet reqOrgNameGet reqOrgsGet reqOrgsGetBulk

type (
	reqOrgGet struct {
		OrgID string `valid:"orgID,has" json:"-"`
	}

	reqOrgNameGet struct {
		Name string `valid:"orgName,has" json:"-"`
	}

	reqOrgsGet struct {
		OrgIDs string `valid:"orgIDs,has" json:"-"`
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
		OrgIDs: c.Query("ids"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.GetOrgs(strings.Split(req.OrgIDs, ","))
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (m *router) getAllOrgs(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	amount, err := strconv.Atoi(c.Query("amount"))
	if err != nil {
		c.WriteError(governor.NewErrorUser("Amount invalid", http.StatusBadRequest, nil))
		return
	}
	offset, err := strconv.Atoi(c.Query("offset"))
	if err != nil {
		c.WriteError(governor.NewErrorUser("Offset invalid", http.StatusBadRequest, nil))
		return
	}

	req := reqOrgsGetBulk{
		Amount: amount,
		Offset: offset,
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

func (m *router) mountRoute(r governor.Router) {
	r.Get("/id/{id}", m.getOrg)
	r.Get("/name/{name}", m.getOrgByName)
	r.Get("/ids", m.getOrgs)
	r.Get("", m.getAllOrgs)
}
