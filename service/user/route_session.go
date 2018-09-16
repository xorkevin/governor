package user

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/gate"
	"github.com/hackform/governor/service/user/session"
	"github.com/labstack/echo"
	"net/http"
)

type (
	resUserGetSessions struct {
		Sessions []session.Session `json:"active_sessions"`
	}
)

func (u *userRouter) getSessions(c echo.Context) error {
	userid := c.Get("userid").(string)
	res, err := u.service.GetSessions(userid)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, res)
}

type (
	reqUserRmSessions struct {
		SessionIDs []string `json:"session_ids"`
	}
)

func (r *reqUserRmSessions) valid() *governor.Error {
	if err := hasIDs(r.SessionIDs); err != nil {
		return err
	}
	return nil
}

func (u *userRouter) killSessions(c echo.Context) error {
	userid := c.Get("userid").(string)

	ruser := reqUserRmSessions{}
	if err := c.Bind(&ruser); err != nil {
		return governor.NewErrorUser(moduleIDUser, err.Error(), 0, http.StatusBadRequest)
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	if err := u.service.KillSessions(userid, ruser.SessionIDs); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (u *userRouter) mountSession(conf governor.Config, r *echo.Group) error {
	r.GET("/sessions", u.getSessions, gate.User(u.service.gate))
	r.DELETE("/sessions", u.killSessions, gate.User(u.service.gate))
	return nil
}
