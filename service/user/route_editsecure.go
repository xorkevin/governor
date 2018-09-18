package user

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/gate"
	"github.com/labstack/echo"
	"net/http"
)

type (
	reqUserPutEmail struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	reqUserPutEmailVerify struct {
		Key      string `json:"key"`
		Password string `json:"password"`
	}
)

func (u *userRouter) putEmail(c echo.Context) error {
	userid := c.Get("userid").(string)

	ruser := reqUserPutEmail{}
	if err := c.Bind(&ruser); err != nil {
		return governor.NewErrorUser(moduleIDUser, err.Error(), 0, http.StatusBadRequest)
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	if err := u.service.UpdateEmail(userid, ruser.Email, ruser.Password); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (r *reqUserPutEmail) valid() *governor.Error {
	if err := validEmail(r.Email); err != nil {
		return err
	}
	if err := hasPassword(r.Password); err != nil {
		return err
	}
	return nil
}

func (r *reqUserPutEmailVerify) valid() *governor.Error {
	if err := hasToken(r.Key); err != nil {
		return err
	}
	if err := hasPassword(r.Password); err != nil {
		return err
	}
	return nil
}

func (u *userRouter) putEmailVerify(c echo.Context) error {
	ruser := reqUserPutEmailVerify{}
	if err := c.Bind(&ruser); err != nil {
		return governor.NewErrorUser(moduleIDUser, err.Error(), 0, http.StatusBadRequest)
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
		NewPassword string `json:"new_password"`
		OldPassword string `json:"old_password"`
	}

	reqForgotPassword struct {
		Username string `json:"username"`
	}

	reqForgotPasswordReset struct {
		Key         string `json:"key"`
		NewPassword string `json:"new_password"`
	}
)

func (r *reqUserPutPassword) valid(passlen int) *governor.Error {
	if err := validPassword(r.NewPassword, passlen); err != nil {
		return err
	}
	if err := hasPassword(r.OldPassword); err != nil {
		return err
	}
	return nil
}

func (r *reqForgotPassword) valid() *governor.Error {
	if err := validUsername(r.Username); err != nil {
		return err
	}
	return nil
}

func (r *reqForgotPassword) validEmail() *governor.Error {
	if err := validEmail(r.Username); err != nil {
		return err
	}
	return nil
}

func (r *reqForgotPasswordReset) valid(passlen int) *governor.Error {
	if err := hasToken(r.Key); err != nil {
		return err
	}
	if err := validPassword(r.NewPassword, passlen); err != nil {
		return err
	}
	return nil
}

func (u *userRouter) putPassword(c echo.Context) error {
	userid := c.Get("userid").(string)

	ruser := reqUserPutPassword{}
	if err := c.Bind(&ruser); err != nil {
		return governor.NewErrorUser(moduleIDUser, err.Error(), 0, http.StatusBadRequest)
	}
	if err := ruser.valid(u.service.passwordMinSize); err != nil {
		return err
	}

	if err := u.service.UpdatePassword(userid, ruser.NewPassword, ruser.OldPassword); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (u *userRouter) forgotPassword(c echo.Context) error {
	ruser := reqForgotPassword{}
	if err := c.Bind(&ruser); err != nil {
		return governor.NewErrorUser(moduleIDUser, err.Error(), 0, http.StatusBadRequest)
	}
	isEmail := false
	if err := ruser.validEmail(); err == nil {
		isEmail = true
	} else if err := ruser.valid(); err != nil {
		return err
	}

	if err := u.service.ForgotPassword(ruser.Username, isEmail); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (u *userRouter) forgotPasswordReset(c echo.Context) error {
	ruser := reqForgotPasswordReset{}
	if err := c.Bind(&ruser); err != nil {
		return governor.NewErrorUser(moduleIDUser, err.Error(), 0, http.StatusBadRequest)
	}
	if err := ruser.valid(u.service.passwordMinSize); err != nil {
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
