package post

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/post/comment/model"
	"github.com/hackform/governor/service/post/model"
	"github.com/hackform/governor/service/post/vote/model"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"net/http"
)

type (
	reqPostComment struct {
		Parentid string `json:"parentid"`
		Content  string `json:"content"`
	}

	reqPutComment struct {
		Content string `json:"content"`
	}

	reqCommentAction struct {
		Commentid string `json:"-"`
		Action    string `json:"-"`
	}

	reqGetComments struct {
		Postid string `json:"-"`
		Amount int    `json:"amount"`
		Offset int    `json:"offset"`
	}

	reqGetComment struct {
		Postid    string `json:"-"`
		Commentid string `json:"-"`
	}

	reqGetCommentChildren struct {
		Postid    string `json:"-"`
		Commentid string `json:"-"`
		Amount    int    `json:"amount"`
		Offset    int    `json:"offset"`
	}

	resUpdateComment struct {
		Commentid []byte `json:"commentid"`
	}

	resGetComment struct {
		Commentid    []byte `json:"commentid"`
		Parentid     []byte `json:"parentid"`
		Postid       []byte `json:"postid"`
		Userid       []byte `json:"userid"`
		Content      string `json:"content"`
		Original     string `json:"original,omitempty"`
		Edited       bool   `json:"edited"`
		Up           int32  `json:"up"`
		Down         int32  `json:"down"`
		Absolute     int32  `json:"absolute"`
		Score        int64  `json:"score"`
		CreationTime int64  `json:"creation_time"`
	}

	resCommentSlice []resGetComment

	resGetComments struct {
		Comments resCommentSlice `json:"comments"`
	}
)

func (r *reqPostComment) valid() *governor.Error {
	if err := hasPostid(r.Parentid); err != nil {
		return err
	}
	if err := validContent(r.Content); err != nil {
		return err
	}
	return nil
}

func (r *reqPutComment) valid() *governor.Error {
	if err := validContent(r.Content); err != nil {
		return err
	}
	return nil
}

func (r *reqCommentAction) valid() *governor.Error {
	if err := hasPostid(r.Commentid); err != nil {
		return err
	}
	if err := validAction(r.Action); err != nil {
		return err
	}
	return nil
}

