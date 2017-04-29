package conf

import (
	"database/sql"
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
		Username string `json:"username"`
		Password string `json:"password"`
		Email    string `json:"email"`
		Orgname  string `json:"orgname"`
	}
)

var (
	emailRegex = regexp.MustCompile(`^[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]+$`)
)

func (r *requestSetupPost) valid() error {
	if len(r.Username) == 0 {
		return governor.NewError("username must be provided", 0, http.StatusBadRequest)
	}
	if len(r.Password) < 10 {
		return governor.NewError("password must be longer than 9 chars", 0, http.StatusBadRequest)
	}
	if !emailRegex.MatchString(r.Email) {
		return governor.NewError("email is invalid", 0, http.StatusBadRequest)
	}
	if len(r.Orgname) == 0 {
		return governor.NewError("organization name must be provided", 0, http.StatusBadRequest)
	}
	return nil
}

type (
	responseSetupPost struct {
		Username string `json:"username"`
		Version  string `json:"version"`
		Orgname  string `json:"orgname"`
	}
)

// Mount is a collection of routes for admins
func (h *Conf) Mount(conf governor.Config, r *echo.Group, db *sql.DB, l *logrus.Logger) error {
	r.POST("/setup", func(c echo.Context) error {
		rsetup := &requestSetupPost{}
		if err := c.Bind(rsetup); err != nil {
			return governor.NewError(err.Error(), 0, http.StatusBadRequest)
		}
		if err := rsetup.valid(); err != nil {
			return err
		}
		mconf, err := confmodel.New(rsetup.Orgname)
		if err != nil {
			l.WithFields(logrus.Fields{
				"service": "conf",
				"request": "setup",
				"action":  "new conf",
				"code":    err.Code(),
			}).Error(err)
			return err
		}
		l.WithFields(logrus.Fields{
			"service": "conf",
			"request": "setup",
			"action":  "new conf",
		}).Info("created new configuration model")

		madmin, err := usermodel.NewAdmin(rsetup.Username, rsetup.Password, rsetup.Email, "Admin", "")
		if err != nil {
			l.WithFields(logrus.Fields{
				"service": "conf",
				"request": "setup",
				"action":  "new admin",
				"code":    err.Code(),
			}).Error(err)
			return err
		}
		l.WithFields(logrus.Fields{
			"service": "conf",
			"request": "setup",
			"action":  "new admin",
		}).Info("created new admin model")

		if err := usermodel.Setup(db); err != nil {
			l.WithFields(logrus.Fields{
				"service": "conf",
				"request": "setup",
				"action":  "user setup",
				"code":    err.Code(),
			}).Error(err)
			return err
		}
		l.WithFields(logrus.Fields{
			"service": "conf",
			"request": "setup",
			"action":  "user setup",
		}).Info("created new user table")

		if err := confmodel.Setup(db); err != nil {
			l.WithFields(logrus.Fields{
				"service": "conf",
				"request": "setup",
				"action":  "conf setup",
				"code":    err.Code(),
			}).Error(err)
			return err
		}
		l.WithFields(logrus.Fields{
			"service": "conf",
			"request": "setup",
			"action":  "conf setup",
		}).Info("created new configuration table")

		if err := mconf.Insert(db); err != nil {
			l.WithFields(logrus.Fields{
				"service": "conf",
				"request": "setup",
				"action":  "insert conf",
				"code":    err.Code(),
			}).Error(err)
			return err
		}
		l.WithFields(logrus.Fields{
			"service": "conf",
			"request": "setup",
			"action":  "insert conf",
		}).Info("inserted new configuration into config")

		if err := madmin.Insert(db); err != nil {
			l.WithFields(logrus.Fields{
				"service": "conf",
				"request": "setup",
				"action":  "insert admin",
				"code":    err.Code(),
			}).Error(err)
			return err
		}
		l.WithFields(logrus.Fields{
			"service": "conf",
			"request": "setup",
			"action":  "insert admin",
		}).Info("inserted new admin into users")

		t, _ := time.Now().MarshalText()
		l.WithFields(logrus.Fields{
			"time":    string(t),
			"service": "conf",
			"request": "setup",
			"action":  "setup",
		}).Info("successfully setup database")

		return c.JSON(http.StatusCreated, &responseSetupPost{
			Username: madmin.Username,
			Version:  conf.Version,
			Orgname:  mconf.Orgname,
		})
	})
	return nil
}
