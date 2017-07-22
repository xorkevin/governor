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

	reqPostDelete struct {
		Postid string `json:"-"`
		Userid string `json:"-"`
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

func (r *reqPostDelete) valid() *governor.Error {
	if err := hasPostid(r.Postid); err != nil {
		return err
	}
	if err := hasUserid(r.Userid); err != nil {
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

		m, err := postmodel.GetByIDB64(db, rpost.Postid)
		if err != nil {
			if err.Code() == 2 {
				err.SetErrorUser()
			}
			return err
		}

		if m.IsLocked() {
			return governor.NewErrorUser(moduleIDPost, "post is locked", 0, http.StatusConflict)
		}

		m.Content = rpost.Content

		if err := m.Update(db); err != nil {
			err.AddTrace(moduleIDPost)
			return err
		}

		return c.NoContent(http.StatusNoContent)
	}, p.gate.OwnerF("id", func(postid string) (string, *governor.Error) {
		m, err := postmodel.GetByIDB64(db, postid)
		if err != nil {
			if err.Code() == 2 {
				err.SetErrorUser()
			}
			err.AddTrace(moduleIDPost)
			return "", err
		}
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

		m, err := postmodel.GetByIDB64(db, rpost.Postid)
		if err != nil {
			if err.Code() == 2 {
				err.SetErrorUser()
			}
			return err
		}

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
	}, p.gate.ModOrAdminF("id", func(postid string) (string, *governor.Error) {
		m, err := postmodel.GetByIDB64(db, postid)
		if err != nil {
			if err.Code() == 2 {
				err.SetErrorUser()
			}
			err.AddTrace(moduleIDPost)
			return "", err
		}
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

		m, err := postmodel.GetByIDB64(db, rpost.Postid)
		if err != nil {
			if err.Code() == 2 {
				err.SetErrorUser()
			}
			err.AddTrace(moduleIDPost)
			return err
		}

		if m.IsLocked() {
			return governor.NewErrorUser(moduleIDPost, "post is locked", 0, http.StatusConflict)
		}

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
	}, p.gate.UserOrBanF("id", func(postid string) (string, *governor.Error) {
		m, err := postmodel.GetByIDB64(db, postid)
		if err != nil {
			if err.Code() == 2 {
				err.SetErrorUser()
			}
			err.AddTrace(moduleIDPost)
			return "", err
		}
		return m.Tag, nil
	}))

	r.DELETE("/:id", func(c echo.Context) error {
		rpost := &reqPostDelete{
			Postid: c.Param("id"),
			Userid: c.Get("userid").(string),
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

		if err := m.Delete(db); err != nil {
			err.AddTrace(moduleIDPost)
			return err
		}

		return c.NoContent(http.StatusNoContent)
	}, p.gate.OwnerModOrAdminF("id", func(postid string) (string, string, *governor.Error) {
		m, err := postmodel.GetByIDB64(db, postid)
		if err != nil {
			if err.Code() == 2 {
				err.SetErrorUser()
			}
			err.AddTrace(moduleIDPost)
			return "", "", err
		}
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

		return c.JSON(http.StatusOK, &resPost{
			Postid:       m.Postid,
			Userid:       m.Userid,
			Tag:          m.Tag,
			Title:        m.Title,
			Content:      m.Content,
			CreationTime: m.CreationTime,
		})
	})
	return nil
}
