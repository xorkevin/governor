package user

import (
	"bytes"
	"encoding/gob"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/model"
	"github.com/hackform/governor/service/user/role/model"
	"github.com/hackform/governor/service/user/session"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"net/http"
	"sort"
	"strconv"
)

func (u *userService) getByID(c echo.Context, l *logrus.Logger) error {
	db := u.db.DB()

	ruser := reqUserGetID{
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

	return c.JSON(http.StatusOK, resUserGetPublic{
		Userid:       userid,
		Username:     m.Username,
		Tags:         m.Tags,
		FirstName:    m.FirstName,
		LastName:     m.LastName,
		CreationTime: m.CreationTime,
	})
}

func (u *userService) getByIDPersonal(c echo.Context, l *logrus.Logger) error {
	db := u.db.DB()

	userid := c.Get("userid").(string)

	m, err := usermodel.GetByIDB64(db, userid)
	if err != nil {
		if err.Code() == 2 {
			err.SetErrorUser()
		}
		return err
	}

	return c.JSON(http.StatusOK, resUserGet{
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

func (u *userService) getByIDPrivate(c echo.Context, l *logrus.Logger) error {
	db := u.db.DB()

	ruser := reqUserGetID{
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

	return c.JSON(http.StatusOK, resUserGet{
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

	userid := c.Get("userid").(string)

	s := session.Session{
		Userid: userid,
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

	return c.JSON(http.StatusOK, resUserGetSessions{
		Sessions: sarr,
	})
}

func (u *userService) getByUsername(c echo.Context, l *logrus.Logger) error {
	db := u.db.DB()

	ruser := reqUserGetUsername{
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

	return c.JSON(http.StatusOK, resUserGetPublic{
		Userid:       userid,
		Username:     m.Username,
		Tags:         m.Tags,
		FirstName:    m.FirstName,
		LastName:     m.LastName,
		CreationTime: m.CreationTime,
	})
}

func (u *userService) getByUsernamePrivate(c echo.Context, l *logrus.Logger) error {
	db := u.db.DB()

	ruser := reqUserGetUsername{
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

	return c.JSON(http.StatusOK, resUserGet{
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

func (u *userService) getUsersByRole(c echo.Context, l *logrus.Logger) error {
	db := u.db.DB()

	var amt, ofs int
	if amount, err := strconv.Atoi(c.QueryParam("amount")); err == nil {
		amt = amount
	} else {
		return governor.NewErrorUser(moduleIDReqValid, "amount invalid", 0, http.StatusBadRequest)
	}
	if offset, err := strconv.Atoi(c.QueryParam("offset")); err == nil {
		ofs = offset
	} else {
		return governor.NewErrorUser(moduleIDReqValid, "offset invalid", 0, http.StatusBadRequest)
	}

	ruser := reqGetRoleUserList{
		Role:   c.Param("role"),
		Amount: amt,
		Offset: ofs,
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	userids, err := rolemodel.GetByRole(db, ruser.Role, ruser.Amount, ruser.Offset)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	if len(userids) == 0 {
		return c.NoContent(http.StatusNotFound)
	}

	return c.JSON(http.StatusOK, resUserList{
		Users: userids,
	})
}

func (u *userService) getByUsernameDebug(c echo.Context, l *logrus.Logger) error {
	db := u.db.DB()

	ruser := reqUserGetUsername{
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

	return c.JSON(http.StatusOK, resUserGet{
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

func (u *userService) getAllUserInfo(c echo.Context, l *logrus.Logger) error {
	db := u.db.DB()

	var amt, ofs int
	if amount, err := strconv.Atoi(c.QueryParam("amount")); err == nil {
		amt = amount
	} else {
		return governor.NewErrorUser(moduleIDReqValid, "amount invalid", 0, http.StatusBadRequest)
	}
	if offset, err := strconv.Atoi(c.QueryParam("offset")); err == nil {
		ofs = offset
	} else {
		return governor.NewErrorUser(moduleIDReqValid, "offset invalid", 0, http.StatusBadRequest)
	}

	ruser := reqGetUserEmails{
		Amount: amt,
		Offset: ofs,
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	infoSlice, err := usermodel.GetGroup(db, ruser.Amount, ruser.Offset)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	if len(infoSlice) == 0 {
		return c.NoContent(http.StatusNotFound)
	}

	info := make(userInfoSlice, 0, len(infoSlice))
	for _, i := range infoSlice {
		useruid, _ := i.IDBase64()

		info = append(info, resUserInfo{
			Userid: useruid,
			Email:  i.Email,
		})
	}

	return c.JSON(http.StatusOK, resUserInfoList{
		Users: info,
	})
}
