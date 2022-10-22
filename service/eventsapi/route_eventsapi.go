package eventsapi

import (
	"crypto/subtle"
	"net/http"

	"xorkevin.dev/governor"
)

type (
	//forge:valid
	reqPublishEvent struct {
		Subject string `valid:"subject" json:"-"`
	}
)

func (m *router) publishEvent(c governor.Context) {
	username, password, ok := c.BasicAuth()
	if !ok || username != "system" {
		c.WriteError(governor.ErrWithRes(nil, http.StatusForbidden, "", "User is forbidden"))
		return
	}
	apisecret, err := m.s.getApiSecret(c.Ctx())
	if err != nil {
		c.WriteError(governor.ErrWithRes(err, http.StatusInternalServerError, "", "Unable to authenticate caller"))
		return
	}
	if subtle.ConstantTimeCompare([]byte(password), []byte(apisecret)) != 1 {
		c.WriteError(governor.ErrWithRes(nil, http.StatusForbidden, "", "User is forbidden"))
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
	if err := m.s.pubsub.Publish(c.Ctx(), req.Subject, data); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusOK)
}

func (m *router) mountRoutes(r *governor.MethodRouter) {
	r.PostCtx("/pubsub/publish", m.publishEvent)
}
