package user

import (
	"bytes"
	"encoding/gob"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/model"
	"github.com/hackform/governor/service/user/session"
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
		if err.Code() == 2 {
			err.SetErrorUser()
		}
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

	ruser := &reqUserGetID{
		Userid: c.Param("id"),
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	m, err := usermodel.GetByIDB64(db, ruser.Userid)
	if err != nil {
		if err.Code() == 2 {
			err.SetErrorUser()
		}
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

func (u *User) getSessions(c echo.Context, l *logrus.Logger) error {
	ch := u.cache.Cache()

	ruser := &reqUserGetID{
		Userid: c.Param("id"),
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	s := session.Session{
		Userid: ruser.Userid,
	}

	sarr := session.Slice{}
	if sgobs, err := ch.HGetAll(s.UserKey()).Result(); err == nil {
		for _, v := range sgobs {
			s := session.Session{}
			if err = gob.NewDecoder(bytes.NewBufferString(v)).Decode(&s); err != nil {
				return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
			}
			sarr = append(sarr, s)
		}
	} else {
		return governor.NewError(moduleIDUser, err.Error(), 0, http.StatusInternalServerError)
	}
	sort.Sort(sort.Reverse(sarr))

	return c.JSON(http.StatusOK, &resUserGetSessions{
		Sessions: sarr,
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
		if err.Code() == 2 {
			err.SetErrorUser()
		}
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
		if err.Code() == 2 {
			err.SetErrorUser()
		}
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
