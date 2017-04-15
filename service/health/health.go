package health

import (
	"database/sql"
	"github.com/hackform/governor"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"net/http"
	"time"
)

type (
	// Health is a health service for monitoring
	Health struct {
	}
)

// New creates a new Health service
func New() *Health {
	return &Health{}
}

// Mount is a collection of routes for healthchecks
func (h *Health) Mount(conf governor.Config, r *echo.Group, db *sql.DB, l *logrus.Logger) error {
	r.GET("/check", func(c echo.Context) error {
		t, err := time.Now().MarshalText()
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.String(http.StatusOK, string(t))
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
	}
	return nil
}
