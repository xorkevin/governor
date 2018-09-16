package user

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/gate"
	"github.com/labstack/echo"
	"net/http"
	"strconv"
)

type (
	reqUserGetID struct {
		Userid string `json:"-"`
	}

	reqUserGetUsername struct {
		Username string `json:"-"`
	}

	reqGetRoleUserList struct {
		Role   string
		Amount int
		Offset int
	}

	reqGetUserEmails struct {
		Amount int
		Offset int
	}
)

func (r *reqUserGetID) valid() *governor.Error {
	return hasUserid(r.Userid)
}

func (r *reqUserGetUsername) valid() *governor.Error {
	return hasUsername(r.Username)
}

func (r *reqGetRoleUserList) valid() *governor.Error {
	if err := validRole(r.Role); err != nil {
		return err
	}
	if err := validAmount(r.Amount); err != nil {
		return err
	}
	if err := validOffset(r.Offset); err != nil {
		return err
	}
	return nil
}

func (r *reqGetUserEmails) valid() *governor.Error {
	if err := validAmount(r.Amount); err != nil {
		return err
	}
	if err := validOffset(r.Offset); err != nil {
		return err
	}
	return nil
}

func (u *userRouter) getByID(c echo.Context) error {
	ruser := reqUserGetID{
		Userid: c.Param("id"),
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	res, err := u.service.GetByIdPublic(ruser.Userid)
	if err != nil {
		if err.Code() == 2 {
			err.SetErrorUser()
		}
		return err
	}

	return c.JSON(http.StatusOK, res)
}

func (u *userRouter) getByIDPersonal(c echo.Context) error {
	userid := c.Get("userid").(string)

	res, err := u.service.GetById(userid)
	if err != nil {
		if err.Code() == 2 {
			err.SetErrorUser()
		}
		return err
	}

	return c.JSON(http.StatusOK, res)
}

func (u *userRouter) getByIDPrivate(c echo.Context) error {
	ruser := reqUserGetID{
		Userid: c.Param("id"),
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	res, err := u.service.GetById(ruser.Userid)
	if err != nil {
		if err.Code() == 2 {
			err.SetErrorUser()
		}
		return err
	}

	return c.JSON(http.StatusOK, res)
}

func (u *userRouter) getByUsername(c echo.Context) error {
	ruser := reqUserGetUsername{
		Username: c.Param("username"),
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	res, err := u.service.GetByUsernamePublic(ruser.Username)
	if err != nil {
		if err.Code() == 2 {
			err.SetErrorUser()
		}
		return err
	}

	return c.JSON(http.StatusOK, res)
}

func (u *userRouter) getByUsernamePrivate(c echo.Context) error {
	ruser := reqUserGetUsername{
		Username: c.Param("username"),
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	res, err := u.service.GetByUsername(ruser.Username)
	if err != nil {
		if err.Code() == 2 {
			err.SetErrorUser()
		}
		return err
	}

	return c.JSON(http.StatusOK, res)
}

func (u *userRouter) getUsersByRole(c echo.Context) error {
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

	res, err := u.service.GetIDsByRole(ruser.Role, ruser.Amount, ruser.Offset)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	if len(res.Users) == 0 {
		return c.NoContent(http.StatusNotFound)
	}

	return c.JSON(http.StatusOK, res)
}

func (u *userRouter) getAllUserInfo(c echo.Context) error {
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

	res, err := u.service.GetInfoAll(ruser.Amount, ruser.Offset)
	if err != nil {
		err.AddTrace(moduleIDUser)
		return err
	}

	if len(res.Users) == 0 {
		return c.NoContent(http.StatusNotFound)
	}

	return c.JSON(http.StatusOK, res)
}

func (u *userRouter) getByUsernameDebug(c echo.Context) error {
	ruser := reqUserGetUsername{
		Username: c.Param("username"),
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	res, err := u.service.GetByUsername(ruser.Username)
	if err != nil {
		if err.Code() == 2 {
			err.SetErrorUser()
		}
		return err
	}

	return c.JSON(http.StatusOK, res)
}

func (u *userRouter) mountGet(conf governor.Config, r *echo.Group) error {
	r.GET("/id/:id", u.getByID, u.service.cc.Control(true, false, min15, nil))
	r.GET("", u.getByIDPersonal, gate.User(u.service.gate))
	r.GET("/id/:id/private", u.getByIDPrivate, gate.Admin(u.service.gate))
	r.GET("/name/:username", u.getByUsername, u.service.cc.Control(true, false, min15, nil))
	r.GET("/name/:username/private", u.getByUsernamePrivate, gate.Admin(u.service.gate))
	r.GET("/role/:role", u.getUsersByRole)
	r.GET("/all", u.getAllUserInfo, gate.Admin(u.service.gate))
	if conf.IsDebug() {
		r.GET("/name/:username/debug", u.getByUsernameDebug)
	}
	return nil
}
