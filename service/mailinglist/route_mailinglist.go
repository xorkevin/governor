package mailinglist

import (
	"net/http"
	"net/url"
	"strings"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/cachecontrol"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/util/rank"
	"xorkevin.dev/kerrors"
)

type (
	//forge:valid
	reqCreatorLists struct {
		CreatorID string `valid:"creatorID,has" json:"-"`
		Amount    int    `valid:"amount" json:"-"`
		Offset    int    `valid:"offset" json:"-"`
	}
)

func (s *router) getCreatorLists(c governor.Context) {
	req := reqCreatorLists{
		CreatorID: c.Param("creatorid"),
		Amount:    c.QueryInt("amount", -1),
		Offset:    c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.getCreatorLists(c.Ctx(), req.CreatorID, req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqUserLists struct {
		Userid string `valid:"userid,has" json:"-"`
		Amount int    `valid:"amount" json:"-"`
		Offset int    `valid:"offset" json:"-"`
	}
)

func (s *router) getPersonalLists(c governor.Context) {
	req := reqUserLists{
		Userid: gate.GetCtxUserid(c),
		Amount: c.QueryInt("amount", -1),
		Offset: c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.getLatestLists(c.Ctx(), req.Userid, req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqList struct {
		Listid string `valid:"listid,has" json:"-"`
	}
)

func (s *router) getList(c governor.Context) {
	req := reqList{
		Listid: c.Param("listid"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.getList(c.Ctx(), req.Listid)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqListMsgs struct {
		Listid string `valid:"listid,has" json:"-"`
		Amount int    `valid:"amount" json:"-"`
		Offset int    `valid:"offset" json:"-"`
	}
)

func (s *router) getListMsgs(c governor.Context) {
	req := reqListMsgs{
		Listid: c.Param("listid"),
		Amount: c.QueryInt("amount", -1),
		Offset: c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.getLatestMsgs(c.Ctx(), req.Listid, req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (s *router) getListThreads(c governor.Context) {
	req := reqListMsgs{
		Listid: c.Param("listid"),
		Amount: c.QueryInt("amount", -1),
		Offset: c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.getLatestThreads(c.Ctx(), req.Listid, req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqListThread struct {
		Listid   string `valid:"listid,has" json:"-"`
		Threadid string `valid:"msgid,has" json:"-"`
		Amount   int    `valid:"amount" json:"-"`
		Offset   int    `valid:"offset" json:"-"`
	}
)

func (s *router) getListThread(c governor.Context) {
	threadid, err := url.QueryUnescape(c.Param("threadid"))
	if err != nil {
		c.WriteError(governor.ErrWithRes(err, http.StatusBadRequest, "", "Invalid msg id"))
		return
	}
	req := reqListThread{
		Listid:   c.Param("listid"),
		Threadid: threadid,
		Amount:   c.QueryInt("amount", -1),
		Offset:   c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.getThreadMsgs(c.Ctx(), req.Listid, req.Threadid, req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqListMsg struct {
		Listid string `valid:"listid,has" json:"-"`
		Msgid  string `valid:"msgid,has" json:"-"`
	}
)

func (s *router) getListMsg(c governor.Context) {
	msgid, err := url.QueryUnescape(c.Param("msgid"))
	if err != nil {
		c.WriteError(governor.ErrWithRes(err, http.StatusBadRequest, "", "Invalid msg id"))
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

	res, err := s.s.getMsg(c.Ctx(), req.Listid, req.Msgid)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (s *router) getListMsgContent(c governor.Context) {
	msgid, err := url.QueryUnescape(c.Param("msgid"))
	if err != nil {
		c.WriteError(governor.ErrWithRes(err, http.StatusBadRequest, "", "Invalid msg id"))
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

	msg, contentType, err := s.s.getMsgContent(c.Ctx(), req.Listid, req.Msgid)
	if err != nil {
		c.WriteError(err)
		return
	}
	defer func() {
		if err := msg.Close(); err != nil {
			s.s.log.Err(c.Ctx(), kerrors.WithMsg(err, "Failed to close msg content"), nil)
		}
	}()
	c.WriteFile(http.StatusOK, contentType, msg)
}

func (s *router) getListMsgCC(c governor.Context) (string, error) {
	msgid, err := url.QueryUnescape(c.Param("msgid"))
	if err != nil {
		return "", governor.ErrWithRes(err, http.StatusBadRequest, "", "Invalid msg id")
	}
	req := reqListMsg{
		Listid: c.Param("listid"),
		Msgid:  msgid,
	}
	if err := req.valid(); err != nil {
		return "", err
	}

	objinfo, err := s.s.statMsg(c.Ctx(), req.Listid, req.Msgid)
	if err != nil {
		return "", err
	}

	return objinfo.ETag, nil
}

func (s *router) getListMembers(c governor.Context) {
	req := reqListMsgs{
		Listid: c.Param("listid"),
		Amount: c.QueryInt("amount", -1),
		Offset: c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.getListMembers(c.Ctx(), req.Listid, req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqListMembers struct {
		Listid  string   `valid:"listid,has" json:"-"`
		Userids []string `valid:"userids,has" json:"-"`
	}
)

func (s *router) getListMemberIDs(c governor.Context) {
	req := reqListMembers{
		Listid:  c.Param("listid"),
		Userids: strings.Split(c.Query("ids"), ","),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.getListMemberIDs(c.Ctx(), req.Listid, req.Userids)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqCreateList struct {
		CreatorID    string `valid:"creatorID,has" json:"-"`
		Listname     string `valid:"listname" json:"listname"`
		Name         string `valid:"name" json:"name"`
		Desc         string `valid:"desc" json:"desc"`
		SenderPolicy string `valid:"senderPolicy" json:"sender_policy"`
		MemberPolicy string `valid:"memberPolicy" json:"member_policy"`
	}
)

func (s *router) createList(c governor.Context) {
	var req reqCreateList
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	req.CreatorID = c.Param("creatorid")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.createList(c.Ctx(), req.CreatorID, req.Listname, req.Name, req.Desc, req.SenderPolicy, req.MemberPolicy)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusCreated, res)
}

type (
	//forge:valid
	reqUpdateList struct {
		CreatorID    string `valid:"creatorID,has" json:"-"`
		Listname     string `valid:"listname,has" json:"-"`
		Name         string `valid:"name" json:"name"`
		Desc         string `valid:"desc" json:"desc"`
		Archive      bool   `json:"archive"`
		SenderPolicy string `valid:"senderPolicy" json:"sender_policy"`
		MemberPolicy string `valid:"memberPolicy" json:"member_policy"`
	}
)

func (s *router) updateList(c governor.Context) {
	var req reqUpdateList
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	req.CreatorID = c.Param("creatorid")
	req.Listname = c.Param("listname")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := s.s.updateList(c.Ctx(), req.CreatorID, req.Listname, req.Name, req.Desc, req.Archive, req.SenderPolicy, req.MemberPolicy); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	//forge:valid
	reqSub struct {
		CreatorID string `valid:"creatorID,has" json:"-"`
		Listname  string `valid:"listname,has" json:"-"`
		Userid    string `valid:"userid,has" json:"-"`
	}
)

func (s *router) subList(c governor.Context) {
	req := reqSub{
		CreatorID: c.Param("creatorid"),
		Listname:  c.Param("listname"),
		Userid:    gate.GetCtxUserid(c),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := s.s.subscribe(c.Ctx(), req.CreatorID, req.Listname, req.Userid); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (s *router) unsubList(c governor.Context) {
	req := reqSub{
		CreatorID: c.Param("creatorid"),
		Listname:  c.Param("listname"),
		Userid:    gate.GetCtxUserid(c),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := s.s.removeListMembers(c.Ctx(), req.CreatorID, req.Listname, []string{req.Userid}); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	//forge:valid
	reqUpdListMembers struct {
		CreatorID string   `valid:"creatorID,has" json:"-"`
		Listname  string   `valid:"listname,has" json:"-"`
		Remove    []string `valid:"userids,has" json:"remove"`
	}
)

func (s *router) updateListMembers(c governor.Context) {
	var req reqUpdListMembers
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	req.CreatorID = c.Param("creatorid")
	req.Listname = c.Param("listname")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := s.s.removeListMembers(c.Ctx(), req.CreatorID, req.Listname, req.Remove); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	//forge:valid
	reqListID struct {
		CreatorID string `valid:"creatorID,has" json:"-"`
		Listname  string `valid:"listname,has" json:"-"`
	}
)

func (s *router) deleteList(c governor.Context) {
	req := reqListID{
		CreatorID: c.Param("creatorid"),
		Listname:  c.Param("listname"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := s.s.deleteList(c.Ctx(), req.CreatorID, req.Listname); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	//forge:valid
	reqMsgIDs struct {
		CreatorID string   `valid:"creatorID,has" json:"-"`
		Listname  string   `valid:"listname,has" json:"-"`
		Msgids    []string `valid:"msgids,has" json:"msgids"`
	}
)

func (s *router) deleteMsgs(c governor.Context) {
	var req reqMsgIDs
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	req.CreatorID = c.Param("creatorid")
	req.Listname = c.Param("listname")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := s.s.deleteMsgs(c.Ctx(), req.CreatorID, req.Listname, req.Msgids); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (s *router) listOwner(c governor.Context, userid string) (string, bool, bool) {
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

func (s *router) listNoBan(c governor.Context, userid string) (string, bool, bool) {
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

func (s *router) mountRoutes(r governor.Router) {
	m := governor.NewMethodRouter(r)
	scopeMailinglistRead := s.s.scopens + ":read"
	scopeMailinglistWrite := s.s.scopens + ":write"
	scopeMailinglistSubWrite := s.s.scopens + ".sub:write"
	m.PostCtx("/c/{creatorid}", s.createList, gate.MemberF(s.s.gate, s.listOwner, scopeMailinglistWrite), s.rt)
	m.GetCtx("/c/{creatorid}/latest", s.getCreatorLists, s.rt)
	m.GetCtx("/latest", s.getPersonalLists, gate.User(s.s.gate, scopeMailinglistRead), s.rt)
	m.PutCtx("/c/{creatorid}/list/{listname}", s.updateList, gate.MemberF(s.s.gate, s.listOwner, scopeMailinglistWrite), s.rt)
	m.PatchCtx("/c/{creatorid}/list/{listname}/sub", s.subList, gate.NoBanF(s.s.gate, s.listNoBan, scopeMailinglistSubWrite), s.rt)
	m.PatchCtx("/c/{creatorid}/list/{listname}/unsub", s.unsubList, gate.User(s.s.gate, scopeMailinglistSubWrite), s.rt)
	m.DeleteCtx("/c/{creatorid}/list/{listname}/msgs", s.deleteMsgs, gate.MemberF(s.s.gate, s.listOwner, scopeMailinglistWrite), s.rt)
	m.PatchCtx("/c/{creatorid}/list/{listname}/member", s.updateListMembers, gate.MemberF(s.s.gate, s.listOwner, scopeMailinglistWrite), s.rt)
	m.DeleteCtx("/c/{creatorid}/list/{listname}", s.deleteList, gate.MemberF(s.s.gate, s.listOwner, scopeMailinglistWrite), s.rt)
	m.GetCtx("/l/{listid}", s.getList, s.rt)
	m.GetCtx("/l/{listid}/msgs", s.getListMsgs, s.rt)
	m.GetCtx("/l/{listid}/threads", s.getListThreads, s.rt)
	m.GetCtx("/l/{listid}/threads/id/{threadid}/msgs", s.getListThread, s.rt)
	m.GetCtx("/l/{listid}/msgs/id/{msgid}", s.getListMsg, s.rt)
	m.GetCtx("/l/{listid}/msgs/id/{msgid}/content", s.getListMsgContent, cachecontrol.ControlCtx(true, nil, 60, s.getListMsgCC), s.rt)
	m.GetCtx("/l/{listid}/member", s.getListMembers, s.rt)
	m.GetCtx("/l/{listid}/member/ids", s.getListMemberIDs, s.rt)
}
