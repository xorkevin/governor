package conduit

import (
	"net/http"
	"strings"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
)

//go:generate forge validation -o validation_conduit_gen.go reqChatID reqCreateChat reqUpdateChat reqChatMembers reqLatestChats reqChats reqCreateMsg reqLatestMsgs

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
	res, err := m.s.CreateChatWithUsers(req.Kind, req.Name, req.Theme, append(req.Userids, gate.GetCtxUserid(c)))
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
	scopeChatRead := m.s.scopens + ".chat:read"
	scopeChatWrite := m.s.scopens + ".chat:write"
	scopeChatAdminWrite := m.s.scopens + ".chat.admin:write"
	r.Get("/chat/latest", m.getLatestChats, gate.User(m.s.gate, scopeChatRead))
	r.Get("/chat", m.getChats, gate.Owner(m.s.gate, m.conduitChatsOwner, scopeChatRead))
	r.Post("/chat", m.createChat, gate.User(m.s.gate, scopeChatAdminWrite))
	r.Put("/chat/id/{id}", m.updateChat, gate.Owner(m.s.gate, m.conduitChatOwner, scopeChatAdminWrite))
	r.Patch("/chat/id/{id}/member", m.updateChatMembers, gate.Owner(m.s.gate, m.conduitChatOwner, scopeChatAdminWrite))
	r.Delete("/chat/id/{id}", m.deleteChat, gate.Owner(m.s.gate, m.conduitChatOwner, scopeChatAdminWrite))
	r.Post("/chat/id/{id}/msg", m.createMsg, gate.Owner(m.s.gate, m.conduitChatOwner, scopeChatWrite))
	r.Get("/chat/id/{id}/msg/latest", m.getLatestMsgs, gate.Owner(m.s.gate, m.conduitChatOwner, scopeChatRead))
}
