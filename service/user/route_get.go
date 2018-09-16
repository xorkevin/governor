package user

import (
	"github.com/hackform/governor"
	"github.com/labstack/echo"
	"net/http"
	"strconv"
)

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

func (u *userRouter) getSessions(c echo.Context) error {
	userid := c.Get("userid").(string)
	res, err := u.service.GetSessions(userid)
	if err != nil {
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
