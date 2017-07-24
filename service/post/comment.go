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
		Userid   string `json:"-"`
		Postid   string `json:"-"`
		Parentid string `json:"parentid"`
		Content  string `json:"content"`
	}

	reqPutComment struct {
		Userid    string `json:"-"`
		Postid    string `json:"-"`
		Commentid string `json:"-"`
		Content   string `json:"content"`
	}

	reqCommentAction struct {
		Postid    string `json:"-"`
		Commentid string `json:"-"`
		Userid    string `json:"-"`
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
		Amount    int    `json:"amount"`
		Offset    int    `json:"offset"`
	}

	resUpdateComment struct {
		Commentid []byte `json:"commentid"`
	}

	resGetComments struct {
		Comments commentmodel.ModelSlice `json:"comments"`
	}

	resGetComment struct {
		commentmodel.Model
	}
)

func (r *reqPostComment) valid() *governor.Error {
	if err := hasPostid(r.Postid); err != nil {
		return err
	}
	if err := hasPostid(r.Parentid); err != nil {
		return err
	}
	if err := hasUserid(r.Userid); err != nil {
		return err
	}
	if err := validContent(r.Content); err != nil {
		return err
	}
	return nil
}

func (r *reqPutComment) valid() *governor.Error {
	if err := hasPostid(r.Postid); err != nil {
		return err
	}
	if err := hasPostid(r.Commentid); err != nil {
		return err
	}
	if err := hasUserid(r.Userid); err != nil {
		return err
	}
	if err := validContent(r.Content); err != nil {
		return err
	}
	return nil
}

func (r *reqCommentAction) valid() *governor.Error {
	if err := hasPostid(r.Postid); err != nil {
		return err
	}
	if err := hasPostid(r.Commentid); err != nil {
		return err
	}
	if err := hasUserid(r.Userid); err != nil {
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
		rcomms.Postid = c.Param("postid")
		rcomms.Userid = c.Get("userid").(string)
		if err := rcomms.valid(); err != nil {
			return err
		}

		if _, err := postmodel.GetByIDB64(db, rcomms.Postid); err != nil {
			if err.Code() == 2 {
				err.SetErrorUser()
			}
			err.AddTrace(moduleIDComments)
			return err
		}

		mComment, err := commentmodel.New(rcomms.Userid, rcomms.Postid, rcomms.Parentid, rcomms.Content)
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
	})

	r.PUT("/:postid/c/:commentid", func(c echo.Context) error {
		rcomms := &reqPutComment{}
		if err := c.Bind(rcomms); err != nil {
			return governor.NewErrorUser(moduleIDComments, err.Error(), 0, http.StatusBadRequest)
		}
		rcomms.Postid = c.Param("postid")
		rcomms.Commentid = c.Param("commentid")
		rcomms.Userid = c.Get("userid").(string)
		if err := rcomms.valid(); err != nil {
			return err
		}

		mComment, err := commentmodel.GetByIDB64(db, rcomms.Commentid, rcomms.Postid)
		if err != nil {
			if err.Code() == 2 {
				err.SetErrorUser()
			}
			err.AddTrace(moduleIDComments)
			return err
		}

		mComment.Content = rcomms.Content

		if err := mComment.Update(db); err != nil {
			return err
		}

		return c.JSON(http.StatusOK, resUpdateComment{
			Commentid: mComment.Commentid,
		})
	}, p.gate.OwnerFM(func(paramValues ...string) (string, *governor.Error) {
		m, err := commentmodel.GetByIDB64(db, paramValues[1], paramValues[0])
		if err != nil {
			if err.Code() == 2 {
				err.SetErrorUser()
			}
			err.AddTrace(moduleIDComments)
			return "", err
		}
		return m.UserIDBase64()
	}, "postid", "commentid"))

	r.PATCH("/:postid/c/:commentid/:action", func(c echo.Context) error {
		rcomm := &reqCommentAction{
			Postid:    c.Param("postid"),
			Commentid: c.Param("commentid"),
			Userid:    c.Get("userid").(string),
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

		post, err := postmodel.GetByIDB64(db, rcomm.Postid)
		if err != nil {
			if err.Code() == 2 {
				err.SetErrorUser()
			}
			err.AddTrace(moduleIDComments)
			return err
		}

		if post.IsLocked() {
			return governor.NewErrorUser(moduleIDComments, "post is locked", 0, http.StatusConflict)
		}

		comm, err := commentmodel.GetByIDB64(db, rcomm.Commentid, rcomm.Postid)
		if err != nil {
			if err.Code() == 2 {
				err.SetErrorUser()
			}
			err.AddTrace(moduleIDComments)
			return err
		}

		switch action {
		case actionUpvote:
			v, err := votemodel.NewUp(rcomm.Commentid, rcomm.Postid, post.Tag, rcomm.Userid)
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
			v, err := votemodel.NewDown(rcomm.Commentid, rcomm.Postid, post.Tag, rcomm.Userid)
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
			v, err := votemodel.GetByIDB64(db, rcomm.Commentid, rcomm.Userid)
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
	}, p.gate.UserOrBanF("postid", func(postid string) (string, *governor.Error) {
		m, err := postmodel.GetByIDB64(db, postid)
		if err != nil {
			if err.Code() == 2 {
				err.SetErrorUser()
			}
			err.AddTrace(moduleIDComments)
			return "", err
		}
		return m.Tag, nil
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

		return c.JSON(http.StatusOK, &resGetComments{
			Comments: commentsSlice,
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

		return c.JSON(http.StatusOK, &resGetComment{
			Model: *comment,
		})
	})

	r.GET("/:postid/c/:commentid/children", func(c echo.Context) error {
		rcomms := &reqGetComment{}
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

		return c.JSON(http.StatusOK, &resGetComments{
			Comments: comments,
		})
	})

	return nil
}
