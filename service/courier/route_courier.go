package courier

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/gate"
	"github.com/labstack/echo"
	"net/http"
	"strconv"
)

//go:generate forge validation -o validation_courier_gen.go reqLinkGet reqLinkGetGroup reqLinkPost

type (
	reqLinkGet struct {
		LinkID string `valid:"linkID,has" json:"-"`
	}
)

func (cr *courierRouter) getLink(c echo.Context) error {
	rlink := reqLinkGet{
		LinkID: c.Param("linkid"),
	}
	if err := rlink.valid(); err != nil {
		return err
	}
	url, err := cr.service.GetLinkFast(rlink.LinkID)
	if err != nil {
		if len(cr.service.fallbackLink) > 0 {
			return c.Redirect(http.StatusMovedPermanently, cr.service.fallbackLink)
		}
		return err
	}
	return c.Redirect(http.StatusTemporaryRedirect, url)
}

func (cr *courierRouter) getLinkImage(c echo.Context) error {
	rlink := reqLinkGet{
		LinkID: c.Param("linkid"),
	}
	if err := rlink.valid(); err != nil {
		return err
	}
	qrimage, contentType, err := cr.service.GetLinkImage(rlink.LinkID)
	if err != nil {
		return err
	}
	return c.Stream(http.StatusOK, contentType, qrimage)
}

type (
	reqLinkGetGroup struct {
		Amount int `valid:"amount" json:"-"`
		Offset int `valid:"offset" json:"-"`
	}
)

func (cr *courierRouter) getLinkGroup(c echo.Context) error {
	amount, err := strconv.Atoi(c.QueryParam("amount"))
	if err != nil {
		return governor.NewErrorUser("Amount invalid", http.StatusBadRequest, err)
	}
	offset, err := strconv.Atoi(c.QueryParam("offset"))
	if err != nil {
		return governor.NewErrorUser("Offset invalid", http.StatusBadRequest, err)
	}

	rlink := reqLinkGetGroup{
		Amount: amount,
		Offset: offset,
	}
	if err := rlink.valid(); err != nil {
		return err
	}

	res, err := cr.service.GetLinkGroup(amount, offset, c.QueryParam("creatorid"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, res)
}

type (
	reqLinkPost struct {
		CreatorID string `valid:"creatorID,has" json:"-"`
		LinkID    string `valid:"linkID" json:"linkid"`
		URL       string `valid:"URL" json:"url"`
	}
)

func (cr *courierRouter) createLink(c echo.Context) error {
	rlink := reqLinkPost{}
	if err := c.Bind(&rlink); err != nil {
		return err
	}
	rlink.CreatorID = c.Get("userid").(string)
	if err := rlink.valid(); err != nil {
		return err
	}

	res, err := cr.service.CreateLink(rlink.LinkID, rlink.URL, rlink.CreatorID)
	if err != nil {
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
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (cr *courierRouter) gateModOrAdmin(c echo.Context) (string, error) {
	return "website", nil
}

func (cr *courierRouter) getLinkImageCC(c echo.Context) (string, error) {
	rlink := reqLinkGet{
		LinkID: c.Param("linkid"),
	}
	if err := rlink.valid(); err != nil {
		return "", err
	}

	objinfo, err := cr.service.StatLinkImage(rlink.LinkID)
	if err != nil {
		return "", err
	}

	return objinfo.ETag, nil
}

func (cr *courierRouter) mountRoutes(conf governor.Config, r *echo.Group) error {
	r.GET("/link/:linkid", cr.getLink)
	r.GET("/link/:linkid/image", cr.getLinkImage, cr.service.cc.Control(true, false, min15, cr.getLinkImageCC))
	r.GET("/link", cr.getLinkGroup, gate.ModOrAdminF(cr.service.gate, cr.gateModOrAdmin))
	r.POST("/link", cr.createLink, gate.ModOrAdminF(cr.service.gate, cr.gateModOrAdmin))
	r.DELETE("/link/:linkid", cr.deleteLink, gate.ModOrAdminF(cr.service.gate, cr.gateModOrAdmin))
	return nil
}
