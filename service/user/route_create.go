package user

import (
	"github.com/labstack/echo"
	"net/http"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
)

//go:generate forge validation -o validation_create_gen.go reqUserPost reqUserPostConfirm reqUserDelete

type (
	reqUserPost struct {
		Username  string `valid:"username" json:"username"`
		Password  string `valid:"password" json:"password"`
		Email     string `valid:"email" json:"email"`
		FirstName string `valid:"firstName" json:"first_name"`
		LastName  string `valid:"lastName" json:"last_name"`
	}
)

func (r *router) createUser(c echo.Context) error {
	req := reqUserPost{}
	if err := c.Bind(&req); err != nil {
		return err
	}
	if err := req.valid(); err != nil {
		return err
	}

	res, err := r.s.CreateUser(req)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, res)
}

type (
	reqUserPostConfirm struct {
		Key string `valid:"token,has" json:"key"`
	}
)

func (r *router) commitUser(c echo.Context) error {
	req := reqUserPostConfirm{}
	if err := c.Bind(&req); err != nil {
		return err
	}
	if err := req.valid(); err != nil {
		return err
	}

	res, err := r.s.CommitUser(req.Key)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, res)
}

type (
	reqUserDelete struct {
		Userid   string `valid:"userid,has" json:"userid"`
		Username string `valid:"username,has" json:"username"`
		Password string `valid:"password,has" json:"password"`
	}
)

func (r *router) deleteUser(c echo.Context) error {
	req := reqUserDelete{}
	if err := c.Bind(&req); err != nil {
		return err
	}
	if err := req.valid(); err != nil {
		return err
	}

	if c.Param("id") != req.Userid {
		return governor.NewErrorUser("information does not match", http.StatusBadRequest, nil)
	}

	if err := r.s.DeleteUser(req.Userid, req.Username, req.Password); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (r *router) mountCreate(debugMode bool, g *echo.Group) error {
	g.POST("", r.createUser)
	g.POST("/confirm", r.commitUser)
	g.DELETE("/id/:id", r.deleteUser, gate.Owner(r.s.gate, "id"))
	return nil
}
