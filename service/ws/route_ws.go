package ws

import (
	"encoding/json"
	"net/http"

	"nhooyr.io/websocket"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
)

func (m *router) echo(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	conn, err := c.Websocket()
	if err != nil {
		m.s.logger.Debug("Failed to accept WS conn", map[string]string{
			"msg": err.Error(),
		})
		return
	}
	defer conn.Close(int(websocket.StatusInternalError), "Internal error")

	for {
		t, b, err := conn.Read(c.Ctx())
		if err != nil {
			m.s.logger.Debug("WS conn closed", map[string]string{
				"msg": err.Error(),
			})
			return
		}
		if !t {
			conn.CloseError(governor.ErrWS(
				governor.NewError(governor.ErrOptUser),
				int(websocket.StatusUnsupportedData),
				"Invalid msg type binary",
			))
			return
		}
		channel, msg, err := DecodeRcvMsg(b)
		if err != nil {
			conn.CloseError(governor.ErrWS(
				governor.ErrWithKind(err, governor.ErrorUser{}, "Failed to decode received msg"),
				int(websocket.StatusUnsupportedData),
				"Invalid msg format",
			))
			return
		}
		if channel != "echo" {
			conn.CloseError(governor.ErrWS(
				governor.NewError(governor.ErrOptUser),
				int(websocket.StatusUnsupportedData),
				"Invalid msg channel",
			))
			return
		}
		res, err := json.Marshal(RcvMsg{
			Channel: channel,
			Value:   msg,
		})
		if err != nil {
			conn.CloseError(governor.ErrWithMsg(err, "Failed to marshal sent json msg"))
			return
		}
		if err := conn.Write(c.Ctx(), true, res); err != nil {
			m.s.logger.Debug("WS conn closed", map[string]string{
				"msg": err.Error(),
			})
			return
		}
	}
}

func (m *router) mountRoutes(r governor.Router) {
	// scopeWS := m.s.scopens + ".ws:read"
	scopeWSWrite := m.s.scopens + ".ws:write"
	r.Any("/echo", m.echo, gate.Member(m.s.gate, m.s.rolens, scopeWSWrite))
}
