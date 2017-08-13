package post

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/post/model"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"net/http"
	"strconv"
)

type (
	reqGroupGetPosts struct {
		Group  string `json:"-"`
		Amount int
		Offset int
	}

	resPostInfo struct {
		Postid       string `json:"postid"`
		Userid       string `json:"userid"`
		Tag          string `json:"group_tag"`
		Title        string `json:"title"`
		Up           int32  `json:"up"`
		Down         int32  `json:"down"`
		Absolute     int32  `json:"absolute"`
		Score        int64  `json:"score"`
		CreationTime int64  `json:"creation_time"`
	}

	postInfoSlice []resPostInfo

	resGroupPosts struct {
		Posts postInfoSlice `json:"posts"`
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

func (p *postService) mountGroup(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
	db := p.db.DB()

	r.GET("/:group", func(c echo.Context) error {
		var amt, ofs int
		if amount, err := strconv.Atoi(c.QueryParam("amount")); err == nil {
			amt = amount
		} else {
			return governor.NewErrorUser(moduleIDReqValid, "amount invalid", 0, http.StatusBadRequest)
		}
		if offset, err := strconv.Atoi(c.QueryParam("offset")); err == nil {
			ofs = offset
		} else {
			return governor.NewErrorUser(moduleIDReqValid, "offset invalid", 0, http.StatusBadRequest)
		}
		rgroup := &reqGroupGetPosts{
			Group:  c.Param("group"),
			Amount: amt,
			Offset: ofs,
		}
		if err := rgroup.valid(); err != nil {
			return err
		}

		postsSlice, err := postmodel.GetGroup(db, rgroup.Group, rgroup.Amount, rgroup.Offset)
		if err != nil {
			err.AddTrace(moduleIDGroup)
			return err
		}

		if len(postsSlice) == 0 {
			return c.NoContent(http.StatusNotFound)
		}

		posts := make(postInfoSlice, 0, len(postsSlice))
		for _, i := range postsSlice {
			postuid, _ := postmodel.ParseUIDToB64(i.Postid)
			useruid, _ := postmodel.ParseUIDToB64(i.Userid)

			posts = append(posts, resPostInfo{
				Postid:       postuid.Base64(),
				Userid:       useruid.Base64(),
				Tag:          i.Tag,
				Title:        i.Title,
				Up:           i.Up,
				Down:         i.Down,
				Absolute:     i.Absolute,
				Score:        i.Score,
				CreationTime: i.CreationTime,
			})
		}

		return c.JSON(http.StatusOK, &resGroupPosts{
			Posts: posts,
		})
	})
	return nil
}
