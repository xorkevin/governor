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

	responseUserPost struct {
		Userid    string `json:"userid"`
		Username  string `json:"username"`
		Firstname string `json:"first_name"`
		Lastname  string `json:"last_name"`
	}
)

func (r *requestUserPost) valid() *governor.Error {
	if len(r.Username) < 3 || len(r.Username) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "username must be longer than 2 chars", 0, http.StatusBadRequest)
	}
	if len(r.Password) < 10 || len(r.Password) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "password must be longer than 9 chars", 0, http.StatusBadRequest)
	}
	if !emailRegex.MatchString(r.Email) || len(r.Email) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "email is invalid", 0, http.StatusBadRequest)
	}
	if len(r.FirstName) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "first name is too long", 0, http.StatusBadRequest)
	}
	if len(r.LastName) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "last name is too long", 0, http.StatusBadRequest)
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

		return c.JSON(http.StatusCreated, &responseUserPost{
			Userid:    userid,
			Username:  m.Username,
			Firstname: m.FirstName,
			Lastname:  m.LastName,
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

	return nil
}
