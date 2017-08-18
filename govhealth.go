package governor

import (
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"net/http"
	"time"
)

const (
	moduleIDHealth = "govhealth"
)

type (
	// Health is a health service for monitoring
	health struct {
		services []Service
	}

	errRes struct {
		Origin  string `json:"origin"`
		Source  string `json:"source"`
		Code    int    `json:"code"`
		Message string `json:"message"`
	}

	healthRes struct {
		Time string   `json:"time"`
		Errs []errRes `json:"errs"`
	}
)

func newHealth() *health {
	return &health{
		services: []Service{},
	}
}

// Mount is a collection of routes for healthchecks
func (h *health) Mount(conf Config, r *echo.Group, l *logrus.Logger) error {
	r.GET("/check", func(c echo.Context) error {
		t, _ := time.Now().MarshalText()
		if errs := h.check(); len(errs) > 0 {
			errReslist := []errRes{}
			for _, i := range errs {
				errReslist = append(errReslist, errRes{
					Origin:  i.Origin(),
					Source:  i.Source(),
					Code:    i.Code(),
					Message: i.Message(),
				})
			}
			return c.JSON(http.StatusServiceUnavailable, &healthRes{
				Time: string(t),
				Errs: errReslist,
			})
		}
		return c.JSON(http.StatusOK, &healthRes{
			Time: string(t),
			Errs: nil,
		})
	})
	if conf.IsDebug() {
		r.GET("/version", func(c echo.Context) error {
			return c.String(http.StatusOK, conf.Version)
		})
		r.GET("/ping", func(c echo.Context) error {
			t, _ := time.Now().MarshalText()
			l.WithFields(logrus.Fields{
				"time":     string(t),
				"service":  "health",
				"request":  "ping",
				"response": "pong",
			}).Info("Ping")
			return c.String(http.StatusOK, "Pong")
		})
		r.GET("/error", func(c echo.Context) error {
			return NewError(moduleIDHealth, "test error", 0, http.StatusBadRequest)
		})
	}

	l.Info("mounted health checkpoint")
	return nil
}

func (h *health) addService(s Service) {
	h.services = append(h.services, s)
}

func (h *health) check() []*Error {
	k := []*Error{}
	for _, i := range h.services {
		if err := i.Health(); err != nil {
			k = append(k, err)
		}
	}
	return k
}
