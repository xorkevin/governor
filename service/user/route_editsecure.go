package user

import (
	"github.com/labstack/echo"
	"net/http"
	"xorkevin.dev/governor/service/user/gate"
)

//go:generate forge validation -o validation_editsecure_gen.go reqUserPutEmail reqUserPutEmailVerify reqUserPutPassword reqForgotPasswordReset

type (
	reqUserPutEmail struct {
		Userid   string `valid:"userid,has" json:"-"`
		Email    string `valid:"email" json:"email"`
		Password string `valid:"password,has" json:"password"`
	}
)

func (r *router) putEmail(c echo.Context) error {
	req := reqUserPutEmail{}
	if err := c.Bind(&req); err != nil {
		return err
	}
	req.Userid = c.Get("userid").(string)
	if err := req.valid(); err != nil {
		return err
	}

	if err := r.s.UpdateEmail(req.Userid, req.Email, req.Password); err != nil {
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

func (r *router) putEmailVerify(c echo.Context) error {
	req := reqUserPutEmailVerify{}
	if err := c.Bind(&req); err != nil {
		return err
	}
	if err := req.valid(); err != nil {
		return err
	}

	if err := r.s.CommitEmail(req.Key, req.Password); err != nil {
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

func (r *router) putPassword(c echo.Context) error {

	req := reqUserPutPassword{}
	if err := c.Bind(&req); err != nil {
		return err
	}
	req.Userid = c.Get("userid").(string)
	if err := req.valid(); err != nil {
		return err
	}

	if err := r.s.UpdatePassword(req.Userid, req.NewPassword, req.OldPassword); err != nil {
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

func (r *router) forgotPassword(c echo.Context) error {
	req := reqForgotPassword{}
	if err := c.Bind(&req); err != nil {
		return err
	}
	isEmail, err := req.valid()
	if err != nil {
		return err
	}

	if err := r.s.ForgotPassword(req.Username, isEmail); err != nil {
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

func (r *router) forgotPasswordReset(c echo.Context) error {
	req := reqForgotPasswordReset{}
	if err := c.Bind(&req); err != nil {
		return err
	}
	if err := req.valid(); err != nil {
		return err
	}

	if err := r.s.ResetPassword(req.Key, req.NewPassword); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (r *router) mountEditSecure(debugMode bool, g *echo.Group) error {
	g.PUT("/email", r.putEmail, gate.User(r.s.gate))
	g.PUT("/email/verify", r.putEmailVerify)
	g.PUT("/password", r.putPassword, gate.User(r.s.gate))
	g.PUT("/password/forgot", r.forgotPassword)
	g.PUT("/password/forgot/reset", r.forgotPasswordReset)
	return nil
}
