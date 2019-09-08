package user

import (
	"github.com/labstack/echo"
	"net/http"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/service/user/token"
	"xorkevin.dev/governor/util/rank"
)

//go:generate forge validation -o validation_edit_gen.go reqUserPut reqUserPutRank

type (
	reqUserPut struct {
		Username  string `valid:"username" json:"username"`
		FirstName string `valid:"firstName" json:"first_name"`
		LastName  string `valid:"lastName" json:"last_name"`
	}
)

func (r *router) putUser(c echo.Context) error {
	userid := c.Get("userid").(string)

	req := reqUserPut{}
	if err := c.Bind(&req); err != nil {
		return err
	}
	if err := req.valid(); err != nil {
		return err
	}

	if err := r.s.UpdateUser(userid, req); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

type (
	reqUserPutRank struct {
		Userid string `valid:"userid,has" json:"-"`
		Add    string `valid:"rank" json:"add"`
		Remove string `valid:"rank" json:"remove"`
	}
)

func (r *router) patchRank(c echo.Context) error {
	req := reqUserPutRank{}
	if err := c.Bind(&req); err != nil {
		return err
	}
	req.Userid = c.Param("id")
	if err := req.valid(); err != nil {
		return err
	}

	updaterClaims := c.Get("user").(*token.Claims)
	updaterRank, _ := rank.FromStringUser(updaterClaims.AuthTags)
	editAddRank, _ := rank.FromStringUser(req.Add)
	editRemoveRank, _ := rank.FromStringUser(req.Remove)

	if err := r.s.UpdateRank(req.Userid, updaterClaims.Userid, updaterRank, editAddRank, editRemoveRank); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (r *router) mountEdit(debugMode bool, g *echo.Group) error {
	g.PUT("", r.putUser, gate.User(r.s.gate))
	g.PATCH("/id/:id/rank", r.patchRank, gate.User(r.s.gate))
	return nil
}
