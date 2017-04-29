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

const (
	moduleID = "conf"
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

const (
	moduleIDReqValid = moduleID + ".reqvalid"
)

var (
	emailRegex = regexp.MustCompile(`^[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]+$`)
)

func (r *requestSetupPost) valid() *governor.Error {
	if len(r.Username) < 3 {
		return governor.NewErrorUser(moduleIDReqValid, "username must be longer than 2 chars", 0, http.StatusBadRequest)
	}
	if len(r.Password) < 10 {
		return governor.NewErrorUser(moduleIDReqValid, "password must be longer than 9 chars", 0, http.StatusBadRequest)
	}
	if !emailRegex.MatchString(r.Email) {
		return governor.NewErrorUser(moduleIDReqValid, "email is invalid", 0, http.StatusBadRequest)
	}
	if len(r.Orgname) == 0 {
		return governor.NewErrorUser(moduleIDReqValid, "organization name must be provided", 0, http.StatusBadRequest)
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

const (
	moduleIDSetup = moduleID + ".setup"
)

// Mount is a collection of routes for admins
func (h *Conf) Mount(conf governor.Config, r *echo.Group, db *sql.DB, l *logrus.Logger) error {
	lsetup := l.WithFields(logrus.Fields{
		"origin": moduleIDSetup,
	})
	r.POST("/setup", func(c echo.Context) error {
		rsetup := &requestSetupPost{}
		if err := c.Bind(rsetup); err != nil {
			return governor.NewErrorUser(moduleIDSetup, err.Error(), 0, http.StatusBadRequest)
		}
		if err := rsetup.valid(); err != nil {
			return err
		}
		mconf, err := confmodel.New(rsetup.Orgname)
		if err != nil {
			err.AddTrace(moduleIDSetup)
			return err
		}
		lsetup.Info("created new configuration model")

		madmin, err := usermodel.NewAdmin(rsetup.Username, rsetup.Password, rsetup.Email, "Admin", "")
		if err != nil {
			err.AddTrace(moduleIDSetup)
			return err
		}
		lsetup.Info("created new admin model")

		if err := usermodel.Setup(db); err != nil {
			err.AddTrace(moduleIDSetup)
			return err
		}
		lsetup.Info("created new user table")

		if err := confmodel.Setup(db); err != nil {
			err.AddTrace(moduleIDSetup)
			return err
		}
		lsetup.Info("created new configuration table")

		if err := mconf.Insert(db); err != nil {
			err.AddTrace(moduleIDSetup)
			return err
		}
		lsetup.Info("inserted new configuration into config")

		if err := madmin.Insert(db); err != nil {
			err.AddTrace(moduleIDSetup)
			return err
		}
		lsetup.Info("inserted new admin into users")

		t, _ := time.Now().MarshalText()
		lsetup.WithFields(logrus.Fields{
			"time": string(t),
		}).Info("successfully setup database")

		return c.JSON(http.StatusCreated, &responseSetupPost{
			Username: madmin.Username,
			Version:  conf.Version,
			Orgname:  mconf.Orgname,
		})
	})
	return nil
}
