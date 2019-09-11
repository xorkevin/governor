package courier

import (
	"github.com/labstack/echo/v4"
	"net/http"
	"strconv"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/cachecontrol"
	"xorkevin.dev/governor/service/user/gate"
)

//go:generate forge validation -o validation_courier_gen.go reqLinkGet reqLinkGetGroup reqLinkPost

type (
	reqLinkGet struct {
		LinkID string `valid:"linkID,has" json:"-"`
	}
)

func (r *router) getLink(c echo.Context) error {
	rlink := reqLinkGet{
		LinkID: c.Param("linkid"),
	}
	if err := rlink.valid(); err != nil {
		return err
	}
	url, err := r.s.GetLinkFast(rlink.LinkID)
	if err != nil {
		if len(r.s.fallbackLink) > 0 {
			return c.Redirect(http.StatusMovedPermanently, r.s.fallbackLink)
		}
		return err
	}
	return c.Redirect(http.StatusTemporaryRedirect, url)
}

func (r *router) getLinkImage(c echo.Context) error {
	rlink := reqLinkGet{
		LinkID: c.Param("linkid"),
	}
	if err := rlink.valid(); err != nil {
		return err
	}
	qrimage, contentType, err := r.s.GetLinkImage(rlink.LinkID)
	if err != nil {
		return err
	}
	defer qrimage.Close()
	return c.Stream(http.StatusOK, contentType, qrimage)
}

type (
	reqLinkGetGroup struct {
		Amount int `valid:"amount" json:"-"`
		Offset int `valid:"offset" json:"-"`
	}
)

func (r *router) getLinkGroup(c echo.Context) error {
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

	res, err := r.s.GetLinkGroup(amount, offset, c.QueryParam("creatorid"))
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

func (r *router) createLink(c echo.Context) error {
	rlink := reqLinkPost{}
	if err := c.Bind(&rlink); err != nil {
		return err
	}
	rlink.CreatorID = c.Get("userid").(string)
	if err := rlink.valid(); err != nil {
		return err
	}

	res, err := r.s.CreateLink(rlink.LinkID, rlink.URL, rlink.CreatorID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, res)
}

func (r *router) deleteLink(c echo.Context) error {
	rlink := reqLinkGet{
		LinkID: c.Param("linkid"),
	}
	if err := rlink.valid(); err != nil {
		return err
	}
	if err := r.s.DeleteLink(rlink.LinkID); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (r *router) gateModOrAdmin(c echo.Context) (string, error) {
	return "website", nil
}

func (r *router) getLinkImageCC(c echo.Context) (string, error) {
	rlink := reqLinkGet{
		LinkID: c.Param("linkid"),
	}
	if err := rlink.valid(); err != nil {
		return "", err
	}

	objinfo, err := r.s.StatLinkImage(rlink.LinkID)
	if err != nil {
		return "", err
	}

	return objinfo.ETag, nil
}

func (r *router) mountRoutes(g *echo.Group) error {
	g.GET("/link/:linkid", r.getLink)
	g.GET("/link/:linkid/image", r.getLinkImage, cachecontrol.Control(true, false, min15, r.getLinkImageCC))
	g.GET("/link", r.getLinkGroup, gate.ModOrAdminF(r.s.gate, r.gateModOrAdmin))
	g.POST("/link", r.createLink, gate.ModOrAdminF(r.s.gate, r.gateModOrAdmin))
	g.DELETE("/link/:linkid", r.deleteLink, gate.ModOrAdminF(r.s.gate, r.gateModOrAdmin))
	return nil
}
