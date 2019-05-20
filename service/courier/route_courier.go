package courier

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/gate"
	"github.com/labstack/echo"
	"net/http"
	"strconv"
)

type (
	reqLinkGet struct {
		LinkID string `json:"-"`
	}

	reqLinkGetGroup struct {
		Amount int `json:"-"`
		Offset int `json:"-"`
	}

	reqLinkPost struct {
		CreatorID string `json:"-"`
		LinkID    string `json:"linkid"`
		URL       string `json:"url"`
	}
)

func (r *reqLinkGet) valid() error {
	return hasLinkID(r.LinkID)
}

func (r *reqLinkGetGroup) valid() error {
	if err := validAmount(r.Amount); err != nil {
		return err
	}
	if err := validOffset(r.Offset); err != nil {
		return err
	}
	return nil
}

func (r *reqLinkPost) valid() error {
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

func (cr *courierRouter) getLinkGroup(c echo.Context) error {
	var amt, ofs int
	if amount, err := strconv.Atoi(c.QueryParam("amount")); err == nil {
		amt = amount
	} else {
		return governor.NewErrorUser("Amount invalid", http.StatusBadRequest, err)
	}
	if offset, err := strconv.Atoi(c.QueryParam("offset")); err == nil {
		ofs = offset
	} else {
		return governor.NewErrorUser("Offset invalid", http.StatusBadRequest, err)
	}

	rlink := reqLinkGetGroup{
		Amount: amt,
		Offset: ofs,
	}
	if err := rlink.valid(); err != nil {
		return err
	}

	res, err := cr.service.GetLinkGroup(amt, ofs, c.QueryParam("creatorid"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, res)
}

func (cr *courierRouter) createLink(c echo.Context) error {
	userid := c.Get("userid").(string)

	rlink := reqLinkPost{}
	if err := c.Bind(&rlink); err != nil {
		return err
	}
	rlink.CreatorID = userid
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

	objinfo, err := cr.service.linkImageBucket.Stat(rlink.LinkID + "-qr")
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return "", governor.NewErrorUser("", 0, err)
		}
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
