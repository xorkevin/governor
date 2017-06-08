package user

import (
	"database/sql"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/model"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"net/http"
	"time"
)

type (
	requestUserPost struct {
		Username  string `json:"username"`
		Password  string `json:"password"`
		Email     string `json:"email"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
	}

	requestUserPut struct {
		Username  string `json:"username"`
		Email     string `json:"email"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
	}

	responseUserUpdate struct {
		Userid   []byte `json:"userid"`
		Username string `json:"username"`
	}
)

func validUsername(username string) *governor.Error {
	if len(username) < 3 || len(username) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "username must be longer than 2 chars", 0, http.StatusBadRequest)
	}
	return nil
}

func validPassword(password string) *governor.Error {
	if len(password) < 10 || len(password) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "password must be longer than 9 chars", 0, http.StatusBadRequest)
	}
	return nil
}

func validEmail(email string) *governor.Error {
	if !emailRegex.MatchString(email) || len(email) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "email is invalid", 0, http.StatusBadRequest)
	}
	return nil
}

func validFirstName(firstname string) *governor.Error {
	if len(firstname) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "first name is too long", 0, http.StatusBadRequest)
	}
	return nil
}

func validLastName(lastname string) *governor.Error {
	if len(lastname) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "last name is too long", 0, http.StatusBadRequest)
	}
	return nil
}

func (r *requestUserPost) valid() *governor.Error {
	if err := validUsername(r.Username); err != nil {
		return err
	}
	if err := validPassword(r.Password); err != nil {
		return err
	}
	if err := validEmail(r.Email); err != nil {
		return err
	}
	if err := validFirstName(r.FirstName); err != nil {
		return err
	}
	if err := validLastName(r.LastName); err != nil {
		return err
	}
	return nil
}

func (r *requestUserPut) valid() *governor.Error {
	if err := validUsername(r.Username); err != nil {
		return err
	}
	if err := validEmail(r.Email); err != nil {
		return err
	}
	if err := validFirstName(r.FirstName); err != nil {
		return err
	}
	if err := validLastName(r.LastName); err != nil {
		return err
	}
	return nil
}

type (
	requestUserGetUsername struct {
		Username string `json:"username"`
	}

	requestUserGetID struct {
		Userid string `json:"userid"`
	}

	responseUserGetPublic struct {
		Username     string `json:"username"`
		Tags         string `json:"auth_tags"`
		FirstName    string `json:"first_name"`
		LastName     string `json:"last_name"`
		CreationTime int64  `json:"creation_time"`
	}

	responseUserGetPrivate struct {
		responseUserGetPublic
		Userid []byte `json:"userid"`
		Email  string `json:"email"`
	}
)

func (r *requestUserGetUsername) valid() *governor.Error {
	if len(r.Username) < 1 || len(r.Username) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "username must be provided", 0, http.StatusBadRequest)
	}
	return nil
}

func (r *requestUserGetID) valid() *governor.Error {
	if len(r.Userid) < 1 {
		return governor.NewErrorUser(moduleIDReqValid, "userid must be provided", 0, http.StatusBadRequest)
	}
	return nil
}

func mountRest(conf governor.Config, r *echo.Group, db *sql.DB, l *logrus.Logger) error {
	r.POST("", func(c echo.Context) error {
		ruser := &requestUserPost{}
		if err := c.Bind(ruser); err != nil {
			return governor.NewErrorUser(moduleIDUser, err.Error(), 0, http.StatusBadRequest)
		}
		if err := ruser.valid(); err != nil {
			return err
		}
		m, err := usermodel.NewBaseUser(ruser.Username, ruser.Password, ruser.Email, ruser.FirstName, ruser.LastName)
		if err != nil {
			err.AddTrace(moduleIDUser)
			return err
		}
		if err := m.Insert(db); err != nil {
			err.AddTrace(moduleIDUser)
			return err
		}

		t, _ := time.Now().MarshalText()
		userid, _ := m.IDBase64()
		l.WithFields(logrus.Fields{
			"time":     string(t),
			"origin":   moduleIDUser,
			"userid":   userid,
			"username": m.Username,
		}).Info("user created")

		return c.JSON(http.StatusCreated, &responseUserUpdate{
			Userid:   m.ID.Userid,
			Username: m.Username,
		})
	})

	ri := r.Group("/id")

	ri.GET("/:id", func(c echo.Context) error {
		ruser := &requestUserGetID{
			Userid: c.Param("id"),
		}
		if err := ruser.valid(); err != nil {
			return err
		}
		m, err := usermodel.GetByIDB64(db, ruser.Userid)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, &responseUserGetPublic{
			Username:     m.Username,
			Tags:         m.Tags,
			FirstName:    m.FirstName,
			LastName:     m.LastName,
			CreationTime: m.CreationTime,
		})
	})

	ri.GET("/:id/private", func(c echo.Context) error {
		ruser := &requestUserGetID{
			Userid: c.Param("id"),
		}
		if err := ruser.valid(); err != nil {
			return err
		}
		m, err := usermodel.GetByIDB64(db, ruser.Userid)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, &responseUserGetPrivate{
			responseUserGetPublic: responseUserGetPublic{
				Username:     m.Username,
				Tags:         m.Tags,
				FirstName:    m.FirstName,
				LastName:     m.LastName,
				CreationTime: m.CreationTime,
			},
			Userid: m.Userid,
			Email:  m.Email,
		})
	})

	ri.PUT("/:id", func(c echo.Context) error {
		reqid := &requestUserGetID{
			Userid: c.Param("id"),
		}
		if err := reqid.valid(); err != nil {
			return err
		}
		ruser := &requestUserPut{}
		if err := c.Bind(ruser); err != nil {
			return governor.NewErrorUser(moduleIDUser, err.Error(), 0, http.StatusBadRequest)
		}
		if err := ruser.valid(); err != nil {
			return err
		}
		m, err := usermodel.GetByIDB64(db, reqid.Userid)
		if err != nil {
			return err
		}
		m.Username = ruser.Username
		m.Email = ruser.Email
		m.FirstName = ruser.FirstName
		m.LastName = ruser.LastName
		if err = m.Update(db); err != nil {
			return err
		}
		return c.JSON(http.StatusCreated, &responseUserUpdate{
			Userid:   m.ID.Userid,
			Username: m.Username,
		})
	})

	rn := r.Group("/name")

	rn.GET("/:username", func(c echo.Context) error {
		ruser := &requestUserGetUsername{
			Username: c.Param("username"),
		}
		if err := ruser.valid(); err != nil {
			return err
		}
		m, err := usermodel.GetByUsername(db, ruser.Username)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, &responseUserGetPublic{
			Username:     m.Username,
			Tags:         m.Tags,
			FirstName:    m.FirstName,
			LastName:     m.LastName,
			CreationTime: m.CreationTime,
		})
	})

	if conf.IsDebug() {
		rn.GET("/:username/debug", func(c echo.Context) error {
			ruser := &requestUserGetUsername{
				Username: c.Param("username"),
			}
			if err := ruser.valid(); err != nil {
				return err
			}
			m, err := usermodel.GetByUsername(db, ruser.Username)
			if err != nil {
				return err
			}
			return c.JSON(http.StatusOK, &responseUserGetPrivate{
				responseUserGetPublic: responseUserGetPublic{
					Username:     m.Username,
					Tags:         m.Tags,
					FirstName:    m.FirstName,
					LastName:     m.LastName,
					CreationTime: m.CreationTime,
				},
				Userid: m.Userid,
				Email:  m.Email,
			})
		})
	}

	return nil
}
