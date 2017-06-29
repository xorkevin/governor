package user

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/model"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"net/http"
	"sort"
)

func (u *User) getByID(c echo.Context, l *logrus.Logger) error {
	db := u.db.DB()

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
}

func (u *User) getByIDPrivate(c echo.Context, l *logrus.Logger) error {
	db := u.db.DB()
	ch := u.cache.Cache()

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

	userid, err := m.IDBase64()
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	sessionIDSetKey := "usersession:" + userid

	var sessions []string
	if s, err := ch.HGetAll(sessionIDSetKey).Result(); err == nil {
		sessions = []string{}
		for k, v := range s {
			sessions = append(sessions, v+","+k)
		}
		sort.Sort(sort.Reverse(sort.StringSlice(sessions)))
	} else {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}

	return c.JSON(http.StatusOK, &resUserGet{
		resUserGetPublic: resUserGetPublic{
			Username:     m.Username,
			Tags:         m.Tags,
			FirstName:    m.FirstName,
			LastName:     m.LastName,
			CreationTime: m.CreationTime,
		},
		Userid:   m.Userid,
		Email:    m.Email,
		Sessions: sessions,
	})
}

func (u *User) getByUsername(c echo.Context, l *logrus.Logger) error {
	db := u.db.DB()

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
}

func (u *User) getByUsernameDebug(c echo.Context, l *logrus.Logger) error {
	db := u.db.DB()

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
}
