package mailinglist

import (
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/util/rank"
)

//go:generate forge validation -o validation_mailinglist_gen.go reqCreatorLists reqUserLists reqListMsgs

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
)

func (m *router) getUserLists(w http.ResponseWriter, r *http.Request) {
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

func (m *router) getOrgLists(w http.ResponseWriter, r *http.Request) {
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
	res, err := m.s.GetCreatorLists(rank.ToOrgName(req.CreatorID), req.Before, req.Amount)
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

const (
	scopeMailinglistRead  = "gov.mailinglist:read"
	scopeMailinglistWrite = "gov.mailinglist:write"
)

func (m *router) mountRoutes(r governor.Router) {
	r.Get("/u/{creatorid}/latest", m.getUserLists, gate.User(m.s.gate, scopeMailinglistRead))
	r.Get("/o/{creatorid}/latest", m.getOrgLists, gate.User(m.s.gate, scopeMailinglistRead))
	r.Get("/latest", m.getPersonalLists, gate.User(m.s.gate, scopeMailinglistRead))
	r.Get("/l/{listid}/msgs", m.getListMsgs, gate.User(m.s.gate, scopeMailinglistRead))
}