func (r *reqGetComments) valid() *governor.Error {
	if err := hasPostid(r.Postid); err != nil {
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

func (r *reqGetComment) valid() *governor.Error {
	if err := hasPostid(r.Postid); err != nil {
		return err
	}
	if err := hasPostid(r.Commentid); err != nil {
		return err
	}
	return nil
}

func (r *reqGetCommentChildren) valid() *governor.Error {
	if err := hasPostid(r.Postid); err != nil {
		return err
	}
	if err := hasPostid(r.Commentid); err != nil {
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

func (p *Post) mountComments(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
	db := p.db.DB()

	r.POST("/:postid/c", func(c echo.Context) error {
		rcomms := &reqPostComment{}
		if err := c.Bind(rcomms); err != nil {
			return governor.NewErrorUser(moduleIDComments, err.Error(), 0, http.StatusBadRequest)
		}
		if err := rcomms.valid(); err != nil {
			return err
		}

		m := c.Get("postmodel").(*postmodel.Model)
		postid, err := m.IDBase64()
		if err != nil {
			err.AddTrace(moduleIDPost)
			return err
		}
		userid := c.Get("userid").(string)

		mComment, err := commentmodel.New(userid, postid, rcomms.Parentid, rcomms.Content)
		if err != nil {
			err.AddTrace(moduleIDComments)
			return err
		}

		if err := mComment.Insert(db); err != nil {
			return err
		}

		return c.JSON(http.StatusCreated, resUpdateComment{
			Commentid: mComment.Commentid,
		})
	}, p.archiveGate("postid", true))

	r.PUT("/:postid/c/:commentid", func(c echo.Context) error {
		rcomms := &reqPutComment{}
		if err := c.Bind(rcomms); err != nil {
			return governor.NewErrorUser(moduleIDComments, err.Error(), 0, http.StatusBadRequest)
		}
		if err := rcomms.valid(); err != nil {
			return err
		}

		mComment := c.Get("commentmodel").(*commentmodel.Model)

		s1, _, _ := parseContent(mComment.Content)

		mComment.Content = assembleContent(s1, rcomms.Content)

		if err := mComment.Update(db); err != nil {
			return err
		}

		return c.JSON(http.StatusOK, resUpdateComment{
			Commentid: mComment.Commentid,
		})
	}, p.archiveGate("postid", true), p.gate.OwnerF(func(c echo.Context) (string, *governor.Error) {
		rcomm := &reqGetComment{
			Postid:    c.Param("postid"),
			Commentid: c.Param("commentid"),
		}
		if err := rcomm.valid(); err != nil {
			return "", err
		}

		m, err := commentmodel.GetByIDB64(db, rcomm.Commentid, rcomm.Postid)
		if err != nil {
			if err.Code() == 2 {
				err.SetErrorUser()
			}
			err.AddTrace(moduleIDComments)
			return "", err
		}
		c.Set("commentmodel", m)
		return m.UserIDBase64()
	}))

	r.PATCH("/:postid/c/:commentid/:action", func(c echo.Context) error {
		rcomm := &reqCommentAction{
			Commentid: c.Param("commentid"),
			Action:    c.Param("action"),
		}
		if err := rcomm.valid(); err != nil {
			return err
		}

		var action int

		switch rcomm.Action {
		case "upvote":
			action = actionUpvote
		case "downvote":
			action = actionDownvote
		case "rmvote":
			action = actionRmvote
		default:
			return governor.NewErrorUser(moduleIDComments, "invalid action", 0, http.StatusBadRequest)
		}

		post := c.Get("postmodel").(*postmodel.Model)
		postid, err := post.IDBase64()
		if err != nil {
			err.AddTrace(moduleIDPost)
			return err
		}
		userid := c.Get("userid").(string)

		comm, err := commentmodel.GetByIDB64(db, rcomm.Commentid, postid)
		if err != nil {
			if err.Code() == 2 {
				err.SetErrorUser()
			}
			err.AddTrace(moduleIDComments)
			return err
		}

		switch action {
		case actionUpvote:
			v, err := votemodel.NewUp(rcomm.Commentid, postid, post.Tag, userid)
			if err != nil {
				err.AddTrace(moduleIDComments)
				return err
			}
			if err := v.Insert(db); err != nil {
				if err.Code() == 3 {
					err.SetErrorUser()
				}
				err.AddTrace(moduleIDComments)
				return err
			}
		case actionDownvote:
			v, err := votemodel.NewDown(rcomm.Commentid, postid, post.Tag, userid)
			if err != nil {
				err.AddTrace(moduleIDComments)
				return err
			}
			if err := v.Insert(db); err != nil {
				if err.Code() == 3 {
					err.SetErrorUser()
				}
				err.AddTrace(moduleIDComments)
				return err
			}
		case actionRmvote:
			v, err := votemodel.GetByIDB64(db, rcomm.Commentid, userid)
			if err != nil {
				if err.Code() == 2 {
					err.SetErrorUser()
				}
				return err
			}
			if err := v.Delete(db); err != nil {
				err.AddTrace(moduleIDComments)
				return err
			}
		}

		if err := comm.Rescore(db); err != nil {
			err.AddTrace(moduleIDComments)
			return err
		}

		if err := comm.Update(db); err != nil {
			err.AddTrace(moduleIDComments)
			return err
		}

		return c.NoContent(http.StatusNoContent)
	}, p.archiveGate("postid", true), p.gate.UserOrBanF(func(c echo.Context) (string, *governor.Error) {
		m := c.Get("postmodel").(*postmodel.Model)
		return m.Tag, nil
	}))

	r.DELETE("/:postid/c/:commentid", func(c echo.Context) error {
		mComment := c.Get("commentmodel").(*commentmodel.Model)

		if err := mComment.Delete(db); err != nil {
			err.AddTrace(moduleIDComments)
			return err
		}

		return nil
	}, p.archiveGate("postid", false), p.gate.OwnerModOrAdminF(func(c echo.Context) (string, string, *governor.Error) {
		post := c.Get("postmodel").(*postmodel.Model)
		m, err := commentmodel.GetByIDB64(db, c.Param("commentid"), c.Param("postid"))
		if err != nil {
			if err.Code() == 2 {
				err.SetErrorUser()
			}
			err.AddTrace(moduleIDComments)
			return "", "", err
		}
		s, err := m.UserIDBase64()
		if err != nil {
			err.AddTrace(moduleIDPost)
			return "", "", err
		}
		c.Set("commentmodel", m)
		return s, post.Tag, nil
	}))

	r.GET("/:postid/c", func(c echo.Context) error {
		rcomms := &reqGetComments{}
		if err := c.Bind(rcomms); err != nil {
			return governor.NewErrorUser(moduleIDComments, err.Error(), 0, http.StatusBadRequest)
		}
		rcomms.Postid = c.Param("postid")
		if err := rcomms.valid(); err != nil {
			return err
		}

		commentsSlice, err := commentmodel.GetResponses(db, rcomms.Postid, rcomms.Amount, rcomms.Offset)
		if err != nil {
			err.AddTrace(moduleIDComments)
			return err
		}

		k := make(resCommentSlice, 0, len(commentsSlice))

		for _, i := range commentsSlice {
			r := resGetComment{
				Commentid:    i.Commentid,
				Parentid:     i.Parentid,
				Postid:       i.Postid,
				Userid:       i.Userid,
				Up:           i.Up,
				Down:         i.Down,
				Absolute:     i.Absolute,
				Score:        i.Score,
				CreationTime: i.CreationTime,
			}

			s1, s2, edited := parseContent(i.Content)
			if edited {
				r.Content = s2
				r.Original = s1
			} else {
				r.Content = s1
			}
			r.Edited = edited

			k = append(k, r)
		}

		return c.JSON(http.StatusOK, &resGetComments{
			Comments: k,
		})
	})

	r.GET("/:postid/c/:commentid", func(c echo.Context) error {
		rcomms := &reqGetComment{}
		if err := c.Bind(rcomms); err != nil {
			return governor.NewErrorUser(moduleIDComments, err.Error(), 0, http.StatusBadRequest)
		}
		rcomms.Postid = c.Param("postid")
		rcomms.Commentid = c.Param("commentid")
		if err := rcomms.valid(); err != nil {
			return err
		}

		comment, err := commentmodel.GetByIDB64(db, rcomms.Commentid, rcomms.Postid)
		if err != nil {
			if err.Code() == 2 {
				err.SetErrorUser()
			}
			err.AddTrace(moduleIDComments)
			return err
		}

		r := &resGetComment{
			Commentid:    comment.Commentid,
			Parentid:     comment.Parentid,
			Postid:       comment.Postid,
			Userid:       comment.Userid,
			Up:           comment.Up,
			Down:         comment.Down,
			Absolute:     comment.Absolute,
			Score:        comment.Score,
			CreationTime: comment.CreationTime,
		}

		s1, s2, edited := parseContent(comment.Content)
		if edited {
			r.Content = s2
			r.Original = s1
		} else {
			r.Content = s1
		}
		r.Edited = edited

		return c.JSON(http.StatusOK, r)
	})

	r.GET("/:postid/c/:commentid/children", func(c echo.Context) error {
		rcomms := &reqGetCommentChildren{}
		if err := c.Bind(rcomms); err != nil {
			return governor.NewErrorUser(moduleIDComments, err.Error(), 0, http.StatusBadRequest)
		}
		rcomms.Postid = c.Param("postid")
		rcomms.Commentid = c.Param("commentid")
		if err := rcomms.valid(); err != nil {
			return err
		}

		comments, err := commentmodel.GetChildren(db, rcomms.Commentid, rcomms.Postid, rcomms.Amount, rcomms.Offset)
		if err != nil {
			if err.Code() == 2 {
				err.SetErrorUser()
			}
			err.AddTrace(moduleIDComments)
			return err
		}

		k := make(resCommentSlice, 0, len(comments))

		for _, i := range comments {
			r := resGetComment{
				Commentid:    i.Commentid,
				Parentid:     i.Parentid,
				Postid:       i.Postid,
				Userid:       i.Userid,
				Up:           i.Up,
				Down:         i.Down,
				Absolute:     i.Absolute,
				Score:        i.Score,
				CreationTime: i.CreationTime,
			}

			s1, s2, edited := parseContent(i.Content)
			if edited {
				r.Content = s2
				r.Original = s1
			} else {
				r.Content = s1
			}
			r.Edited = edited

			k = append(k, r)
		}

		return c.JSON(http.StatusOK, &resGetComments{
			Comments: k,
		})
	})

	return nil
}
