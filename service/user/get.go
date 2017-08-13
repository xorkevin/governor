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

func (u *userService) getByID(c echo.Context, l *logrus.Logger) error {
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

	userid, _ := m.IDBase64()

	return c.JSON(http.StatusOK, &resUserGetPublic{
		Userid:       userid,
		Username:     m.Username,
		Tags:         m.Tags,
		FirstName:    m.FirstName,
		LastName:     m.LastName,
		CreationTime: m.CreationTime,
	})
}

func (u *userService) getByIDPrivate(c echo.Context, l *logrus.Logger) error {
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

	userid, _ := m.IDBase64()

	return c.JSON(http.StatusOK, &resUserGet{
		resUserGetPublic: resUserGetPublic{
			Userid:       userid,
			Username:     m.Username,
			Tags:         m.Tags,
			FirstName:    m.FirstName,
			LastName:     m.LastName,
			CreationTime: m.CreationTime,
		},
		Email: m.Email,
	})
}

func (u *userService) getSessions(c echo.Context, l *logrus.Logger) error {
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

	var sarr session.Slice
	if sgobs, err := ch.HGetAll(s.UserKey()).Result(); err == nil {
		sarr = make(session.Slice, 0, len(sgobs))
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

func (u *userService) getByUsername(c echo.Context, l *logrus.Logger) error {
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

	userid, _ := m.IDBase64()

	return c.JSON(http.StatusOK, &resUserGetPublic{
		Userid:       userid,
		Username:     m.Username,
		Tags:         m.Tags,
		FirstName:    m.FirstName,
		LastName:     m.LastName,
		CreationTime: m.CreationTime,
	})
}

func (u *userService) getByUsernameDebug(c echo.Context, l *logrus.Logger) error {
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

	userid, _ := m.IDBase64()

	return c.JSON(http.StatusOK, &resUserGet{
		resUserGetPublic: resUserGetPublic{
			Userid:       userid,
			Username:     m.Username,
			Tags:         m.Tags,
			FirstName:    m.FirstName,
			LastName:     m.LastName,
			CreationTime: m.CreationTime,
		},
		Email: m.Email,
	})
}
