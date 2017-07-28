package conf

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/conf/model"
	"github.com/hackform/governor/service/db"
	"github.com/hackform/governor/service/post/model"
	"github.com/hackform/governor/service/profile/model"
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
		db *db.Database
	}
)

// New creates a new Conf service
func New(l *logrus.Logger, database *db.Database) *Conf {
	l.Info("initialized conf service")

	return &Conf{
		db: database,
	}
}

type (
	requestSetupPost struct {
		Username  string `json:"username"`
		Password  string `json:"password"`
		Email     string `json:"email"`
		Firstname string `json:"first_name"`
		Lastname  string `json:"last_name"`
		Orgname   string `json:"orgname"`
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
		Username  string `json:"username"`
		Firstname string `json:"first_name"`
		Lastname  string `json:"last_name"`
		Version   string `json:"version"`
		Orgname   string `json:"orgname"`
	}
)

const (
	moduleIDSetup = moduleID + ".setup"
)

// Mount is a collection of routes for admins
func (c *Conf) Mount(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
	sdb := c.db.DB()
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

		madmin, err := usermodel.NewAdmin(rsetup.Username, rsetup.Password, rsetup.Email, rsetup.Firstname, rsetup.Lastname)
		if err != nil {
			err.AddTrace(moduleIDSetup)
			return err
		}
		lsetup.Info("created new admin model")

		if err := usermodel.Setup(sdb); err != nil {
			err.AddTrace(moduleIDSetup)
			return err
		}
		lsetup.Info("created new user table")

		if err := profilemodel.Setup(sdb); err != nil {
			err.AddTrace(moduleIDSetup)
			return err
		}
		lsetup.Info("created new profile table")

		if err := postmodel.Setup(sdb); err != nil {
			err.AddTrace(moduleIDSetup)
			return err
		}
		lsetup.Info("created new post, comment, and vote table")

		if err := confmodel.Setup(sdb); err != nil {
			err.AddTrace(moduleIDSetup)
			return err
		}
		lsetup.Info("created new configuration table")

		if err := mconf.Insert(sdb); err != nil {
			err.AddTrace(moduleIDSetup)
			return err
		}
		lsetup.Info("inserted new configuration into config")

		if err := madmin.Insert(sdb); err != nil {
			err.AddTrace(moduleIDSetup)
			return err
		}
		userid, _ := madmin.IDBase64()
		lsetup.WithFields(logrus.Fields{
			"username": madmin.Username,
			"userid":   userid,
		}).Info("inserted new admin into users")

		t, _ := time.Now().MarshalText()
		lsetup.WithFields(logrus.Fields{
			"time": string(t),
		}).Info("successfully setup database")

		return c.JSON(http.StatusCreated, &responseSetupPost{
			Username:  madmin.Username,
			Firstname: madmin.FirstName,
			Lastname:  madmin.LastName,
			Version:   conf.Version,
			Orgname:   mconf.Orgname,
		})
	})

	l.Info("mounted conf service")

	return nil
}

// Health is a check for service health
func (c *Conf) Health() *governor.Error {
	return nil
}
