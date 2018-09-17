package user

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/gate"
	"github.com/labstack/echo"
	"net/http"
)

type (
	reqUserPost struct {
		Username  string `json:"username"`
		Password  string `json:"password"`
		Email     string `json:"email"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
	}

	reqUserPostConfirm struct {
		Key string `json:"key"`
	}

	reqUserDelete struct {
		Userid   string `json:"userid"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
)

func (r *reqUserPost) valid(passlen int) *governor.Error {
	if err := validUsername(r.Username); err != nil {
		return err
	}
	if err := validPassword(r.Password, passlen); err != nil {
		return err
	}
	if err := validEmail(r.Email); err != nil {
		return err
	}
	if err := validFirstName(r.FirstName); err != nil {
		return err
	}
	if err := validLastName(r.LastName); err != nil {
		return err
	}
	return nil
}

func (r *reqUserPostConfirm) valid() *governor.Error {
	if err := hasToken(r.Key); err != nil {
		return err
	}
	return nil
}

func (r *reqUserDelete) valid() *governor.Error {
	if err := hasUserid(r.Userid); err != nil {
		return err
	}
	if err := hasUsername(r.Username); err != nil {
		return err
	}
	if err := hasPassword(r.Password); err != nil {
		return err
	}
	return nil
}

func (u *userRouter) createUser(c echo.Context) error {
	ruser := reqUserPost{}
	if err := c.Bind(&ruser); err != nil {
		return governor.NewErrorUser(moduleIDUser, err.Error(), 0, http.StatusBadRequest)
	}
	if err := ruser.valid(u.service.passwordMinSize); err != nil {
		return err
	}

	res, err := u.service.CreateUser(ruser)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, res)
}

func (u *userRouter) commitUser(c echo.Context) error {
	ruser := reqUserPostConfirm{}
	if err := c.Bind(&ruser); err != nil {
		return governor.NewErrorUser(moduleIDUser, err.Error(), 0, http.StatusBadRequest)
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

func (u *userRouter) deleteUser(c echo.Context) error {
	reqid := &reqUserGetID{
		Userid: c.Param("id"),
	}
	if err := reqid.valid(); err != nil {
		return err
	}
	ruser := reqUserDelete{}
	if err := c.Bind(&ruser); err != nil {
		return governor.NewErrorUser(moduleIDUser, err.Error(), 0, http.StatusBadRequest)
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	if reqid.Userid != ruser.Userid {
		return governor.NewErrorUser(moduleIDUser, "information does not match", 0, http.StatusBadRequest)
	}

	if err := u.service.DeleteUser(reqid.Userid, ruser.Username, ruser.Password); err != nil {
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
