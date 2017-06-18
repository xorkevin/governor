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

	reqUserPut struct {
		Username  string `json:"username"`
		Email     string `json:"email"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
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

func (r *reqUserPut) valid() *governor.Error {
	if err := validUsername(r.Username); err != nil {
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
	db := u.db.DB()
	r.POST("", func(c echo.Context) error {
		return postUser(c, l, db)
	})

	ri := r.Group("/id")
	ri.GET("/:id", func(c echo.Context) error {
		return getByID(c, l, db)
	})
	ri.GET("/:id/private", func(c echo.Context) error {
		return getByIDPrivate(c, l, db)
	}, u.gate.OwnerOrAdmin("id"))
	ri.PUT("/:id", func(c echo.Context) error {
		return putUser(c, l, db)
	}, u.gate.Owner("id"))
	ri.PUT("/:id/password", func(c echo.Context) error {
		return putPassword(c, l, db)
	}, u.gate.Owner("id"))
	ri.PATCH("/:id/rank", func(c echo.Context) error {
		return patchRank(c, l, db)
	}, u.gate.User())

	rn := r.Group("/name")
	rn.GET("/:username", func(c echo.Context) error {
		return getByUsername(c, l, db)
	})

	if conf.IsDebug() {
		rn.GET("/:username/debug", func(c echo.Context) error {
			return getByUsernameDebug(c, l, db)
		})
	}

	return nil
}
