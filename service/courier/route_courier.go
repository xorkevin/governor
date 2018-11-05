package courier

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/gate"
	"github.com/labstack/echo"
	"net/http"
)

type (
	reqLinkPost struct {
		CreatorID string `json:"-"`
		LinkID    string `json:"linkid"`
		URL       string `json:"url"`
	}
)

func (r *reqLinkPost) valid() *governor.Error {
	if err := hasCreatorID(r.CreatorID); err != nil {
		return err
	}
	if err := validLinkID(r.LinkID); err != nil {
		return err
	}
	if err := validURL(r.URL); err != nil {
		return err
	}
	return nil
}

func (cr *courierRouter) createLink(c echo.Context) error {
	userid := c.Get("userid").(string)

	rlink := reqLinkPost{}
	if err := c.Bind(&rlink); err != nil {
		return governor.NewErrorUser(moduleID, err.Error(), 0, http.StatusBadRequest)
	}
	rlink.CreatorID = userid
	if err := rlink.valid(); err != nil {
		return err
	}

	res, err := cr.service.CreateLink(rlink.LinkID, rlink.URL, rlink.CreatorID)
	if err != nil {
		if err.Code() == 3 {
			err.SetErrorUser()
		}
		return err
	}
	return c.JSON(http.StatusCreated, res)
}

func (cr *courierRouter) mountRoutes(conf governor.Config, r *echo.Group) error {
	r.POST("/link", cr.createLink, gate.User(cr.service.gate))
	return nil
}
