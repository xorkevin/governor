package post

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/post/model"
	"github.com/hackform/governor/service/post/vote/model"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
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
		Userid  string `json:"-"`
		Tag     string `json:"-"`
		Title   string `json:"title"`
		Content string `json:"content"`
	}

	reqPostPut struct {
		Postid  string `json:"-"`
		Content string `json:"content"`
	}

	reqPostAction struct {
		Postid string `json:"-"`
		Action string `json:"-"`
	}

	reqPostActionUser struct {
		Postid string `json:"-"`
		Userid string `json:"-"`
		Action string `json:"-"`
	}

	reqPostGet struct {
		Postid string `json:"postid"`
	}

	resPost struct {
		Postid       []byte `json:"postid"`
		Userid       []byte `json:"userid"`
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
	if err := hasUserid(r.Userid); err != nil {
		return err
	}
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
	if err := hasPostid(r.Postid); err != nil {
		return err
	}
	if err := validContent(r.Content); err != nil {
		return err
	}
	return nil
}

func (r *reqPostAction) valid() *governor.Error {
	if err := hasPostid(r.Postid); err != nil {
		return err
	}
	if err := validAction(r.Action); err != nil {
		return err
	}
	return nil
}

func (r *reqPostActionUser) valid() *governor.Error {
	if err := hasPostid(r.Postid); err != nil {
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

func (r *reqPostGet) valid() *governor.Error {
	if err := hasPostid(r.Postid); err != nil {
		return err
	}
	return nil
}

func (p *Post) archiveGate(idparam string, checkLocked bool) echo.MiddlewareFunc {
	db := p.db.DB()
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			m, err := postmodel.GetByIDB64(db, c.Param(idparam))
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

func (p *Post) mountRest(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
	db := p.db.DB()

	if err := p.mountComments(conf, r.Group("/"), l); err != nil {
		return err
	}

	r.POST("/g/:group", func(c echo.Context) error {
		rpost := &reqPostPost{}
		if err := c.Bind(rpost); err != nil {
			return governor.NewErrorUser(moduleIDPost, err.Error(), 0, http.StatusBadRequest)
		}
		rpost.Userid = c.Get("userid").(string)
		rpost.Tag = c.Param("group")
		if err := rpost.valid(); err != nil {
			return err
		}

		m, err := postmodel.New(rpost.Userid, rpost.Tag, rpost.Title, rpost.Content)
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

		t, _ := time.Now().MarshalText()
		postid, _ := m.IDBase64()
		userid, _ := m.UserIDBase64()
		l.WithFields(logrus.Fields{
			"time":   string(t),
			"origin": moduleIDPost,
			"postid": postid,
			"userid": userid,
			"group":  m.Tag,
			"title":  m.Title,
		}).Info("post created")

		return c.NoContent(http.StatusNoContent)
	}, p.gate.UserOrBan("group"))

	r.PUT("/:id", func(c echo.Context) error {
		rpost := &reqPostPut{}
		if err := c.Bind(rpost); err != nil {
			return governor.NewErrorUser(moduleIDPost, err.Error(), 0, http.StatusBadRequest)
		}
		rpost.Postid = c.Param("id")
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
	}, p.archiveGate("id", true), p.gate.OwnerF(func(c echo.Context) (string, *governor.Error) {
		m := c.Get("postmodel").(*postmodel.Model)
		return m.UserIDBase64()
	}))

	r.PATCH("/:id/mod/:action", func(c echo.Context) error {
		rpost := &reqPostAction{
			Postid: c.Param("id"),
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
	}, p.archiveGate("id", false), p.gate.ModOrAdminF(func(c echo.Context) (string, *governor.Error) {
		m := c.Get("postmodel").(*postmodel.Model)
		return m.Tag, nil
	}))

	r.PATCH("/:id/:action", func(c echo.Context) error {
		rpost := &reqPostActionUser{
			Postid: c.Param("id"),
			Userid: c.Get("userid").(string),
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

		switch action {
		case actionUpvote:
			v, err := votemodel.NewUpPost(rpost.Postid, m.Tag, rpost.Userid)
			if err != nil {
				err.AddTrace(moduleIDPost)
				return err
			}
			if err := v.Insert(db); err != nil {
				if err.Code() == 3 {
					err.SetErrorUser()
				}
				err.AddTrace(moduleIDPost)
				return err
			}
		case actionDownvote:
			v, err := votemodel.NewDownPost(rpost.Postid, m.Tag, rpost.Userid)
			if err != nil {
				err.AddTrace(moduleIDPost)
				return err
			}
			if err := v.Insert(db); err != nil {
				if err.Code() == 3 {
					err.SetErrorUser()
				}
				err.AddTrace(moduleIDPost)
				return err
			}
		case actionRmvote:
			v, err := votemodel.GetByIDB64(db, rpost.Postid, rpost.Userid)
			if err != nil {
				if err.Code() == 2 {
					err.SetErrorUser()
				}
				return err
			}
			if err := v.Delete(db); err != nil {
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
	}, p.archiveGate("id", true), p.gate.UserOrBanF(func(c echo.Context) (string, *governor.Error) {
		m := c.Get("postmodel").(*postmodel.Model)
		return m.Tag, nil
	}))

	r.DELETE("/:id", func(c echo.Context) error {
		m := c.Get("postmodel").(*postmodel.Model)

		if err := m.Delete(db); err != nil {
			err.AddTrace(moduleIDPost)
			return err
		}

		return c.NoContent(http.StatusNoContent)
	}, p.archiveGate("id", false), p.gate.OwnerModOrAdminF(func(c echo.Context) (string, string, *governor.Error) {
		m := c.Get("postmodel").(*postmodel.Model)
		s, err := m.UserIDBase64()
		if err != nil {
			err.AddTrace(moduleIDPost)
			return "", "", err
		}
		return s, m.Tag, nil
	}))

	r.GET("/:id", func(c echo.Context) error {
		rpost := &reqPostGet{
			Postid: c.Param("id"),
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

		r := &resPost{
			Postid:       m.Postid,
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
	})
	return nil
}
