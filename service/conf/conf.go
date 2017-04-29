package conf

import (
	"database/sql"
	"fmt"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/conf/model"
	"github.com/hackform/governor/service/user/model"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"net/http"
	"regexp"
	"time"
)

type (
	// Conf is a configuration service for admins
	Conf struct {
	}
)

// New creates a new Conf service
func New() *Conf {
	return &Conf{}
}

type (
	requestSetupPost struct {
		Password string `json:"password"`
		Email    string `json:"email"`
		Orgname  string `json:"orgname"`
	}
)

var (
	emailRegex = regexp.MustCompile(`^[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]+$`)
)

func (r *requestSetupPost) valid() error {
	if len(r.Password) < 10 {
		return fmt.Errorf("password must be longer than 9 chars")
	}
	if !emailRegex.MatchString(r.Email) {
		return fmt.Errorf("email is invalid")
	}
	if len(r.Orgname) == 0 {
		return fmt.Errorf("organization name must be provided")
	}
	return nil
}

type (
	responseSetupPost struct {
		Version int    `json:"version"`
		Orgname string `json:"orgname"`
	}
)

// Mount is a collection of routes for admins
func (h *Conf) Mount(conf governor.Config, r *echo.Group, db *sql.DB, l *logrus.Logger) error {
	r.POST("/setup", func(c echo.Context) error {
		rsetup := &requestSetupPost{}
		if err := c.Bind(rsetup); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		if err := rsetup.valid(); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		mconf, err := confmodel.New(rsetup.Orgname)
		if err != nil {
			l.WithFields(logrus.Fields{
				"service": "conf",
				"request": "setup",
				"action":  "new conf",
			}).Error(err)
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		madmin, err := usermodel.NewAdmin("admin", rsetup.Password, rsetup.Email, "Admin", "")
		if err != nil {
			l.WithFields(logrus.Fields{
				"service": "conf",
				"request": "setup",
				"action":  "new admin",
			}).Error(err)
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}

		if err := usermodel.Setup(db); err != nil {
			l.WithFields(logrus.Fields{
				"service": "conf",
				"request": "setup",
				"action":  "user setup",
			}).Error(err)
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if err := confmodel.Setup(db); err != nil {
			l.WithFields(logrus.Fields{
				"service": "conf",
				"request": "setup",
				"action":  "conf setup",
			}).Error(err)
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}

		if err := mconf.Insert(db); err != nil {
			l.WithFields(logrus.Fields{
				"service": "conf",
				"request": "setup",
				"action":  "insert conf",
			}).Error(err)
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if err := madmin.Insert(db); err != nil {
			l.WithFields(logrus.Fields{
				"service": "conf",
				"request": "setup",
				"action":  "insert admin",
			}).Error(err)
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}

		t, _ := time.Now().MarshalText()
		l.WithFields(logrus.Fields{
			"time":    string(t),
			"service": "conf",
			"request": "setup",
			"action":  "setup",
		}).Info("success")

		return c.JSON(http.StatusCreated, &responseSetupPost{
			Orgname: mconf.Orgname,
		})
	})
	return nil
}
