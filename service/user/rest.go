package user

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/gate"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
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

	reqForgotPassword struct {
		Username string `json:"username"`
	}

	reqForgotPasswordReset struct {
		Key         string `json:"key"`
		NewPassword string `json:"new_password"`
	}

	reqUserPut struct {
		Username  string `json:"username"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
	}

	reqUserPutEmail struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	reqUserPutEmailVerify struct {
		Key      string `json:"key"`
		Password string `json:"password"`
	}

	reqUserPutPassword struct {
		NewPassword string `json:"new_password"`
		OldPassword string `json:"old_password"`
	}

	reqUserRmSessions struct {
		SessionIDs []string `json:"session_ids"`
	}

	reqUserPutRank struct {
		Add    string `json:"add"`
		Remove string `json:"remove"`
	}

	reqUserDelete struct {
		Userid   string `json:"userid"`
		Username string `json:"username"`
		Password string `json:"password"`
	}

	resUserUpdate struct {
		Userid   string `json:"userid"`
		Username string `json:"username"`
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

func (r *reqUserPut) valid() *governor.Error {
	if err := validUsername(r.Username); err != nil {
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

func (r *reqUserPutPassword) valid(passlen int) *governor.Error {
	if err := validPassword(r.NewPassword, passlen); err != nil {
		return err
	}
	if err := hasPassword(r.OldPassword); err != nil {
		return err
	}
	return nil
}

func (r *reqUserRmSessions) valid() *governor.Error {
	if err := hasIDs(r.SessionIDs); err != nil {
		return err
	}
	return nil
}

func (r *reqUserPutRank) valid() *governor.Error {
	if err := validRank(r.Add); err != nil {
		return err
	}
	if err := validRank(r.Remove); err != nil {
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

type (
	reqUserGetUsername struct {
		Username string `json:"-"`
	}

	reqUserGetID struct {
		Userid string `json:"-"`
	}

	reqGetRoleUserList struct {
		Role   string
		Amount int
		Offset int
	}

	reqGetUserEmails struct {
		Amount int
		Offset int
	}
)

func (r *reqUserGetUsername) valid() *governor.Error {
	return hasUsername(r.Username)
}

func (r *reqUserGetID) valid() *governor.Error {
	return hasUserid(r.Userid)
}

func (r *reqGetRoleUserList) valid() *governor.Error {
	if err := validRole(r.Role); err != nil {
		return err
	}
	if err := validAmount(r.Amount); err != nil {
		return err
	}
	if err := validOffset(r.Offset); err != nil {
		return err
	}
	return nil
}

func (r *reqGetUserEmails) valid() *governor.Error {
	if err := validAmount(r.Amount); err != nil {
		return err
	}
	if err := validOffset(r.Offset); err != nil {
		return err
	}
	return nil
}

const (
	min15 = 900
	min1  = 60
)

func (u *userRouter) mountRest(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
	r.GET("", u.getByIDPersonal, gate.User(u.service.gate))
	r.GET("/id/:id", u.getByID, u.service.cc.Control(true, false, min15, nil))
	r.GET("/id/:id/private", u.getByIDPrivate, gate.Admin(u.service.gate))
	r.GET("/name/:username", u.getByUsername, u.service.cc.Control(true, false, min15, nil))
	r.GET("/name/:username/private", u.getByUsernamePrivate, gate.Admin(u.service.gate))

	r.GET("/role/:role", u.getUsersByRole)
	r.GET("/all", u.getAllUserInfo, gate.Admin(u.service.gate))

	if conf.IsDebug() {
		r.GET("/name/:username/debug", u.getByUsernameDebug)
	}

	// new user routes
	r.POST("", func(c echo.Context) error {
		return u.confirmUser(c, l)
	})
	r.POST("/confirm", func(c echo.Context) error {
		return u.postUser(c, l)
	})

	// password reset
	r.PUT("/password/forgot", func(c echo.Context) error {
		return u.forgotPassword(c, l)
	})

	r.PUT("/password/forgot/reset", func(c echo.Context) error {
		return u.forgotPasswordReset(c, l)
	})

	r.GET("/sessions", u.getSessions, gate.User(u.service.gate))

	r.PUT("", func(c echo.Context) error {
		return u.putUser(c, l)
	}, gate.User(u.service.gate))

	r.PUT("/email", func(c echo.Context) error {
		return u.putEmail(c, l)
	}, gate.User(u.service.gate))

	r.PUT("/email/verify", func(c echo.Context) error {
		return u.putEmailVerify(c, l)
	})

	r.PUT("/password", func(c echo.Context) error {
		return u.putPassword(c, l)
	}, gate.User(u.service.gate))

	r.DELETE("/sessions", func(c echo.Context) error {
		return u.killSessions(c, l)
	}, gate.User(u.service.gate))

	r.PATCH("/id/:id/rank", func(c echo.Context) error {
		return u.patchRank(c, l)
	}, gate.User(u.service.gate))

	r.DELETE("/id/:id", func(c echo.Context) error {
		return u.deleteUser(c, l)
	}, gate.Owner(u.service.gate, "id"))

	return nil
}
