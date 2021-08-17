package conduit

import (
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
)

//go:generate forge validation -o validation_conduit_gen.go reqChatID reqCreateChat

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
	res, err := m.s.CreateChatWithUsers(req.Kind, req.Name, req.Theme, req.Userids)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusCreated, res)
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

func (m *router) conduitOwner(c governor.Context, userid string) bool {
	chatid := c.Param("id")
	if err := validhasChatid(chatid); err != nil {
		return false
	}
	members, err := m.s.GetChatMembers(chatid, []string{userid})
	if err != nil {
		m.s.logger.Error("Failed to get chat owners", map[string]string{
			"error":      err.Error(),
			"actiontype": "getchatowners",
		})
		return false
	}
	return len(members) != 0
}

const (
	scopeChatWrite = "gov.conduit.chat:write"
)

func (m *router) mountRoutes(r governor.Router) {
	r.Post("/chat", m.createChat, gate.User(m.s.gate, scopeChatWrite))
	r.Delete("/chat/id/{id}", m.deleteChat, gate.Owner(m.s.gate, m.conduitOwner, scopeChatWrite))
}
