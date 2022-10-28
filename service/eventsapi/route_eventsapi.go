package eventsapi

import (
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
)

type (
	//forge:valid
	reqPublishEvent struct {
		Subject string `valid:"subject" json:"-"`
	}
)

func (s *router) publishEvent(c *governor.Context) {
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
	if err := s.s.pubsub.Publish(c.Ctx(), req.Subject, data); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusOK)
}

func (s *router) mountRoutes(r *governor.MethodRouter) {
	scopePublish := s.s.scopens + ".publish:write"
	r.PostCtx("/pubsub/publish", s.publishEvent, gate.System(s.s.gate, scopePublish))
}
