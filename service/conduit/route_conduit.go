package conduit

import (
	"net/http"
	"strings"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
)

//go:generate forge validation -o validation_conduit_gen.go reqGetFriends reqRmFriend reqAcceptFriendInvitation reqDelFriendInvitation reqGetFriendInvitations reqGetLatestChats reqGetChats reqSearchDMs reqUpdateDM reqCreateDMMsg reqGetDMMsgs reqDelDMMsg reqChatID reqCreateChat reqUpdateChat reqChatMembers reqLatestChats reqSearchChats reqChats reqCreateMsg reqLatestMsgs

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

type (
	reqSearchDMs struct {
		Userid string `valid:"userid,has" json:"-"`
		Prefix string `valid:"username,has" json:"-"`
		Amount int    `valid:"amount" json:"-"`
	}
)

func (m *router) searchDMs(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqSearchDMs{
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
	reqCreateDMMsg struct {
		Userid string `valid:"userid,has" json:"-"`
		Chatid string `valid:"chatid,has" json:"-"`
		Kind   string `valid:"msgkind" json:"kind"`
		Value  string `valid:"msgvalue" json:"value"`
	}
)

func (m *router) createDMMsg(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqCreateDMMsg{}
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
	reqGetDMMsgs struct {
		Userid string `valid:"userid,has" json:"-"`
		Chatid string `valid:"chatid,has" json:"-"`
		Kind   string `valid:"msgkind,opt" json:"-"`
		Before string `valid:"msgid,opt" json:"-"`
		Amount int    `valid:"amount" json:"-"`
	}
)

func (m *router) getDMMsgs(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqGetDMMsgs{
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
	reqDelDMMsg struct {
		Userid string `valid:"userid,has" json:"-"`
		Chatid string `valid:"chatid,has" json:"-"`
		Msgid  string `valid:"msgid,has" json:"-"`
	}
)

func (m *router) deleteMsg(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqDelDMMsg{
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
	reqChatID struct {
		Chatid string `valid:"chatid,has" json:"-"`
	}

	reqCreateChat struct {
		Kind    string   `valid:"kind" json:"kind"`
		Name    string   `valid:"name" json:"name"`
		Theme   string   `valid:"theme" json:"theme"`
		Userids []string `valid:"userids,has" json:"userids"`
	}

	reqUpdateChat struct {
		Chatid string `valid:"chatid,has" json:"-"`
		Name   string `valid:"name" json:"name"`
		Theme  string `valid:"theme" json:"theme"`
	}

	reqChatMembers struct {
		Chatid string   `valid:"chatid,has" json:"-"`
		Add    []string `valid:"userids,opt" json:"add"`
		Remove []string `valid:"userids,opt" json:"remove"`
	}

	reqLatestChats struct {
		Kind   string `valid:"kind,has" json:"-"`
		Before int64  `json:"-"`
		Amount int    `valid:"amount" json:"-"`
	}

	reqSearchChats struct {
		Kind   string `valid:"kind,has" json:"-"`
		Search string `valid:"search" json:"-"`
		Amount int    `valid:"amount" json:"-"`
	}

	reqChats struct {
		Chatids []string `valid:"chatids,has" json:"-"`
	}

	reqCreateMsg struct {
		Chatid string `valid:"chatid,has" json:"-"`
		Kind   string `valid:"msgkind" json:"kind"`
		Value  string `valid:"msgvalue" json:"value"`
	}

	reqLatestMsgs struct {
		Chatid string `valid:"chatid,has" json:"-"`
		Kind   string `valid:"msgkind,opt" json:"-"`
		Before string `valid:"msgid,opt" json:"-"`
		Amount int    `valid:"amount" json:"-"`
	}
)

func (m *router) createChat(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqCreateChat{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	members := make([]string, 0, len(req.Userids)+1)
	members[0] = gate.GetCtxUserid(c)
	copy(members[1:], req.Userids)
	res, err := m.s.CreateChatWithUsers(req.Kind, req.Name, req.Theme, members)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusCreated, res)
}

func (m *router) updateChat(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqUpdateChat{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	req.Chatid = c.Param("id")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := m.s.UpdateChat(req.Chatid, req.Name, req.Theme); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (m *router) updateChatMembers(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqChatMembers{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	req.Chatid = c.Param("id")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if len(req.Add) > 0 {
		if err := m.s.AddChatMembers(req.Chatid, req.Add); err != nil {
			c.WriteError(err)
			return
		}
	}
	if len(req.Remove) > 0 {
		if err := m.s.RemoveChatMembers(req.Chatid, req.Remove); err != nil {
			c.WriteError(err)
			return
		}
	}
	c.WriteStatus(http.StatusNoContent)
}

func (m *router) deleteChat(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqChatID{
		Chatid: c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := m.s.DeleteChat(req.Chatid); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (m *router) getLatestChats(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqLatestChats{
		Kind:   c.Query("kind"),
		Before: c.QueryInt64("before", 0),
		Amount: c.QueryInt("amount", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.GetLatestChatsByKind(req.Kind, gate.GetCtxUserid(c), req.Before, req.Amount)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (m *router) searchChats(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqSearchChats{
		Kind:   c.Query("kind"),
		Search: c.Query("search"),
		Amount: c.QueryInt("amount", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.SearchChats(req.Kind, gate.GetCtxUserid(c), req.Search, req.Amount)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (m *router) getChats(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqChats{
		Chatids: strings.Split(c.Query("ids"), ","),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.GetChats(req.Chatids)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (m *router) createMsg(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqCreateMsg{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	req.Chatid = c.Param("id")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := m.s.CreateMsg(req.Chatid, gate.GetCtxUserid(c), req.Kind, req.Value)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusCreated, res)
}

func (m *router) getLatestMsgs(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqLatestMsgs{
		Chatid: c.Param("id"),
		Kind:   c.Query("kind"),
		Before: c.Query("before"),
		Amount: c.QueryInt("amount", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := m.s.GetLatestChatMsgsByKind(req.Chatid, req.Kind, req.Before, req.Amount)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (m *router) conduitChatOwner(c governor.Context, userid string) bool {
	chatid := c.Param("id")
	if err := validhasChatid(chatid); err != nil {
		return false
	}
	members, err := m.s.GetUserChats(userid, []string{chatid})
	if err != nil {
		m.s.logger.Error("Failed to get chat owners", map[string]string{
			"error":      err.Error(),
			"actiontype": "getchatowners",
		})
		return false
	}
	return len(members) != 0
}

func (m *router) conduitChatsOwner(c governor.Context, userid string) bool {
	chatids := strings.Split(c.Query("ids"), ",")
	if err := validhasChatids(chatids); err != nil {
		return false
	}
	members, err := m.s.GetUserChats(userid, chatids)
	if err != nil {
		m.s.logger.Error("Failed to get user chats", map[string]string{
			"error":      err.Error(),
			"actiontype": "getuserchats",
		})
		return false
	}
	return len(members) == len(chatids)
}

func (m *router) mountRoutes(r governor.Router) {
	scopeFriendRead := m.s.scopens + ".friend:read"
	scopeFriendWrite := m.s.scopens + ".friend:write"
	r.Get("/friend", m.getFriends, gate.User(m.s.gate, scopeFriendRead))
	r.Delete("/friend/id/{id}", m.removeFriend, gate.User(m.s.gate, scopeFriendWrite))
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
	r.Put("/dm/id/{id}", m.updateDM, gate.User(m.s.gate, scopeChatAdminWrite))
	r.Get("/dm/search", m.searchDMs, gate.User(m.s.gate, scopeChatRead))
	r.Post("/dm/id/{id}/msg", m.createDMMsg, gate.User(m.s.gate, scopeChatWrite))
	r.Get("/dm/id/{id}/msg", m.getDMMsgs, gate.User(m.s.gate, scopeChatRead))
	r.Delete("/dm/id/{id}/msg/id/{msgid}", m.deleteMsg, gate.User(m.s.gate, scopeChatWrite))

	r.Get("/chat/latest", m.getLatestChats, gate.User(m.s.gate, scopeChatRead))
	r.Get("/chat/search", m.searchChats, gate.User(m.s.gate, scopeChatRead))
	r.Get("/chat", m.getChats, gate.Owner(m.s.gate, m.conduitChatsOwner, scopeChatRead))
	r.Post("/chat", m.createChat, gate.User(m.s.gate, scopeChatAdminWrite))
	r.Put("/chat/id/{id}", m.updateChat, gate.Owner(m.s.gate, m.conduitChatOwner, scopeChatAdminWrite))
	r.Patch("/chat/id/{id}/member", m.updateChatMembers, gate.Owner(m.s.gate, m.conduitChatOwner, scopeChatAdminWrite))
	r.Delete("/chat/id/{id}", m.deleteChat, gate.Owner(m.s.gate, m.conduitChatOwner, scopeChatAdminWrite))
	r.Post("/chat/id/{id}/msg", m.createMsg, gate.Owner(m.s.gate, m.conduitChatOwner, scopeChatWrite))
	r.Get("/chat/id/{id}/msg/latest", m.getLatestMsgs, gate.Owner(m.s.gate, m.conduitChatOwner, scopeChatRead))
}
