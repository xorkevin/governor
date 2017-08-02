package post

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/post/model"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"net/http"
)

type (
	reqGroupGetPosts struct {
		Group  string `json:"-"`
		Amount int    `query:"amount"`
		Offset int    `query:"offset"`
	}

	resGroupPosts struct {
		Posts postmodel.ModelSlice `json:"posts"`
	}
)

func (r *reqGroupGetPosts) valid() *governor.Error {
	if err := validGroup(r.Group); err != nil {
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

func (p *Post) mountGroup(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
	db := p.db.DB()

	r.GET("/:group", func(c echo.Context) error {
		rgroup := &reqGroupGetPosts{}
		if err := c.Bind(rgroup); err != nil {
			return governor.NewErrorUser(moduleIDGroup, err.Error(), 0, http.StatusBadRequest)
		}
		rgroup.Group = c.Param("group")
		if err := rgroup.valid(); err != nil {
			return err
		}

		postsSlice, err := postmodel.GetGroup(db, rgroup.Group, rgroup.Amount, rgroup.Offset)
		if err != nil {
			err.AddTrace(moduleIDGroup)
			return err
		}

		return c.JSON(http.StatusOK, &resGroupPosts{
			Posts: postsSlice,
		})
	})
	return nil
}
