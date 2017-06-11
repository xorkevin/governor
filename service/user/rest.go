package user

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/model"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"net/http"
	"time"
)

type (
	reqUserPost struct {
		Username  string `json:"username"`
		Password  string `json:"password"`
		Email     string `json:"email"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
	}

	reqUserPut struct {
		Username  string `json:"username"`
		Email     string `json:"email"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
	}

	reqUserPutPassword struct {
		Password string `json:"password"`
	}

	reqUserPutRank struct {
		Rank string `json:"rank"`
	}

	resUserUpdate struct {
		Userid   []byte `json:"userid"`
		Username string `json:"username"`
	}
)

func (r *reqUserPost) valid() *governor.Error {
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

func (r *reqUserPut) valid() *governor.Error {
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

func (r *reqUserPutPassword) valid() *governor.Error {
	if err := validPassword(r.Password); err != nil {
		return err
	}
	return nil
}

type (
	reqUserGetUsername struct {
		Username string `json:"username"`
	}

	reqUserGetID struct {
		Userid string `json:"userid"`
	}

	resUserGetPublic struct {
		Username     string `json:"username"`
		Tags         string `json:"auth_tags"`
		FirstName    string `json:"first_name"`
		LastName     string `json:"last_name"`
		CreationTime int64  `json:"creation_time"`
	}

	resUserGet struct {
		resUserGetPublic
		Userid []byte `json:"userid"`
		Email  string `json:"email"`
	}
)

func (r *reqUserGetUsername) valid() *governor.Error {
	return hasUsername(r.Username)
}

func (r *reqUserGetID) valid() *governor.Error {
	return hasUserid(r.Userid)
}

func (u *User) mountRest(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
	db := u.db.DB()
	r.POST("", func(c echo.Context) error {
		ruser := &reqUserPost{}
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

		return c.JSON(http.StatusCreated, &resUserUpdate{
			Userid:   m.ID.Userid,
			Username: m.Username,
		})
	})

	ri := r.Group("/id")

	ri.GET("/:id", func(c echo.Context) error {
		ruser := &reqUserGetID{
			Userid: c.Param("id"),
		}
		if err := ruser.valid(); err != nil {
			return err
		}

		m, err := usermodel.GetByIDB64(db, ruser.Userid)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, &resUserGetPublic{
			Username:     m.Username,
			Tags:         m.Tags,
			FirstName:    m.FirstName,
			LastName:     m.LastName,
			CreationTime: m.CreationTime,
		})
	})

	ri.GET("/:id/private", func(c echo.Context) error {
		ruser := &reqUserGetID{
			Userid: c.Param("id"),
		}
		if err := ruser.valid(); err != nil {
			return err
		}

		m, err := usermodel.GetByIDB64(db, ruser.Userid)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, &resUserGet{
			resUserGetPublic: resUserGetPublic{
				Username:     m.Username,
				Tags:         m.Tags,
				FirstName:    m.FirstName,
				LastName:     m.LastName,
				CreationTime: m.CreationTime,
			},
			Userid: m.Userid,
			Email:  m.Email,
		})
	}, u.gate.OwnerOrAdmin("id"))

	ri.PUT("/:id", func(c echo.Context) error {
		reqid := &reqUserGetID{
			Userid: c.Param("id"),
		}
		if err := reqid.valid(); err != nil {
			return err
		}
		ruser := &reqUserPut{}
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
			err.AddTrace(moduleIDUser)
			return err
		}
		return c.JSON(http.StatusCreated, &resUserUpdate{
			Userid:   m.ID.Userid,
			Username: m.Username,
		})
	}, u.gate.Owner("id"))

	ri.PUT("/:id/password", func(c echo.Context) error {
		reqid := &reqUserGetID{
			Userid: c.Param("id"),
		}
		if err := reqid.valid(); err != nil {
			return err
		}
		ruser := &reqUserPutPassword{}
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
		if err = m.RehashPass(ruser.Password); err != nil {
			err.AddTrace(moduleIDUser)
			return err
		}
		if err = m.Update(db); err != nil {
			err.AddTrace(moduleIDUser)
			return err
		}
		return c.JSON(http.StatusCreated, &resUserUpdate{
			Userid:   m.ID.Userid,
			Username: m.Username,
		})
	}, u.gate.Owner("id"))

	rn := r.Group("/name")

	rn.GET("/:username", func(c echo.Context) error {
		ruser := &reqUserGetUsername{
			Username: c.Param("username"),
		}
		if err := ruser.valid(); err != nil {
			return err
		}

		m, err := usermodel.GetByUsername(db, ruser.Username)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, &resUserGetPublic{
			Username:     m.Username,
			Tags:         m.Tags,
			FirstName:    m.FirstName,
			LastName:     m.LastName,
			CreationTime: m.CreationTime,
		})
	})

	if conf.IsDebug() {
		rn.GET("/:username/debug", func(c echo.Context) error {
			ruser := &reqUserGetUsername{
				Username: c.Param("username"),
			}
			if err := ruser.valid(); err != nil {
				return err
			}

			m, err := usermodel.GetByUsername(db, ruser.Username)
			if err != nil {
				return err
			}
			return c.JSON(http.StatusOK, &resUserGet{
				resUserGetPublic: resUserGetPublic{
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
