package user

import (
	"github.com/labstack/echo"
	"net/http"
	"strings"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
)

//go:generate forge validation -o validation_session_gen.go reqUserRmSessions

func (u *userRouter) getSessions(c echo.Context) error {
	ruser := reqUserGetID{
		Userid: c.Get("userid").(string),
	}
	if err := ruser.valid(); err != nil {
		return err
	}
	res, err := u.service.GetUserSessions(ruser.Userid)
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
	j := strings.SplitN(r.SessionIDs[0], "|", 2)
	if r.Userid != j[0] {
		return governor.NewErrorUser("Invalid session ids", http.StatusForbidden, nil)
	}
	return nil
}

func (u *userRouter) killSessions(c echo.Context) error {
	ruser := reqUserRmSessions{}
	if err := c.Bind(&ruser); err != nil {
		return err
	}
	ruser.Userid = c.Get("userid").(string)
	if err := ruser.valid(); err != nil {
		return err
	}
	if err := ruser.validUserid(); err != nil {
		return err
	}

	if err := u.service.KillSessions(ruser.SessionIDs); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (u *userRouter) mountSession(conf governor.Config, r *echo.Group) error {
	r.GET("/sessions", u.getSessions, gate.User(u.service.gate))
	r.DELETE("/sessions", u.killSessions, gate.User(u.service.gate))
	return nil
}
