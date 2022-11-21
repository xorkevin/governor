package governor

import (
	"net/http"

	"xorkevin.dev/klog"
)

type (
	healthVersionRes struct {
		Version string `json:"version"`
	}
)

func (s *Server) initHealth(r Router) {
	m := NewMethodRouter(r)
	m.GetCtx("/live", func(c *Context) {
		c.WriteStatus(http.StatusOK)
	})

	m.GetCtx("/ready", func(c *Context) {
		errs := s.checkHealthServices(c.Ctx())
		status := http.StatusOK
		if len(errs) > 0 {
			status = http.StatusInternalServerError
			errstrs := make([]string, 0, len(errs))
			for _, i := range errs {
				errstrs = append(errstrs, i.Error())
			}
			s.log.Error(c.Ctx(), "Failed readiness check", klog.Fields{
				"errors": errstrs,
			})
		}
		c.WriteStatus(status)
	})

	m.GetCtx("/version", func(c *Context) {
		c.WriteJSON(http.StatusOK, healthVersionRes{
			Version: s.settings.config.Version.String(),
		})
	})
}
