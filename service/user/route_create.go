package user

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/gate"
	"github.com/labstack/echo"
	"net/http"
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

func (u *userRouter) createUser(c echo.Context) error {
	ruser := reqUserPost{}
	if err := c.Bind(&ruser); err != nil {
		return err
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	res, err := u.service.CreateUser(ruser)
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

func (u *userRouter) commitUser(c echo.Context) error {
	ruser := reqUserPostConfirm{}
	if err := c.Bind(&ruser); err != nil {
		return err
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	res, err := u.service.CommitUser(ruser.Key)
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

func (u *userRouter) deleteUser(c echo.Context) error {
	ruser := reqUserDelete{}
	if err := c.Bind(&ruser); err != nil {
		return err
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	if c.Param("id") != ruser.Userid {
		return governor.NewErrorUser("information does not match", http.StatusBadRequest, nil)
	}

	if err := u.service.DeleteUser(ruser.Userid, ruser.Username, ruser.Password); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (u *userRouter) mountCreate(conf governor.Config, r *echo.Group) error {
	r.POST("", u.createUser)
	r.POST("/confirm", u.commitUser)
	r.DELETE("/id/:id", u.deleteUser, gate.Owner(u.service.gate, "id"))
	return nil
}
