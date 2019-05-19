package governor

import (
	"github.com/labstack/echo"
	"net/http"
	"time"
)

type (
	// Health is a health service for monitoring
	health struct {
		services []Service
	}

	errRes struct {
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
func (h *health) Mount(conf Config, l Logger, r *echo.Group) error {
	r.GET("/check", func(c echo.Context) error {
		t, _ := time.Now().MarshalText()
		if errs := h.check(); len(errs) > 0 {
			errReslist := []errRes{}
			for _, i := range errs {
				errReslist = append(errReslist, errRes{
					Message: i.Error(),
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
			l.Debug("govhealth: Ping", map[string]string{
				"request":  "ping",
				"response": "pong",
			})
			return c.String(http.StatusOK, "Pong")
		})
		r.GET("/error", func(c echo.Context) error {
			return NewError("Test error", http.StatusBadRequest, nil)
		})
	}

	l.Info("mount health service", nil)
	return nil
}

func (h *health) addService(s Service) {
	h.services = append(h.services, s)
}

func (h *health) check() []error {
	k := []error{}
	for _, i := range h.services {
		if err := i.Health(); err != nil {
			k = append(k, err)
		}
	}
	return k
}
