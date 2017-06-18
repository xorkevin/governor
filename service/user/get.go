package user

import (
	"database/sql"
	"github.com/hackform/governor/service/user/model"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"net/http"
)

func getByID(c echo.Context, l *logrus.Logger, db *sql.DB) error {
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

func getByIDPrivate(c echo.Context, l *logrus.Logger, db *sql.DB) error {
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

func getByUsername(c echo.Context, l *logrus.Logger, db *sql.DB) error {
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

func getByUsernameDebug(c echo.Context, l *logrus.Logger, db *sql.DB) error {
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
