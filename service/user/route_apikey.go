package user

import (
	"github.com/labstack/echo/v4"
	"net/http"
	"strconv"
	"strings"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/util/rank"
)

//go:generate forge validation -o validation_apikey_gen.go reqGetUserApikeys reqApikeyPost reqApikeyID reqApikeyUpdate

type (
	reqGetUserApikeys struct {
		Userid string `valid:"userid,has" json:"-"`
		Amount int    `valid:"amount" json:"-"`
		Offset int    `valid:"offset" json:"-"`
	}
)

func (r *router) getUserApikeys(c echo.Context) error {
	amount, err := strconv.Atoi(c.QueryParam("amount"))
	if err != nil {
		return governor.NewErrorUser("amount invalid", http.StatusBadRequest, nil)
	}
	offset, err := strconv.Atoi(c.QueryParam("offset"))
	if err != nil {
		return governor.NewErrorUser("amount invalid", http.StatusBadRequest, nil)
	}
	req := reqGetUserApikeys{
		Userid: c.Get("userid").(string),
		Amount: amount,
		Offset: offset,
	}
	if err := req.valid(); err != nil {
		return err
	}
	res, err := r.s.GetUserApikeys(req.Userid, req.Amount, req.Offset)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, res)
}

type (
	reqApikeyPost struct {
		Userid   string `valid:"userid,has" json:"-"`
		AuthTags string `valid:"rank" json:"auth_tags"`
		Name     string `valid:"apikeyName" json:"name"`
		Desc     string `valid:"apikeyDesc" json:"desc"`
	}
)

func (r *router) createApikey(c echo.Context) error {
	req := reqApikeyPost{}
	if err := c.Bind(&req); err != nil {
		return err
	}
	req.Userid = c.Get("userid").(string)
	if err := req.valid(); err != nil {
		return err
	}
	authTags, _ := rank.FromStringUser(req.AuthTags)
	res, err := r.s.CreateApikey(req.Userid, authTags, req.Name, req.Desc)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, res)
}

type (
	reqApikeyID struct {
		Userid string `valid:"userid,has" json:"-"`
		Keyid  string `valid:"apikeyid,has" json:"-"`
	}
)

func (r *reqApikeyID) validUserid() error {
	k := strings.SplitN(r.Keyid, "|", 2)
	if r.Userid != k[0] {
		return governor.NewErrorUser("Invalid apikey id", http.StatusForbidden, nil)
	}
	return nil
}

func (r *router) deleteApikey(c echo.Context) error {
	req := reqApikeyID{
		Userid: c.Get("userid").(string),
		Keyid:  c.Param("id"),
	}
	if err := req.valid(); err != nil {
		return err
	}
	if err := req.validUserid(); err != nil {
		return err
	}
	if err := r.s.DeleteApikey(req.Keyid); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

type (
	reqApikeyUpdate struct {
		Userid   string `valid:"userid,has" json:"-"`
		Keyid    string `valid:"apikeyid,has" json:"-"`
		AuthTags string `valid:"rank" json:"auth_tags"`
		Name     string `valid:"apikeyName" json:"name"`
		Desc     string `valid:"apikeyDesc" json:"desc"`
	}
)

func (r *reqApikeyUpdate) validUserid() error {
	k := strings.SplitN(r.Keyid, "|", 2)
	if r.Userid != k[0] {
		return governor.NewErrorUser("Invalid apikey id", http.StatusForbidden, nil)
	}
	return nil
}

func (r *router) updateApikey(c echo.Context) error {
	req := reqApikeyUpdate{}
	if err := c.Bind(&req); err != nil {
		return err
	}
	req.Userid = c.Get("userid").(string)
	req.Keyid = c.Param("id")
	if err := req.valid(); err != nil {
		return err
	}
	if err := req.validUserid(); err != nil {
		return err
	}
	authTags, _ := rank.FromStringUser(req.AuthTags)
	if err := r.s.UpdateApikey(req.Userid, req.Keyid, authTags, req.Name, req.Desc); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (r *router) rotateApikey(c echo.Context) error {
	req := reqApikeyID{
		Userid: c.Get("userid").(string),
		Keyid:  c.Param("id"),
	}
	if err := req.valid(); err != nil {
		return err
	}
	if err := req.validUserid(); err != nil {
		return err
	}
	res, err := r.s.RotateApikey(req.Keyid)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, res)
}

func (r *router) mountApikey(g *echo.Group) {
	g.GET("", r.getUserApikeys, gate.User(r.s.gate))
	g.POST("", r.createApikey, gate.User(r.s.gate))
	g.PUT("/id/:id", r.updateApikey, gate.User(r.s.gate))
	g.PUT("/id/:id/rotate", r.updateApikey, gate.User(r.s.gate))
	g.DELETE("/id/:id", r.deleteApikey, gate.User(r.s.gate))
}
