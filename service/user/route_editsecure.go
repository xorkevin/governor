package user

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/gate"
	"github.com/labstack/echo"
	"net/http"
)

//go:generate forge validation -o validation_editsecure_gen.go reqUserPutEmail reqUserPutEmailVerify reqUserPutPassword reqForgotPasswordReset

type (
	reqUserPutEmail struct {
		Userid   string `valid:"userid,has" json:"-"`
		Email    string `valid:"email" json:"email"`
		Password string `valid:"password,has" json:"password"`
	}
)

func (u *userRouter) putEmail(c echo.Context) error {
	ruser := reqUserPutEmail{}
	if err := c.Bind(&ruser); err != nil {
		return err
	}
	ruser.Userid = c.Get("userid").(string)
	if err := ruser.valid(); err != nil {
		return err
	}

	if err := u.service.UpdateEmail(ruser.Userid, ruser.Email, ruser.Password); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

type (
	reqUserPutEmailVerify struct {
		Key      string `valid:"token,has" json:"key"`
		Password string `valid:"password,has" json:"password"`
	}
)

func (u *userRouter) putEmailVerify(c echo.Context) error {
	ruser := reqUserPutEmailVerify{}
	if err := c.Bind(&ruser); err != nil {
		return err
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	if err := u.service.CommitEmail(ruser.Key, ruser.Password); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

type (
	reqUserPutPassword struct {
		Userid      string `valid:"userid,has" json:"-"`
		NewPassword string `valid:"password" json:"new_password"`
		OldPassword string `valid:"password,has" json:"old_password"`
	}
)

func (u *userRouter) putPassword(c echo.Context) error {

	ruser := reqUserPutPassword{}
	if err := c.Bind(&ruser); err != nil {
		return err
	}
	ruser.Userid = c.Get("userid").(string)
	if err := ruser.valid(); err != nil {
		return err
	}

	if err := u.service.UpdatePassword(ruser.Userid, ruser.NewPassword, ruser.OldPassword); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

type (
	reqForgotPassword struct {
		Username string `json:"username"`
	}
)

func (r *reqForgotPassword) valid() (bool, error) {
	return validhasUsernameOrEmail(r.Username)
}

func (u *userRouter) forgotPassword(c echo.Context) error {
	ruser := reqForgotPassword{}
	if err := c.Bind(&ruser); err != nil {
		return err
	}
	isEmail, err := ruser.valid()
	if err != nil {
		return err
	}

	if err := u.service.ForgotPassword(ruser.Username, isEmail); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

type (
	reqForgotPasswordReset struct {
		Key         string `valid:"token,has" json:"key"`
		NewPassword string `valid:"password" json:"new_password"`
	}
)

func (u *userRouter) forgotPasswordReset(c echo.Context) error {
	ruser := reqForgotPasswordReset{}
	if err := c.Bind(&ruser); err != nil {
		return err
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	if err := u.service.ResetPassword(ruser.Key, ruser.NewPassword); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (u *userRouter) mountEditSecure(conf governor.Config, r *echo.Group) error {
	r.PUT("/email", u.putEmail, gate.User(u.service.gate))
	r.PUT("/email/verify", u.putEmailVerify)
	r.PUT("/password", u.putPassword, gate.User(u.service.gate))
	r.PUT("/password/forgot", u.forgotPassword)
	r.PUT("/password/forgot/reset", u.forgotPasswordReset)
	return nil
}
