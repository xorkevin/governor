package conduit

import (
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
)

//go:generate forge validation -o validation_conduit_gen.go reqCreateChat

type (
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

const (
	scopeChatWrite = "gov.conduit.chat:write"
)

func (m *router) mountRoutes(r governor.Router) {
	r.Post("/chat", m.createChat, gate.User(m.s.gate, scopeChatWrite))
}
