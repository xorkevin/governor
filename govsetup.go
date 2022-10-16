package governor

import (
	"net/http"

	"xorkevin.dev/klog"
)

type (
	// ReqSetup is a service setup request
	ReqSetup struct {
	}
)

const (
	lengthCapSetupSecret = 255
)

func setupSecretValid(secret string) error {
	if len(secret) > lengthCapSetupSecret {
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
		c.LogFields(klog.Fields{
			"gov.service.phase": "setup",
		})
		username, password, ok := c.BasicAuth()
		if !ok || username != "setup" {
			c.WriteError(ErrWithRes(nil, http.StatusForbidden, "", "Invalid setup secret"))
			return
		}
		if err := setupSecretValid(password); err != nil {
			c.WriteError(err)
			return
		}
		if err := s.setupServices(c.Ctx(), password, ReqSetup{}); err != nil {
			c.WriteError(err)
			return
		}
		c.WriteJSON(http.StatusCreated, &ResSetup{
			Version: s.config.version.Num,
		})
	})
}
