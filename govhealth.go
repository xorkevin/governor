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
		Time int64            `json:"time"`
		Errs []healthErrorRes `json:"errs"`
	}
)

func (s *Server) initHealth(r Router) {
	m := NewMethodRouter(r)
	m.GetCtx("/live", func(c Context) {
		c.WriteStatus(http.StatusOK)
	})

	m.GetCtx("/ready", func(c Context) {
		t := time.Now().Round(0).Unix()
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
			Time: t,
			Errs: errReslist,
		})
	})

	m.GetCtx("/version", func(c Context) {
		c.WriteString(http.StatusOK, s.config.version.String())
	})
}
