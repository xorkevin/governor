package post

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/post/comment/model"
	"github.com/hackform/governor/service/post/model"
	"github.com/hackform/governor/service/post/vote/model"
	"github.com/hackform/governor/service/user/gate"
	"github.com/labstack/echo"
	"net/http"
	"time"
)

const (
	actionLock = iota
	actionUnlock
	actionUpvote
	actionDownvote
	actionRmvote
)

type (
	reqPostPost struct {
		Tag     string `json:"-"`
		Title   string `json:"title"`
		Content string `json:"content"`
	}

	reqPostPut struct {
		Content string `json:"content"`
	}

	reqPostAction struct {
		Action string `json:"-"`
	}

	reqPostGet struct {
		Postid string `json:"-"`
	}

	resPostUpdate struct {
		Postid string `json:"postid"`
	}

	resPost struct {
		Postid       string `json:"postid"`
		Userid       string `json:"userid"`
		Tag          string `json:"group_tag"`
		Title        string `json:"title"`
		Content      string `json:"content"`
		Original     string `json:"original,omitempty"`
		Edited       bool   `json:"edited"`
		Locked       bool   `json:"locked"`
		Up           int32  `json:"up"`
		Down         int32  `json:"down"`
		Absolute     int32  `json:"absolute"`
		Score        int64  `json:"score"`
		CreationTime int64  `json:"creation_time"`
	}
)

func (r *reqPostPost) valid() *governor.Error {
	if err := validTitle(r.Title); err != nil {
		return err
	}
	if err := validContent(r.Content); err != nil {
		return err
	}
	if err := validGroup(r.Tag); err != nil {
		return err
	}
	return nil
}

func (r *reqPostPut) valid() *governor.Error {
	if err := validContent(r.Content); err != nil {
		return err
	}
	return nil
}

func (r *reqPostAction) valid() *governor.Error {
	if err := validAction(r.Action); err != nil {
		return err
	}
	return nil
}

func (r *reqPostGet) valid() *governor.Error {
	if err := hasPostid(r.Postid); err != nil {
		return err
	}
	return nil
}

func (p *postService) archiveGate(idparam string, checkLocked bool) echo.MiddlewareFunc {
	if idparam == "" {
		panic("idparam cannot be empty")
	}

	db := p.db.DB()
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			rpost := reqPostGet{
				Postid: c.Param(idparam),
			}
			if err := rpost.valid(); err != nil {
				return err
			}

			m, err := postmodel.GetByIDB64(db, rpost.Postid)
			if err != nil {
				if err.Code() == 2 {
					err.SetErrorUser()
				}
				err.AddTrace(moduleIDPost)
				return err
			}

			if time.Now().Unix()-m.CreationTime > p.archiveTime {
				return governor.NewErrorUser(moduleIDPost, "post is archived", 0, http.StatusBadRequest)
			}

			if checkLocked && m.IsLocked() {
				if m.IsLocked() {
					return governor.NewErrorUser(moduleIDPost, "post is locked", 0, http.StatusConflict)
				}
			}

			c.Set("postmodel", m)

			return next(c)
		}
	}
}

