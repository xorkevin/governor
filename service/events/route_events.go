package events

import (
	"crypto/subtle"
	"net/http"

	"xorkevin.dev/governor"
)

//go:generate forge validation -o validation_events_gen.go reqPublishEvent

type (
	reqPublishEvent struct {
		Subject string `valid:"subject" json:"-"`
	}
)

func (m *router) publishEvent(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)

	username, password, ok := c.BasicAuth()
	if !ok || username != "system" || subtle.ConstantTimeCompare([]byte(password), []byte(m.s.apisecret)) != 1 {
		c.WriteError(governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusForbidden,
			Message: "User is forbidden",
		})))
		return
	}

	req := reqPublishEvent{
		Subject: c.Query("subject"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	data, err := c.ReadAllBody()
	if err != nil {
		c.WriteError(err)
		return
	}
	if err := m.s.Publish(req.Subject, data); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusOK)
}

func (m *router) mountRoutes(r governor.Router) {
	r.Post("/publish", m.publishEvent)
}
