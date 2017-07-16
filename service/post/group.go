package post

import (
	"github.com/hackform/governor"
	// "github.com/hackform/governor/service/post/model"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"net/http"
)

type (
	reqGroupGetPosts struct {
		Group string `json:"group_tag"`
	}

	resGroupPost struct {
		Postid       []byte `json:"postid"`
		Userid       []byte `json:"userid"`
		Tag          string `json:"group_tag"`
		Title        string `json:"title"`
		CreationTime int64  `json:"creation_time"`
	}

	postsSlice []resGroupPost

	resGroupPosts struct {
		Posts postsSlice `json:"posts"`
	}
)

func (r *reqGroupGetPosts) valid() *governor.Error {
	if err := validGroup(r.Group); err != nil {
		return err
	}
	return nil
}

func (p *Post) mountGroup(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
	// db := p.db.DB()

	r.GET("/:id", func(c echo.Context) error {
		rgroup := &reqGroupGetPosts{
			Group: c.Param("id"),
		}
		if err := rgroup.valid(); err != nil {
			return err
		}

		return c.JSON(http.StatusOK, &resGroupPosts{
			Posts: postsSlice{},
		})
	})
	return nil
}