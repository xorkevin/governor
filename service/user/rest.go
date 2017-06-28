package user

import (
	"github.com/hackform/governor"
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

	reqUserPutRank struct {
		Add    string `json:"add"`
		Remove string `json:"remove"`
	}

	resUserUpdate struct {
		Userid   []byte `json:"userid"`
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

func (r *reqUserPutRank) valid() *governor.Error {
	if err := validRank(r.Add); err != nil {
		return err
	}
	if err := validRank(r.Remove); err != nil {
		return err
	}
	return nil
}

type (
	reqUserGetUsername struct {
		Username string `json:"username"`
	}

	reqUserGetID struct {
		Userid string `json:"userid"`
	}

	resUserGetPublic struct {
		Username     string `json:"username"`
		Tags         string `json:"auth_tags"`
		FirstName    string `json:"first_name"`
		LastName     string `json:"last_name"`
		CreationTime int64  `json:"creation_time"`
	}

	resUserGet struct {
		resUserGetPublic
		Userid []byte `json:"userid"`
		Email  string `json:"email"`
	}
)

func (r *reqUserGetUsername) valid() *governor.Error {
	return hasUsername(r.Username)
}

func (r *reqUserGetID) valid() *governor.Error {
	return hasUserid(r.Userid)
}

func (u *User) mountRest(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
	// new user routes

	r.POST("", func(c echo.Context) error {
		return u.confirmUser(c, l)
	})
	r.POST("/confirm", func(c echo.Context) error {
		return u.postUser(c, l)
	})

	// id routes
	ri := r.Group("/id")

	ri.GET("/:id", func(c echo.Context) error {
		return u.getByID(c, l)
	})

	ri.GET("/:id/private", func(c echo.Context) error {
		return u.getByIDPrivate(c, l)
	}, u.gate.OwnerOrAdmin("id"))

	ri.PUT("/:id", func(c echo.Context) error {
		return u.putUser(c, l)
	}, u.gate.Owner("id"))

	ri.PUT("/:id/email", func(c echo.Context) error {
		return u.putEmail(c, l)
	}, u.gate.Owner("id"))

	ri.PUT("/:id/password", func(c echo.Context) error {
		return u.putPassword(c, l)
	}, u.gate.Owner("id"))

	ri.PATCH("/:id/rank", func(c echo.Context) error {
		return u.patchRank(c, l)
	}, u.gate.User())

	// username routes
	rn := r.Group("/name")

	rn.GET("/:username", func(c echo.Context) error {
		return u.getByUsername(c, l)
	})

	if conf.IsDebug() {
		rn.GET("/:username/debug", func(c echo.Context) error {
			return u.getByUsernameDebug(c, l)
		})
	}

	return nil
}
