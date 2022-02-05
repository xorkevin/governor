package conduit

import (
	"net/http"
	"strings"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
)

//go:generate forge validation -o validation_conduit_gen.go reqGetFriends reqSearchFriends reqRmFriend reqAcceptFriendInvitation reqDelFriendInvitation reqGetFriendInvitations reqGetLatestChats reqGetChats reqUpdateDM reqCreateMsg reqGetMsgs reqDelMsg reqGetPresence reqSearchGDMs reqUpdateGDM reqDelGDM reqGDMMember

type (
	reqGetFriends struct {
		Userid string `valid:"userid,has" json:"-"`
		Prefix string `valid:"username,opt" json:"-"`
		Amount int    `valid:"amount" json:"-"`
		Offset int    `valid:"offset" json:"-"`
	}
)

func (m *router) getFriends(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqGetFriends{
		Userid: gate.GetCtxUserid(c),
		Prefix: c.Query("prefix"),
		Amount: c.QueryInt("amount", -1),
		Offset: c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := m.s.GetFriends(req.Userid, req.Prefix, req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	reqSearchFriends struct {
		Userid string `valid:"userid,has" json:"-"`
		Prefix string `valid:"username,has" json:"-"`
		Amount int    `valid:"amount" json:"-"`
	}
)

func (m *router) searchFriends(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqSearchFriends{
		Userid: gate.GetCtxUserid(c),
		Prefix: c.Query("prefix"),
		Amount: c.QueryInt("amount", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := m.s.SearchFriends(req.Userid, req.Prefix, req.Amount)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	reqRmFriend struct {
		Userid1 string `valid:"userid,has" json:"-"`
		Userid2 string `valid:"userid,has" json:"-"`
	}
)

func (m *router) removeFriend(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqRmFriend{
		Userid1: gate.GetCtxUserid(c),
		Userid2: c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := m.s.RemoveFriend(req.Userid1, req.Userid2); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	reqAcceptFriendInvitation struct {
		Userid    string `valid:"userid,has" json:"-"`
		InvitedBy string `valid:"userid,has" json:"-"`
	}
)

func (m *router) sendFriendInvitation(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqAcceptFriendInvitation{
		Userid:    c.Param("id"),
		InvitedBy: gate.GetCtxUserid(c),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := m.s.InviteFriend(req.Userid, req.InvitedBy); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (m *router) acceptFriendInvitation(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqAcceptFriendInvitation{
		Userid:    gate.GetCtxUserid(c),
		InvitedBy: c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := m.s.AcceptFriendInvitation(req.Userid, req.InvitedBy); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	reqDelFriendInvitation struct {
		Userid    string `valid:"userid,has" json:"-"`
		InvitedBy string `valid:"userid,has" json:"-"`
	}
)

func (m *router) deleteUserFriendInvitation(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqDelFriendInvitation{
		Userid:    gate.GetCtxUserid(c),
		InvitedBy: c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := m.s.DeleteFriendInvitation(req.Userid, req.InvitedBy); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (m *router) deleteInvitedFriendInvitation(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqDelFriendInvitation{
		Userid:    c.Param("id"),
		InvitedBy: gate.GetCtxUserid(c),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := m.s.DeleteFriendInvitation(req.Userid, req.InvitedBy); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	reqGetFriendInvitations struct {
		Userid string `valid:"userid,has" json:"-"`
		Amount int    `valid:"amount" json:"-"`
		Offset int    `valid:"offset" json:"-"`
	}
)

func (m *router) getInvitations(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqGetFriendInvitations{
		Userid: gate.GetCtxUserid(c),
		Amount: c.QueryInt("amount", -1),
		Offset: c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := m.s.GetUserFriendInvitations(req.Userid, req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (m *router) getInvited(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqGetFriendInvitations{
		Userid: gate.GetCtxUserid(c),
		Amount: c.QueryInt("amount", -1),
		Offset: c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := m.s.GetInvitedFriendInvitations(req.Userid, req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	reqGetLatestChats struct {
		Userid string `valid:"userid,has" json:"-"`
		Before int64  `json:"-"`
		Amount int    `valid:"amount" json:"-"`
	}
)

func (m *router) getLatestDMs(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqGetLatestChats{
		Userid: gate.GetCtxUserid(c),
		Before: c.QueryInt64("before", 0),
		Amount: c.QueryInt("amount", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := m.s.GetLatestDMs(req.Userid, req.Before, req.Amount)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	reqGetChats struct {
		Userid  string   `valid:"userid,has" json:"-"`
		Chatids []string `valid:"chatids,has" json:"-"`
	}
)

func (m *router) getDMs(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqGetChats{
		Userid:  gate.GetCtxUserid(c),
		Chatids: strings.Split(c.Query("ids"), ","),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := m.s.GetDMs(req.Userid, req.Chatids)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (m *router) searchDMs(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqSearchFriends{
		Userid: gate.GetCtxUserid(c),
		Prefix: c.Query("prefix"),
		Amount: c.QueryInt("amount", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := m.s.SearchDMs(req.Userid, req.Prefix, req.Amount)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	reqUpdateDM struct {
		Userid string `valid:"userid,has" json:"-"`
		Chatid string `valid:"chatid,has" json:"-"`
		Name   string `valid:"name" json:"name"`
		Theme  string `valid:"theme" json:"theme"`
	}
)

func (m *router) updateDM(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqUpdateDM{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	req.Chatid = c.Param("id")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := m.s.UpdateDM(req.Userid, req.Chatid, req.Name, req.Theme); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	reqCreateMsg struct {
		Userid string `valid:"userid,has" json:"-"`
		Chatid string `valid:"chatid,has" json:"-"`
		Kind   string `valid:"msgkind" json:"kind"`
		Value  string `valid:"msgvalue" json:"value"`
	}
)

func (m *router) createDMMsg(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqCreateMsg{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	req.Chatid = c.Param("id")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := m.s.CreateDMMsg(req.Userid, req.Chatid, req.Kind, req.Value)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusCreated, res)
}

type (
	reqGetMsgs struct {
		Userid string `valid:"userid,has" json:"-"`
		Chatid string `valid:"chatid,has" json:"-"`
		Kind   string `valid:"msgkind,opt" json:"-"`
		Before string `valid:"msgid,opt" json:"-"`
		Amount int    `valid:"amount" json:"-"`
	}
)

func (m *router) getDMMsgs(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqGetMsgs{
		Userid: gate.GetCtxUserid(c),
		Chatid: c.Param("id"),
		Kind:   c.Query("kind"),
		Before: c.Query("before"),
		Amount: c.QueryInt("amount", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := m.s.GetDMMsgs(req.Userid, req.Chatid, req.Kind, req.Before, req.Amount)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	reqDelMsg struct {
		Userid string `valid:"userid,has" json:"-"`
		Chatid string `valid:"chatid,has" json:"-"`
		Msgid  string `valid:"msgid,has" json:"-"`
	}
)

func (m *router) deleteDMMsg(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqDelMsg{
		Userid: gate.GetCtxUserid(c),
		Chatid: c.Param("id"),
		Msgid:  c.Param("msgid"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := m.s.DelDMMsg(req.Userid, req.Chatid, req.Msgid); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	reqGetPresence struct {
		Userid  string   `valid:"userid,has" json:"-"`
		Userids []string `valid:"userids,has" json:"userids"`
	}
)

func (m *router) getLatestGDMs(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqGetLatestChats{
		Userid: gate.GetCtxUserid(c),
		Before: c.QueryInt64("before", 0),
		Amount: c.QueryInt("amount", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := m.s.GetLatestGDMs(req.Userid, req.Before, req.Amount)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (m *router) getGDMs(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqGetChats{
		Userid:  gate.GetCtxUserid(c),
		Chatids: strings.Split(c.Query("ids"), ","),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := m.s.GetGDMs(req.Userid, req.Chatids)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	reqSearchGDMs struct {
		Userid1 string `valid:"userid,has" json:"-"`
		Userid2 string `valid:"userid,has" json:"-"`
		Amount  int    `valid:"amount" json:"-"`
		Offset  int    `valid:"offset" json:"-"`
	}
)

func (m *router) searchGDMs(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqSearchGDMs{
		Userid1: gate.GetCtxUserid(c),
		Userid2: c.Query("id"),
		Amount:  c.QueryInt("amount", -1),
		Offset:  c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := m.s.SearchGDMs(req.Userid1, req.Userid2, req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	reqUpdateGDM struct {
		Userid string `valid:"userid,has" json:"-"`
		Chatid string `valid:"chatid,has" json:"-"`
		Name   string `valid:"name" json:"name"`
		Theme  string `valid:"theme" json:"theme"`
	}
)

func (m *router) updateGDM(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqUpdateGDM{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	req.Chatid = c.Param("id")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := m.s.UpdateGDM(req.Userid, req.Chatid, req.Name, req.Theme); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	reqDelGDM struct {
		Userid string `valid:"userid,has" json:"-"`
		Chatid string `valid:"chatid,has" json:"-"`
	}
)

func (m *router) deleteGDM(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqDelGDM{
		Userid: gate.GetCtxUserid(c),
		Chatid: c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := m.s.DeleteGDM(req.Userid, req.Chatid); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (m *router) createGDMMsg(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqCreateMsg{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	req.Chatid = c.Param("id")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := m.s.CreateGDMMsg(req.Userid, req.Chatid, req.Kind, req.Value)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusCreated, res)
}

func (m *router) getGDMMsgs(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqGetMsgs{
		Userid: gate.GetCtxUserid(c),
		Chatid: c.Param("id"),
		Kind:   c.Query("kind"),
		Before: c.Query("before"),
		Amount: c.QueryInt("amount", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := m.s.GetGDMMsgs(req.Userid, req.Chatid, req.Kind, req.Before, req.Amount)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (m *router) deleteGDMMsg(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqDelMsg{
		Userid: gate.GetCtxUserid(c),
		Chatid: c.Param("id"),
		Msgid:  c.Param("msgid"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := m.s.DelGDMMsg(req.Userid, req.Chatid, req.Msgid); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	reqGDMMember struct {
		Userid  string   `valid:"userid,has" json:"-"`
		Chatid  string   `valid:"chatid,has" json:"-"`
		Members []string `valid:"userids,has" json:"members"`
	}
)

func (m *router) addGDMMember(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqGDMMember{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	req.Chatid = c.Param("id")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := m.s.AddGDMMembers(req.Userid, req.Chatid, req.Members); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (m *router) rmGDMMembers(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqGDMMember{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	req.Chatid = c.Param("id")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := m.s.RmGDMMembers(req.Userid, req.Chatid, req.Members); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (m *router) mountRoutes(r governor.Router) {
	scopeFriendRead := m.s.scopens + ".friend:read"
	scopeFriendWrite := m.s.scopens + ".friend:write"
	r.Get("/friend", m.getFriends, gate.User(m.s.gate, scopeFriendRead))
	r.Delete("/friend/id/{id}", m.removeFriend, gate.User(m.s.gate, scopeFriendWrite))
	r.Get("/friend/search", m.searchFriends, gate.User(m.s.gate, scopeFriendRead))
	r.Get("/friend/invitation", m.getInvitations, gate.User(m.s.gate, scopeFriendRead))
	r.Get("/friend/invitation/invited", m.getInvited, gate.User(m.s.gate, scopeFriendRead))
	r.Post("/friend/invitation/id/{id}", m.sendFriendInvitation, gate.User(m.s.gate, scopeFriendWrite))
	r.Post("/friend/invitation/id/{id}/accept", m.acceptFriendInvitation, gate.User(m.s.gate, scopeFriendWrite))
	r.Delete("/friend/invitation/id/{id}", m.deleteUserFriendInvitation, gate.User(m.s.gate, scopeFriendWrite))
	r.Delete("/friend/invitation/invited/{id}", m.deleteInvitedFriendInvitation, gate.User(m.s.gate, scopeFriendWrite))

	scopeChatRead := m.s.scopens + ".chat:read"
	scopeChatWrite := m.s.scopens + ".chat:write"
	scopeChatAdminWrite := m.s.scopens + ".chat.admin:write"
	r.Get("/dm", m.getLatestDMs, gate.User(m.s.gate, scopeChatRead))
	r.Get("/dm/ids", m.getDMs, gate.User(m.s.gate, scopeChatRead))
	r.Get("/dm/search", m.searchDMs, gate.User(m.s.gate, scopeChatRead))
	r.Put("/dm/id/{id}", m.updateDM, gate.User(m.s.gate, scopeChatAdminWrite))
	r.Post("/dm/id/{id}/msg", m.createDMMsg, gate.User(m.s.gate, scopeChatWrite))
	r.Get("/dm/id/{id}/msg", m.getDMMsgs, gate.User(m.s.gate, scopeChatRead))
	r.Delete("/dm/id/{id}/msg/id/{msgid}", m.deleteDMMsg, gate.User(m.s.gate, scopeChatWrite))

	r.Get("/gdm", m.getLatestGDMs, gate.User(m.s.gate, scopeChatRead))
	r.Get("/gdm/ids", m.getGDMs, gate.User(m.s.gate, scopeChatRead))
	r.Get("/gdm/search", m.searchGDMs, gate.User(m.s.gate, scopeChatRead))
	r.Put("/gdm/id/{id}", m.updateGDM, gate.User(m.s.gate, scopeChatAdminWrite))
	r.Delete("/gdm/id/{id}", m.deleteGDM, gate.User(m.s.gate, scopeChatAdminWrite))
	r.Patch("/gdm/id/{id}/member/add", m.addGDMMember, gate.User(m.s.gate, scopeChatAdminWrite))
	r.Patch("/gdm/id/{id}/member/rm", m.rmGDMMembers, gate.User(m.s.gate, scopeChatAdminWrite))
	r.Post("/gdm/id/{id}/msg", m.createGDMMsg, gate.User(m.s.gate, scopeChatWrite))
	r.Get("/gdm/id/{id}/msg", m.getGDMMsgs, gate.User(m.s.gate, scopeChatRead))
	r.Delete("/gdm/id/{id}/msg/id/{msgid}", m.deleteGDMMsg, gate.User(m.s.gate, scopeChatWrite))
}
