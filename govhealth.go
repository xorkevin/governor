package governor

import (
	"net/http"
	"time"
)

type (
	healthErrRes struct {
		Message string `json:"message"`
	}

	healthRes struct {
		Time int64          `json:"time"`
		Errs []healthErrRes `json:"errs"`
	}
)

func (s *Server) initHealth(m Router) {
	m.Get("/live", func(w http.ResponseWriter, r *http.Request) {
		c := NewContext(w, r, s.log.Logger)
		c.WriteStatus(http.StatusOK)
	})

	m.Get("/ready", func(w http.ResponseWriter, r *http.Request) {
		c := NewContext(w, r, s.log.Logger)
		t := time.Now().Round(0).Unix()
		errs := s.checkHealthServices(c.Ctx())
		errReslist := make([]healthErrRes, 0, len(errs))
		for _, i := range errs {
			errReslist = append(errReslist, healthErrRes{
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

	m.Get("/version", func(w http.ResponseWriter, r *http.Request) {
		c := NewContext(w, r, s.log.Logger)
		c.WriteString(http.StatusOK, s.config.version.String())
	})
}
