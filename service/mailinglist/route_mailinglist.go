package mailinglist

import (
	"net/http"
	"net/url"
	"strings"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/cachecontrol"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/util/rank"
)

//go:generate forge validation -o validation_mailinglist_gen.go reqCreatorLists reqUserLists reqList reqListMsgs reqListMsg reqListMembers reqCreateList reqUpdateList reqSub reqUpdListMembers reqListID reqMsgIDs

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

	reqListMsg struct {
		Listid string `valid:"listid,has" json:"-"`
		Msgid  string `valid:"msgid,has" json:"-"`
	}

	reqListMembers struct {
		Listid  string   `valid:"listid,has" json:"-"`
		Userids []string `valid:"userids,has" json:"-"`
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

	reqSub struct {
		CreatorID string `valid:"creatorID,has" json:"-"`
		Listname  string `valid:"listname,has" json:"-"`
		Userid    string `valid:"userid,has" json:"-"`
	}

	reqUpdListMembers struct {
		CreatorID string   `valid:"creatorID,has" json:"-"`
		Listname  string   `valid:"listname,has" json:"-"`
		Remove    []string `valid:"userids,has" json:"remove"`
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

func (m *router) getListMsg(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	msgid, err := url.QueryUnescape(c.Param("msgid"))
	if err != nil {
		c.WriteError(governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Invalid msg id",
		})))
		return
	}
	req := reqListMsg{
		Listid: c.Param("listid"),
		Msgid:  msgid,
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	msg, contentType, err := m.s.GetMsg(req.Listid, req.Msgid)
	if err != nil {
		c.WriteError(err)
		return
	}
	defer func() {
		if err := msg.Close(); err != nil {
			m.s.logger.Error("Failed to close msg content", map[string]string{
				"actiontype": "getlistmsg",
				"error":      err.Error(),
			})
		}
	}()
	c.WriteFile(http.StatusOK, contentType, msg)
}

func (m *router) getListMsgCC(c governor.Context) (string, error) {
	msgid, err := url.QueryUnescape(c.Param("msgid"))
	if err != nil {
		return "", (governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Invalid msg id",
		})))
	}
	req := reqListMsg{
		Listid: c.Param("listid"),
		Msgid:  msgid,
	}
	if err := req.valid(); err != nil {
		return "", err
	}

	objinfo, err := m.s.StatMsg(req.Listid, req.Msgid)
	if err != nil {
		return "", err
	}

	return objinfo.ETag, nil
}

func (m *router) getListMembers(w http.ResponseWriter, r *http.Request) {
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
	res, err := m.s.GetListMembers(req.Listid, req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (m *router) getListMemberIDs(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqListMembers{
		Listid:  c.Param("listid"),
		Userids: strings.Split(c.Query("ids"), ","),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := m.s.GetListMemberIDs(req.Listid, req.Userids)
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

func (m *router) subList(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqSub{
		CreatorID: c.Param("creatorid"),
		Listname:  c.Param("listname"),
		Userid:    gate.GetCtxUserid(c),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := m.s.Subscribe(req.CreatorID, req.Listname, req.Userid); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (m *router) unsubList(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqSub{
		CreatorID: c.Param("creatorid"),
		Listname:  c.Param("listname"),
		Userid:    gate.GetCtxUserid(c),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := m.s.RemoveListMembers(req.CreatorID, req.Listname, []string{req.Userid}); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (m *router) updateListMembers(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqUpdListMembers{}
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
	if err := m.s.RemoveListMembers(req.CreatorID, req.Listname, req.Remove); err != nil {
		c.WriteError(err)
		return
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

func (m *router) listNoBan(c governor.Context, userid string) (string, bool, bool) {
	creatorid := c.Param("creatorid")
	if err := validhasCreatorID(creatorid); err != nil {
		return "", false, false
	}
	if creatorid == userid {
		return "", true, true
	}
	if !rank.IsValidOrgName(creatorid) {
		return "", true, true
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
	r.Patch("/c/{creatorid}/list/{listname}/sub", m.subList, gate.NoBanF(m.s.gate, m.listNoBan, scopeMailinglistWrite))
	r.Patch("/c/{creatorid}/list/{listname}/unsub", m.unsubList, gate.User(m.s.gate, scopeMailinglistWrite))
	r.Delete("/c/{creatorid}/list/{listname}/msgs", m.deleteMsgs, gate.MemberF(m.s.gate, m.listOwner, scopeMailinglistWrite))
	r.Patch("/c/{creatorid}/list/{listname}/member", m.updateListMembers, gate.MemberF(m.s.gate, m.listOwner, scopeMailinglistWrite))
	r.Delete("/c/{creatorid}/list/{listname}", m.deleteList, gate.MemberF(m.s.gate, m.listOwner, scopeMailinglistWrite))
	r.Get("/l/{listid}", m.getList)
	r.Get("/l/{listid}/msgs", m.getListMsgs)
	r.Get("/l/{listid}/msgs/{msgid}", m.getListMsg, cachecontrol.Control(m.s.logger, true, nil, 60, m.getListMsgCC))
	r.Get("/l/{listid}/member", m.getListMembers)
	r.Get("/l/{listid}/member/ids", m.getListMemberIDs)
}
