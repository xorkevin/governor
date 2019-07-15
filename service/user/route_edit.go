package user

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/gate"
	"github.com/hackform/governor/service/user/token"
	"github.com/hackform/governor/util/rank"
	"github.com/labstack/echo"
	"net/http"
)

//go:generate forge validation -o validation_edit_gen.go reqUserPut reqUserPutRank

type (
	reqUserPut struct {
		Username  string `valid:"username" json:"username"`
		FirstName string `valid:"firstName" json:"first_name"`
		LastName  string `valid:"lastName" json:"last_name"`
	}
)

func (u *userRouter) putUser(c echo.Context) error {
	userid := c.Get("userid").(string)

	ruser := reqUserPut{}
	if err := c.Bind(&ruser); err != nil {
		return err
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	if err := u.service.UpdateUser(userid, ruser); err != nil {
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

func (u *userRouter) patchRank(c echo.Context) error {
	ruser := reqUserPutRank{}
	if err := c.Bind(&ruser); err != nil {
		return err
	}
	ruser.Userid = c.Param("id")
	if err := ruser.valid(); err != nil {
		return err
	}

	updaterClaims := c.Get("user").(*token.Claims)
	updaterRank, _ := rank.FromStringUser(updaterClaims.AuthTags)
	editAddRank, _ := rank.FromStringUser(ruser.Add)
	editRemoveRank, _ := rank.FromStringUser(ruser.Remove)

	if err := u.service.UpdateRank(ruser.Userid, updaterClaims.Userid, updaterRank, editAddRank, editRemoveRank); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (u *userRouter) mountEdit(conf governor.Config, r *echo.Group) error {
	r.PUT("", u.putUser, gate.User(u.service.gate))
	r.PATCH("/id/:id/rank", u.patchRank, gate.User(u.service.gate))
	return nil
}
