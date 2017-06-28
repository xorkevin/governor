package user

import (
	"github.com/hackform/governor/service/user/model"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"net/http"
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
