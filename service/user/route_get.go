package user

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/gate"
	"github.com/labstack/echo"
	"net/http"
	"strconv"
	"strings"
)

const (
	min15 = 900
)

type (
	reqUserGetID struct {
		Userid string `json:"-"`
	}

	reqGetUsers struct {
		Userids string `json:"-"`
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

func (r *reqUserGetID) valid() error {
	return hasUserid(r.Userid)
}

func (r *reqGetUsers) valid() error {
	return hasUserids(r.Userids)
}

func (r *reqUserGetUsername) valid() error {
	return hasUsername(r.Username)
}

func (r *reqGetRoleUserList) valid() error {
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

func (r *reqGetUserEmails) valid() error {
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

	res, err := u.service.GetByIDPublic(ruser.Userid)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, res)
}

func (u *userRouter) getByIDPersonal(c echo.Context) error {
	userid := c.Get("userid").(string)

	res, err := u.service.GetByID(userid)
	if err != nil {
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

	res, err := u.service.GetByID(ruser.Userid)
	if err != nil {
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
		return err
	}

	return c.JSON(http.StatusOK, res)
}

func (u *userRouter) getUsersByRole(c echo.Context) error {
	amount, err := strconv.Atoi(c.QueryParam("amount"))
	if err != nil {
		return governor.NewErrorUser("amount invalid", http.StatusBadRequest, nil)
	}
	offset, err := strconv.Atoi(c.QueryParam("offset"))
	if err != nil {
		return governor.NewErrorUser("offset invalid", http.StatusBadRequest, nil)
	}

	ruser := reqGetRoleUserList{
		Role:   c.Param("role"),
		Amount: amount,
		Offset: offset,
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	res, err := u.service.GetIDsByRole(ruser.Role, ruser.Amount, ruser.Offset)
	if err != nil {
		return err
	}

	if len(res.Users) == 0 {
		return c.NoContent(http.StatusNotFound)
	}

	return c.JSON(http.StatusOK, res)
}

func (u *userRouter) getAllUserInfo(c echo.Context) error {
	amount, err := strconv.Atoi(c.QueryParam("amount"))
	if err != nil {
		return governor.NewErrorUser("amount invalid", http.StatusBadRequest, nil)
	}
	offset, err := strconv.Atoi(c.QueryParam("offset"))
	if err != nil {
		return governor.NewErrorUser("offset invalid", http.StatusBadRequest, nil)
	}

	ruser := reqGetUserEmails{
		Amount: amount,
		Offset: offset,
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	res, err := u.service.GetInfoAll(ruser.Amount, ruser.Offset)
	if err != nil {
		return err
	}

	if len(res.Users) == 0 {
		return c.NoContent(http.StatusNotFound)
	}

	return c.JSON(http.StatusOK, res)
}

func (u *userRouter) getUserInfoBulkPublic(c echo.Context) error {
	ruser := reqGetUsers{
		Userids: c.QueryParam("ids"),
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	res, err := u.service.GetInfoBulkPublic(strings.Split(ruser.Userids, ","))
	if err != nil {
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
	r.GET("/ids", u.getUserInfoBulkPublic)
	if conf.IsDebug() {
		r.GET("/name/:username/debug", u.getByUsernameDebug)
	}
	return nil
}
