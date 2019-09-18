package user

import (
	"github.com/labstack/echo/v4"
	"net/http"
	"strings"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
)

//go:generate forge validation -o validation_session_gen.go reqUserRmSessions

func (r *router) getSessions(c echo.Context) error {
	req := reqUserGetID{
		Userid: c.Get("userid").(string),
	}
	if err := req.valid(); err != nil {
		return err
	}
	res, err := r.s.GetUserSessions(req.Userid)
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
