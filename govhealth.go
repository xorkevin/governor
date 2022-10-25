package governor

import (
	"net/http"
	"time"
)

type (
	healthErrorRes struct {
		Message string `json:"message"`
	}

	healthRes struct {
		Time     string           `json:"time"`
		UnixTime int64            `json:"unixtime"`
		Errs     []healthErrorRes `json:"errs"`
	}
)

func (s *Server) initHealth(r Router) {
	m := NewMethodRouter(r)
	m.GetCtx("/live", func(c *Context) {
		c.WriteStatus(http.StatusOK)
	})

	m.GetCtx("/ready", func(c *Context) {
		t := time.Now().Round(0)
		errs := s.checkHealthServices(c.Ctx())
		errReslist := make([]healthErrorRes, 0, len(errs))
		for _, i := range errs {
			errReslist = append(errReslist, healthErrorRes{
				Message: i.Error(),
			})
		}
		status := http.StatusOK
		if len(errs) > 0 {
			status = http.StatusInternalServerError
		}
		c.WriteJSON(status, &healthRes{
			Time:     t.Format(time.RFC3339),
			UnixTime: t.Unix(),
			Errs:     errReslist,
		})
	})

	m.GetCtx("/version", func(c *Context) {
		c.WriteString(http.StatusOK, s.config.version.String())
	})
}
