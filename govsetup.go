package governor

import (
	"net/http"

	"xorkevin.dev/klog"
)

type (
	// ReqSetup is a service setup request
	ReqSetup struct {
		Secret string `json:"secret,omitempty"`
	}
)

const (
	lengthCapSetupSecret = 255
)

func (r ReqSetup) valid() error {
	if len(r.Secret) > lengthCapSetupSecret {
		return ErrWithRes(nil, http.StatusBadRequest, "", "Secret must be shorter than 256 chars")
	}
	return nil
}

type (
	// ResSetup is the response to a setup request
	ResSetup struct {
		Version string `json:"version"`
	}
)

func (s *Server) initSetup(r Router) {
	m := NewMethodRouter(r)
	m.PostCtx("", func(c Context) {
		var req ReqSetup
		if err := c.Bind(&req, false); err != nil {
			c.WriteError(err)
			return
		}
		if err := req.valid(); err != nil {
			c.WriteError(err)
			return
		}
		c.LogFields(klog.Fields{
			"gov.service.phase": "setup",
		})
		if err := s.setupServices(c.Ctx(), req); err != nil {
			c.WriteError(err)
			return
		}

		c.WriteJSON(http.StatusCreated, &ResSetup{
			Version: s.config.version.Num,
		})
	})
}
