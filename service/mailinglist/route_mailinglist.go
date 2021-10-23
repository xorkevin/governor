package mailinglist

import (
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/util/rank"
)

//go:generate forge validation -o validation_mailinglist_gen.go reqCreatorLists reqUserLists reqList reqListMsgs reqCreateList reqUpdateList reqListMembers reqListID reqMsgIDs

type (
	reqCreatorLists struct {
		CreatorID string `valid:"creatorID,has" json:"-"`
		Amount    int    `valid:"amount" json:"-"`
		Offset    int    `valid:"offset" json:"-"`
	}

	reqUserLists struct {
		Userid string `valid:"userid,has" json:"-"`
		Amount int    `valid:"amount" json:"-"`
		Offset int    `valid:"offset" json:"-"`
	}

	reqList struct {
		Listid string `valid:"listid,has" json:"-"`
	}

	reqListMsgs struct {
		Listid string `valid:"listid,has" json:"-"`
		Amount int    `valid:"amount" json:"-"`
		Offset int    `valid:"offset" json:"-"`
	}

	reqCreateList struct {
		CreatorID    string `valid:"creatorID,has" json:"-"`
		Listname     string `valid:"listname" json:"listname"`
		Name         string `valid:"name" json:"name"`
		Desc         string `valid:"desc" json:"desc"`
		SenderPolicy string `valid:"senderPolicy" json:"sender_policy"`
		MemberPolicy string `valid:"memberPolicy" json:"member_policy"`
	}

	reqUpdateList struct {
		CreatorID    string `valid:"creatorID,has" json:"-"`
		Listname     string `valid:"listname,has" json:"-"`
		Name         string `valid:"name" json:"name"`
		Desc         string `valid:"desc" json:"desc"`
		Archive      bool   `json:"archive"`
		SenderPolicy string `valid:"senderPolicy" json:"sender_policy"`
		MemberPolicy string `valid:"memberPolicy" json:"member_policy"`
	}

	reqListMembers struct {
		CreatorID string   `valid:"creatorID,has" json:"-"`
		Listname  string   `valid:"listname,has" json:"-"`
		Add       []string `valid:"userids,opt" json:"add"`
		Remove    []string `valid:"userids,opt" json:"remove"`
	}

	reqListID struct {
		CreatorID string `valid:"creatorID,has" json:"-"`
		Listname  string `valid:"listname,has" json:"-"`
	}

	reqMsgIDs struct {
		CreatorID string   `valid:"creatorID,has" json:"-"`
		Listname  string   `valid:"listname,has" json:"-"`
		Msgids    []string `valid:"msgids,has" json:"msgids"`
	}
)

func (m *router) getCreatorLists(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqCreatorLists{
		CreatorID: c.Param("creatorid"),
		Amount:    c.QueryInt("amount", -1),
		Offset:    c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := m.s.GetCreatorLists(req.CreatorID, req.Amount, req.Offset)
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
		Amount: c.QueryInt("amount", -1),
		Offset: c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := m.s.GetLatestLists(req.Userid, req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (m *router) getList(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqList{
		Listid: c.Param("listid"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := m.s.GetList(req.Listid)
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
		Amount: c.QueryInt("amount", -1),
		Offset: c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := m.s.GetLatestMsgs(req.Listid, req.Amount, req.Offset)
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

func (m *router) updateList(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqUpdateList{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	req.CreatorID = c.Param("creatorid")
	req.Listname = c.Param("listname")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := m.s.UpdateList(req.CreatorID, req.Listname, req.Name, req.Desc, req.Archive, req.SenderPolicy, req.MemberPolicy); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (m *router) updateListMembers(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqListMembers{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	req.CreatorID = c.Param("creatorid")
	req.Listname = c.Param("listname")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if len(req.Add) > 0 {
		if err := m.s.AddListMembers(req.CreatorID, req.Listname, req.Add); err != nil {
			c.WriteError(err)
			return
		}
	}
	if len(req.Remove) > 0 {
		if err := m.s.RemoveListMembers(req.CreatorID, req.Listname, req.Remove); err != nil {
			c.WriteError(err)
			return
		}
	}
	c.WriteStatus(http.StatusNoContent)
}

func (m *router) deleteList(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqListID{
		CreatorID: c.Param("creatorid"),
		Listname:  c.Param("listname"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := m.s.DeleteList(req.CreatorID, req.Listname); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (m *router) deleteMsgs(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqMsgIDs{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	req.CreatorID = c.Param("creatorid")
	req.Listname = c.Param("listname")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := m.s.DeleteMsgs(req.CreatorID, req.Listname, req.Msgids); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (m *router) listOwner(c governor.Context, userid string) (string, bool, bool) {
	creatorid := c.Param("creatorid")
	if err := validhasCreatorID(creatorid); err != nil {
		return "", false, false
	}
	if creatorid == userid {
		return "", true, true
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
	r.Get("/c/{creatorid}/latest", m.getCreatorLists)
	r.Get("/latest", m.getPersonalLists, gate.User(m.s.gate, scopeMailinglistRead))
	r.Put("/c/{creatorid}/list/{listname}", m.updateList, gate.MemberF(m.s.gate, m.listOwner, scopeMailinglistWrite))
	r.Patch("/c/{creatorid}/list/{listname}/member", m.updateListMembers, gate.MemberF(m.s.gate, m.listOwner, scopeMailinglistWrite))
	r.Delete("/c/{creatorid}/list/{listname}", m.deleteList, gate.MemberF(m.s.gate, m.listOwner, scopeMailinglistWrite))
	r.Get("/l/{listid}", m.getList)
	r.Get("/l/{listid}/msgs", m.getListMsgs)
	r.Delete("/c/{creatorid}/l/{listname}/msgs", m.deleteMsgs, gate.MemberF(m.s.gate, m.listOwner, scopeMailinglistWrite))
}
