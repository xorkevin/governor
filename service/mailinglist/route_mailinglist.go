package mailinglist

import (
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/util/rank"
)

//go:generate forge validation -o validation_mailinglist_gen.go reqCreatorLists reqUserLists reqListMsgs reqCreateList

type (
	reqCreatorLists struct {
		CreatorID string `valid:"creatorID,has" json:"-"`
		Before    int64  `json:"-"`
		Amount    int    `valid:"amount" json:"-"`
	}

	reqUserLists struct {
		Userid string `valid:"userid,has" json:"-"`
		Before int64  `json:"-"`
		Amount int    `valid:"amount" json:"-"`
	}

	reqListMsgs struct {
		Listid string `valid:"listid,has" json:"-"`
		Before int64  `json:"-"`
		Amount int    `valid:"amount" json:"-"`
	}

	reqCreateList struct {
		CreatorID    string `valid:"creatorID,has" json:"-"`
		Listname     string `valid:"listname" json:"listname"`
		Name         string `valid:"name" json:"name"`
		Desc         string `valid:"desc" json:"desc"`
		SenderPolicy string `valid:"senderPolicy" json:"sender_policy"`
		MemberPolicy string `valid:"memberPolicy" json:"member_policy"`
	}
)

func (m *router) getCreatorLists(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqCreatorLists{
		CreatorID: c.Param("creatorid"),
		Before:    c.QueryInt64("before", 0),
		Amount:    c.QueryInt("amount", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := m.s.GetCreatorLists(req.CreatorID, req.Before, req.Amount)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (m *router) getPersonalLists(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqUserLists{
		Userid: gate.GetCtxUserid(c),
		Before: c.QueryInt64("before", 0),
		Amount: c.QueryInt("amount", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := m.s.GetLatestLists(req.Userid, req.Before, req.Amount)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (m *router) getListMsgs(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqListMsgs{
		Listid: c.Param("listid"),
		Before: c.QueryInt64("before", 0),
		Amount: c.QueryInt("amount", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := m.s.GetLatestMsgs(req.Listid, req.Before, req.Amount)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (m *router) createList(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqCreateList{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	req.CreatorID = c.Param("creatorid")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := m.s.CreateList(req.CreatorID, req.Listname, req.Name, req.Desc, req.SenderPolicy, req.MemberPolicy)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusCreated, res)
}

func (m *router) listOwner(c governor.Context, userid string) (string, bool, bool) {
	creatorid := c.Param("creatorid")
	if err := validhasCreatorID(creatorid); err != nil {
		return "", false, false
	}
	if creatorid == userid {
		return "", false, false
	}
	if !rank.IsValidOrgName(creatorid) {
		return "", false, false
	}
	return creatorid, false, true
}

const (
	scopeMailinglistRead  = "gov.mailinglist:read"
	scopeMailinglistWrite = "gov.mailinglist:write"
)

func (m *router) mountRoutes(r governor.Router) {
	r.Post("/c/{creatorid}", m.createList, gate.MemberF(m.s.gate, m.listOwner, scopeMailinglistWrite))
	r.Get("/c/{creatorid}/latest", m.getCreatorLists, gate.User(m.s.gate, scopeMailinglistRead))
	r.Get("/latest", m.getPersonalLists, gate.User(m.s.gate, scopeMailinglistRead))
	r.Get("/l/{listid}/msgs", m.getListMsgs, gate.User(m.s.gate, scopeMailinglistRead))
}
