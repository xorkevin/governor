package user

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/gate"
	"github.com/hackform/governor/service/user/session"
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

func (r *reqUserPost) valid() *governor.Error {
	if err := validUsername(r.Username); err != nil {
		return err
	}
	if err := validPassword(r.Password); err != nil {
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

func (r *reqForgotPasswordReset) valid() *governor.Error {
	if err := hasToken(r.Key); err != nil {
		return err
	}
	if err := validPassword(r.NewPassword); err != nil {
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

func (r *reqUserPutPassword) valid() *governor.Error {
	if err := validPassword(r.NewPassword); err != nil {
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

	resUserGetPublic struct {
		Userid       string `json:"userid"`
		Username     string `json:"username"`
		Tags         string `json:"auth_tags"`
		FirstName    string `json:"first_name"`
		LastName     string `json:"last_name"`
		CreationTime int64  `json:"creation_time"`
	}

	resUserGet struct {
		resUserGetPublic
		Email string `json:"email"`
	}

	resUserGetSessions struct {
		Sessions []session.Session `json:"active_sessions"`
	}
)

func (r *reqUserGetUsername) valid() *governor.Error {
	return hasUsername(r.Username)
}

func (r *reqUserGetID) valid() *governor.Error {
	return hasUserid(r.Userid)
}

const (
	min2 = 120
)

func (u *userService) mountRest(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
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

	// id routes
	ri := r.Group("/id")

	ri.GET("/:id", func(c echo.Context) error {
		return u.getByID(c, l)
	}, u.cc.Control(true, false, min2, func(c echo.Context) (string, *governor.Error) {
		return "", nil
	}))

	ri.GET("/:id/private", func(c echo.Context) error {
		return u.getByIDPrivate(c, l)
	}, gate.OwnerOrAdmin(u.gate, "id"),
		u.cc.Control(false, false, min2, func(c echo.Context) (string, *governor.Error) {
			return "", nil
		}))

	ri.GET("/sessions", func(c echo.Context) error {
		return u.getSessions(c, l)
	}, gate.User(u.gate))

	ri.PUT("/:id", func(c echo.Context) error {
		return u.putUser(c, l)
	}, gate.Owner(u.gate, "id"))

	ri.PUT("/:id/email", func(c echo.Context) error {
		return u.putEmail(c, l)
	}, gate.Owner(u.gate, "id"))

	ri.PUT("/:id/password", func(c echo.Context) error {
		return u.putPassword(c, l)
	}, gate.Owner(u.gate, "id"))

	ri.DELETE("/sessions", func(c echo.Context) error {
		return u.killSessions(c, l)
	}, gate.User(u.gate))

	ri.PATCH("/:id/rank", func(c echo.Context) error {
		return u.patchRank(c, l)
	}, gate.User(u.gate))

	ri.DELETE("/:id", func(c echo.Context) error {
		return u.deleteUser(c, l)
	}, gate.Owner(u.gate, "id"))

	// username routes
	rn := r.Group("/name")

	rn.GET("/:username", func(c echo.Context) error {
		return u.getByUsername(c, l)
	}, u.cc.Control(true, false, min2, func(c echo.Context) (string, *governor.Error) {
		return "", nil
	}))

	if conf.IsDebug() {
		rn.GET("/:username/debug", func(c echo.Context) error {
			return u.getByUsernameDebug(c, l)
		})
	}

	return nil
}
