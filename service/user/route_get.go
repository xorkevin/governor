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

func (r *router) getByID(c echo.Context) error {
	ruser := reqUserGetID{
		Userid: c.Param("id"),
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	res, err := r.s.GetByIDPublic(ruser.Userid)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, res)
}

func (r *router) getByIDPersonal(c echo.Context) error {
	userid := c.Get("userid").(string)

	ruser := reqUserGetID{
		Userid: userid,
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	res, err := r.s.GetByID(ruser.Userid)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, res)
}

func (r *router) getByIDPrivate(c echo.Context) error {
	ruser := reqUserGetID{
		Userid: c.Param("id"),
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	res, err := r.s.GetByID(ruser.Userid)
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

func (r *router) getByUsername(c echo.Context) error {
	ruser := reqUserGetUsername{
		Username: c.Param("username"),
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	res, err := r.s.GetByUsernamePublic(ruser.Username)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, res)
}

func (r *router) getByUsernamePrivate(c echo.Context) error {
	ruser := reqUserGetUsername{
		Username: c.Param("username"),
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	res, err := r.s.GetByUsername(ruser.Username)
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

func (r *router) getUsersByRole(c echo.Context) error {
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

	res, err := r.s.GetIDsByRole(ruser.Role, ruser.Amount, ruser.Offset)
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

func (r *router) getAllUserInfo(c echo.Context) error {
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

	res, err := r.s.GetInfoAll(ruser.Amount, ruser.Offset)
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

func (r *router) getUserInfoBulkPublic(c echo.Context) error {
	ruser := reqGetUsers{
		Userids: c.QueryParam("ids"),
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	res, err := r.s.GetInfoBulkPublic(strings.Split(ruser.Userids, ","))
	if err != nil {
		return err
	}

	if len(res.Users) == 0 {
		return c.NoContent(http.StatusNotFound)
	}

	return c.JSON(http.StatusOK, res)
}

func (r *router) getByUsernameDebug(c echo.Context) error {
	ruser := reqUserGetUsername{
		Username: c.Param("username"),
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	res, err := r.s.GetByUsername(ruser.Username)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, res)
}

func (r *router) mountGet(debugMode bool, g *echo.Group) error {
	g.GET("/id/:id", r.getByID)
	g.GET("", r.getByIDPersonal, gate.User(r.s.gate))
	g.GET("/id/:id/private", r.getByIDPrivate, gate.Admin(r.s.gate))
	g.GET("/name/:username", r.getByUsername)
	g.GET("/name/:username/private", r.getByUsernamePrivate, gate.Admin(r.s.gate))
	g.GET("/role/:role", r.getUsersByRole)
	g.GET("/all", r.getAllUserInfo, gate.Admin(r.s.gate))
	g.GET("/ids", r.getUserInfoBulkPublic)
	if debugMode {
		g.GET("/name/:username/debug", r.getByUsernameDebug)
	}
	return nil
}
