package user

import (
	"github.com/labstack/echo"
	"net/http"
	"strconv"
	"strings"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
)

//go:generate forge validation -o validation_get_gen.go reqUserGetID reqUserGetUsername reqGetRoleUser reqGetUserBulk reqGetUsers

type (
	reqUserGetID struct {
		Userid string `valid:"userid,has" json:"-"`
	}
)

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

	ruser := reqUserGetID{
		Userid: userid,
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

type (
	reqUserGetUsername struct {
		Username string `valid:"username,has" json:"-"`
	}
)

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

type (
	reqGetRoleUser struct {
		Role   string `valid:"role,has" json:"-"`
		Amount int    `valid:"amount" json:"-"`
		Offset int    `valid:"offset" json:"-"`
	}
)

func (u *userRouter) getUsersByRole(c echo.Context) error {
	amount, err := strconv.Atoi(c.QueryParam("amount"))
	if err != nil {
		return governor.NewErrorUser("amount invalid", http.StatusBadRequest, nil)
	}
	offset, err := strconv.Atoi(c.QueryParam("offset"))
	if err != nil {
		return governor.NewErrorUser("offset invalid", http.StatusBadRequest, nil)
	}

	ruser := reqGetRoleUser{
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

type (
	reqGetUserBulk struct {
		Amount int `valid:"amount" json:"-"`
		Offset int `valid:"offset" json:"-"`
	}
)

func (u *userRouter) getAllUserInfo(c echo.Context) error {
	amount, err := strconv.Atoi(c.QueryParam("amount"))
	if err != nil {
		return governor.NewErrorUser("amount invalid", http.StatusBadRequest, nil)
	}
	offset, err := strconv.Atoi(c.QueryParam("offset"))
	if err != nil {
		return governor.NewErrorUser("offset invalid", http.StatusBadRequest, nil)
	}

	ruser := reqGetUserBulk{
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

type (
	reqGetUsers struct {
		Userids string `valid:"userids,has" json:"-"`
	}
)

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
	r.GET("/id/:id", u.getByID)
	r.GET("", u.getByIDPersonal, gate.User(u.service.gate))
	r.GET("/id/:id/private", u.getByIDPrivate, gate.Admin(u.service.gate))
	r.GET("/name/:username", u.getByUsername)
	r.GET("/name/:username/private", u.getByUsernamePrivate, gate.Admin(u.service.gate))
	r.GET("/role/:role", u.getUsersByRole)
	r.GET("/all", u.getAllUserInfo, gate.Admin(u.service.gate))
	r.GET("/ids", u.getUserInfoBulkPublic)
	if conf.IsDebug() {
		r.GET("/name/:username/debug", u.getByUsernameDebug)
	}
	return nil
}