func (p *postService) mountRest(conf governor.Config, r *echo.Group) error {
	db := p.db.DB()

	if err := p.mountComments(conf, r.Group("")); err != nil {
		return err
	}

	r.POST("/g/:group", func(c echo.Context) error {
		rpost := reqPostPost{}
		if err := c.Bind(&rpost); err != nil {
			return governor.NewErrorUser(moduleIDPost, err.Error(), 0, http.StatusBadRequest)
		}
		rpost.Tag = c.Param("group")
		if err := rpost.valid(); err != nil {
			return err
		}
		userid := c.Get("userid").(string)

		m, err := postmodel.New(userid, rpost.Tag, rpost.Title, rpost.Content)
		if err != nil {
			err.AddTrace(moduleIDPost)
			return err
		}

		if err := m.Insert(db); err != nil {
			if err.Code() == 3 {
				err.SetErrorUser()
			}
			err.AddTrace(moduleIDPost)
			return err
		}

		postid, _ := m.IDBase64()

		return c.JSON(http.StatusCreated, resPostUpdate{
			Postid: postid,
		})
	}, gate.UserOrBan(p.gate, "group"))

	r.PUT("/:postid", func(c echo.Context) error {
		rpost := reqPostPut{}
		if err := c.Bind(&rpost); err != nil {
			return governor.NewErrorUser(moduleIDPost, err.Error(), 0, http.StatusBadRequest)
		}
		if err := rpost.valid(); err != nil {
			return err
		}

		m := c.Get("postmodel").(*postmodel.Model)

		s1, _, _ := parseContent(m.Content)
		m.Content = assembleContent(s1, rpost.Content)

		if err := m.Update(db); err != nil {
			err.AddTrace(moduleIDPost)
			return err
		}

		return c.NoContent(http.StatusNoContent)
	}, p.archiveGate("postid", true), gate.OwnerF(p.gate, func(c echo.Context) (string, *governor.Error) {
		m := c.Get("postmodel").(*postmodel.Model)
		return m.Userid, nil
	}))

	r.PATCH("/:postid/mod/:action", func(c echo.Context) error {
		rpost := reqPostAction{
			Action: c.Param("action"),
		}
		if err := rpost.valid(); err != nil {
			return err
		}

		var action int

		switch rpost.Action {
		case "lock":
			action = actionLock
		case "unlock":
			action = actionUnlock
		default:
			return governor.NewErrorUser(moduleIDPost, "invalid action", 0, http.StatusBadRequest)
		}

		m := c.Get("postmodel").(*postmodel.Model)

		switch action {
		case actionLock:
			m.Lock()
		case actionUnlock:
			m.Unlock()
		}

		if err := m.Update(db); err != nil {
			err.AddTrace(moduleIDPost)
			return err
		}

		return c.NoContent(http.StatusNoContent)
	}, p.archiveGate("postid", false), gate.ModOrAdminF(p.gate, func(c echo.Context) (string, *governor.Error) {
		m := c.Get("postmodel").(*postmodel.Model)
		return m.Tag, nil
	}))

	r.PATCH("/:postid/:action", func(c echo.Context) error {
		rpost := reqPostAction{
			Action: c.Param("action"),
		}
		if err := rpost.valid(); err != nil {
			return err
		}

		var action int

		switch rpost.Action {
		case "upvote":
			action = actionUpvote
		case "downvote":
			action = actionDownvote
		case "rmvote":
			action = actionRmvote
		default:
			return governor.NewErrorUser(moduleIDPost, "invalid action", 0, http.StatusBadRequest)
		}

		m := c.Get("postmodel").(*postmodel.Model)
		postid, err := m.IDBase64()
		if err != nil {
			err.AddTrace(moduleIDPost)
			return err
		}
		userid := c.Get("userid").(string)

		var originalVote *votemodel.Model

		switch action {
		case actionUpvote, actionDownvote, actionRmvote:
			v, err := votemodel.GetByIDB64(db, postid, userid)
			if err == nil {
				originalVote = v
			} else if err.Code() != 2 {
				return err
			}
		}

		if originalVote == nil {
			if action == actionRmvote {
				return c.NoContent(http.StatusNoContent)
			}

			v, err := votemodel.NewUpPost(postid, m.Tag, userid)
			if err != nil {
				err.AddTrace(moduleIDPost)
				return err
			}
			switch action {
			case actionUpvote:
				v.Up()

			case actionDownvote:
				v.Down()
			}

			if err := v.Insert(db); err != nil {
				if err.Code() == 3 {
					err.SetErrorUser()
				}
				err.AddTrace(moduleIDPost)
				return err
			}
		} else if action == actionRmvote {
			if err = originalVote.Delete(db); err != nil {
				err.AddTrace(moduleIDPost)
				return err
			}
		} else {
			switch action {
			case actionUpvote:
				if originalVote.IsUp() {
					return c.NoContent(http.StatusNoContent)
				}
				originalVote.Up()

			case actionDownvote:
				if originalVote.IsDown() {
					return c.NoContent(http.StatusNoContent)
				}
				originalVote.Down()
			}

			if err := originalVote.Update(db); err != nil {
				err.AddTrace(moduleIDPost)
				return err
			}
		}

		if err := m.Rescore(db); err != nil {
			err.AddTrace(moduleIDPost)
			return err
		}

		if err := m.Update(db); err != nil {
			err.AddTrace(moduleIDPost)
			return err
		}

		return c.NoContent(http.StatusNoContent)
	}, p.archiveGate("postid", true), gate.UserOrBanF(p.gate, func(c echo.Context) (string, *governor.Error) {
		m := c.Get("postmodel").(*postmodel.Model)
		return m.Tag, nil
	}))

	r.DELETE("/:postid", func(c echo.Context) error {
		m := c.Get("postmodel").(*postmodel.Model)

		if err := votemodel.DeletePostVotes(db, m.Postid); err != nil {
			err.AddTrace(moduleIDPost)
			return err
		}

		if err := commentmodel.DeletePostComments(db, m.Postid); err != nil {
			err.AddTrace(moduleIDPost)
			return err
		}

		if err := m.Delete(db); err != nil {
			err.AddTrace(moduleIDPost)
			return err
		}

		return c.NoContent(http.StatusNoContent)
	}, p.archiveGate("postid", false), gate.OwnerModOrAdminF(p.gate, func(c echo.Context) (string, string, *governor.Error) {
		m := c.Get("postmodel").(*postmodel.Model)
		return m.Userid, m.Tag, nil
	}))

	r.GET("/:postid", func(c echo.Context) error {
		rpost := reqPostGet{
			Postid: c.Param("postid"),
		}
		if err := rpost.valid(); err != nil {
			return err
		}

		m, err := postmodel.GetByIDB64(db, rpost.Postid)
		if err != nil {
			if err.Code() == 2 {
				err.SetErrorUser()
			}
			err.AddTrace(moduleIDPost)
			return err
		}

		postid, _ := m.IDBase64()

		r := resPost{
			Postid:       postid,
			Userid:       m.Userid,
			Tag:          m.Tag,
			Title:        m.Title,
			Locked:       m.Locked,
			Up:           m.Up,
			Down:         m.Down,
			Absolute:     m.Absolute,
			Score:        m.Score,
			CreationTime: m.CreationTime,
		}

		s1, s2, edited := parseContent(m.Content)

		if edited {
			r.Content = s2
			r.Original = s1
		} else {
			r.Content = s1
		}
		r.Edited = edited

		return c.JSON(http.StatusOK, r)
	}, p.cc.Control(true, false, min2, nil))
	return nil
}
