package governor

import (
	"net/http"
	"time"
)

type (
	errRes struct {
		Message string `json:"message"`
	}

	healthRes struct {
		Time int64    `json:"time"`
		Errs []errRes `json:"errs"`
	}

	reqTestPost struct {
		Test string `json:"test"`
	}
)

type (
	// ErrHealth is returned for a failed health check
	ErrHealth struct{}
)

func (e ErrHealth) Error() string {
	return "Error health check"
}

func (s *Server) initHealth(m Router) {
	m.Get("/live", func(w http.ResponseWriter, r *http.Request) {
		c := NewContext(w, r, s.logger)
		c.WriteStatus(http.StatusOK)
	})

	m.Get("/ready", func(w http.ResponseWriter, r *http.Request) {
		t := time.Now().Round(0).Unix()
		errs := s.checkHealthServices()
		errReslist := make([]errRes, 0, len(errs))
		for _, i := range errs {
			errReslist = append(errReslist, errRes{
				Message: i.Error(),
			})
		}
		c := NewContext(w, r, s.logger)
		status := http.StatusOK
		if len(errs) > 0 {
			status = http.StatusInternalServerError
		}
		c.WriteJSON(status, &healthRes{
			Time: t,
			Errs: errReslist,
		})
	})

	if s.config.IsDebug() {
		m.Get("/version", func(w http.ResponseWriter, r *http.Request) {
			c := NewContext(w, r, s.logger)
			c.WriteString(http.StatusOK, s.config.version.String())
		})

		m.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
			s.logger.Info("Ping", map[string]string{
				"request":  "ping",
				"response": "pong",
			})
			c := NewContext(w, r, s.logger)
			c.WriteString(http.StatusOK, "Pong")
		})

		m.Get("/error", func(w http.ResponseWriter, r *http.Request) {
			c := NewContext(w, r, s.logger)
			c.WriteError(NewError(ErrOptRes(ErrorRes{
				Status:  http.StatusTeapot,
				Code:    "test_error",
				Message: "Test error",
			})))
		})

		m.Post("/testpost", func(w http.ResponseWriter, r *http.Request) {
			c := NewContext(w, r, s.logger)
			req := reqTestPost{}
			if err := c.Bind(&req); err != nil {
				c.WriteError(err)
				return
			}
			c.WriteString(http.StatusOK, "Hello, World!")
		})
	}
}
