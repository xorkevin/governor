package governor

import (
	"github.com/labstack/echo"
	"net/http"
	"strconv"
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
)

func (s *Server) initHealth(r *echo.Group) {
	r.GET("/check", func(c echo.Context) error {
		return c.String(http.StatusOK, strconv.FormatInt(time.Now().Unix(), 10))
	})

	r.GET("/report", func(c echo.Context) error {
		t := time.Now().Unix()
		if errs := s.checkHealthServices(); len(errs) > 0 {
			errReslist := []errRes{}
			for _, i := range errs {
				errReslist = append(errReslist, errRes{
					Message: i.Error(),
				})
			}
			return c.JSON(http.StatusServiceUnavailable, &healthRes{
				Time: t,
				Errs: errReslist,
			})
		}
		return c.JSON(http.StatusOK, &healthRes{
			Time: t,
			Errs: nil,
		})
	})

	if s.config.IsDebug() {
		r.GET("/version", func(c echo.Context) error {
			return c.String(http.StatusOK, s.config.Version)
		})

		r.GET("/ping", func(c echo.Context) error {
			s.logger.Debug("govhealth: Ping", map[string]string{
				"request":  "ping",
				"response": "pong",
			})
			return c.String(http.StatusOK, "Pong")
		})

		r.GET("/error", func(c echo.Context) error {
			return NewError("Test error", http.StatusBadRequest, nil)
		})
	}
}
