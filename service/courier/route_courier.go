package courier

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/gate"
	"github.com/labstack/echo"
	"net/http"
)

type (
	reqLinkGet struct {
		LinkID string `json:"-"`
	}

	reqLinkPost struct {
		CreatorID string `json:"-"`
		LinkID    string `json:"linkid"`
		URL       string `json:"url"`
	}
)

func (r *reqLinkGet) valid() *governor.Error {
	return hasLinkID(r.LinkID)
}

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

func (cr *courierRouter) getLink(c echo.Context) error {
	rlink := reqLinkGet{
		LinkID: c.Param("linkid"),
	}
	if err := rlink.valid(); err != nil {
		return err
	}
	res, err := cr.service.GetLink(rlink.LinkID)
	if err != nil {
		if len(cr.service.fallbackLink) > 0 {
			return c.Redirect(http.StatusMovedPermanently, cr.service.fallbackLink)
		}
		if err.Code() == 2 {
			err.SetErrorUser()
		}
		return err
	}
	return c.Redirect(http.StatusMovedPermanently, res.URL)
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

func (cr *courierRouter) deleteLink(c echo.Context) error {
	rlink := reqLinkGet{
		LinkID: c.Param("linkid"),
	}
	if err := rlink.valid(); err != nil {
		return err
	}
	if err := cr.service.DeleteLink(rlink.LinkID); err != nil {
		if err.Code() == 2 {
			err.SetErrorUser()
		}
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (cr *courierRouter) gateModOrAdmin(c echo.Context) (string, *governor.Error) {
	return "website", nil
}

func (cr *courierRouter) mountRoutes(conf governor.Config, r *echo.Group) error {
	r.GET("/link/:linkid", cr.getLink)
	r.POST("/link", cr.createLink, gate.ModOrAdminF(cr.service.gate, cr.gateModOrAdmin))
	r.DELETE("/link/:linkid", cr.deleteLink, gate.ModOrAdminF(cr.service.gate, cr.gateModOrAdmin))
	return nil
}
