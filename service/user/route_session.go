package user

import (
	"github.com/labstack/echo/v4"
	"net/http"
	"strconv"
	"strings"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
)

//go:generate forge validation -o validation_session_gen.go reqGetUserSessions reqUserRmSessions

type (
	reqGetUserSessions struct {
		Userid string `valid:"userid,has" json:"-"`
		Amount int    `valid:"amount" json:"-"`
		Offset int    `valid:"offset" json:"-"`
	}
)

func (r *router) getSessions(c echo.Context) error {
	amount, err := strconv.Atoi(c.QueryParam("amount"))
	if err != nil {
		return governor.NewErrorUser("amount invalid", http.StatusBadRequest, nil)
	}
	offset, err := strconv.Atoi(c.QueryParam("offset"))
	if err != nil {
		return governor.NewErrorUser("amount invalid", http.StatusBadRequest, nil)
	}
	req := reqGetUserSessions{
		Userid: c.Get("userid").(string),
		Amount: amount,
		Offset: offset,
	}
	if err := req.valid(); err != nil {
		return err
	}
	res, err := r.s.GetUserSessions(req.Userid, req.Amount, req.Offset)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, res)
}

type (
	reqUserRmSessions struct {
		Userid     string   `valid:"userid,has" json:"-"`
		SessionIDs []string `valid:"sessionIDs" json:"session_ids"`
	}
)

func (r *reqUserRmSessions) validUserid() error {
	for _, i := range r.SessionIDs {
		j := strings.SplitN(i, "|", 2)
		if r.Userid != j[0] {
			return governor.NewErrorUser("Invalid session ids", http.StatusForbidden, nil)
		}
	}
	return nil
}

func (r *router) killSessions(c echo.Context) error {
	req := reqUserRmSessions{}
	if err := c.Bind(&req); err != nil {
		return err
	}
	req.Userid = c.Get("userid").(string)
	if err := req.valid(); err != nil {
		return err
	}
	if err := req.validUserid(); err != nil {
		return err
	}

	if err := r.s.KillSessions(req.SessionIDs); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (r *router) mountSession(debugMode bool, g *echo.Group) error {
	g.GET("/sessions", r.getSessions, gate.User(r.s.gate))
	g.DELETE("/sessions", r.killSessions, gate.User(r.s.gate))
	return nil
}
