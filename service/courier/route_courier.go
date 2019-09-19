package courier

import (
	"github.com/labstack/echo/v4"
	"net/http"
	"strconv"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/cachecontrol"
	"xorkevin.dev/governor/service/image"
	"xorkevin.dev/governor/service/user/gate"
)

//go:generate forge validation -o validation_courier_gen.go reqLinkGet reqGetGroup reqLinkPost reqBrandGet reqBrandPost

type (
	reqLinkGet struct {
		LinkID string `valid:"linkID,has" json:"-"`
	}
)

func (r *router) getLink(c echo.Context) error {
	req := reqLinkGet{
		LinkID: c.Param("linkid"),
	}
	if err := req.valid(); err != nil {
		return err
	}
	url, err := r.s.GetLinkFast(req.LinkID)
	if err != nil {
		if len(r.s.fallbackLink) > 0 {
			return c.Redirect(http.StatusMovedPermanently, r.s.fallbackLink)
		}
		return err
	}
	return c.Redirect(http.StatusTemporaryRedirect, url)
}

func (r *router) getLinkImage(c echo.Context) error {
	req := reqLinkGet{
		LinkID: c.Param("linkid"),
	}
	if err := req.valid(); err != nil {
		return err
	}
	img, contentType, err := r.s.GetLinkImage(req.LinkID)
	if err != nil {
		return err
	}
	defer func() {
		if err := img.Close(); err != nil {
			r.s.logger.Error("Failed to close link image", map[string]string{
				"actiontype": "getlinkimage",
				"error":      err.Error(),
			})
		}
	}()
	return c.Stream(http.StatusOK, contentType, img)
}

type (
	reqGetGroup struct {
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

	req := reqGetGroup{
		Amount: amount,
		Offset: offset,
	}
	if err := req.valid(); err != nil {
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
		LinkID    string `valid:"linkID" json:"linkid"`
		URL       string `valid:"URL" json:"url"`
		BrandID   string `valid:"brandID,has" json:"brandid"`
		CreatorID string `valid:"creatorID,has" json:"-"`
	}
)

func (r *router) createLink(c echo.Context) error {
	req := reqLinkPost{}
	if err := c.Bind(&req); err != nil {
		return err
	}
	req.CreatorID = c.Get("userid").(string)
	if err := req.valid(); err != nil {
		return err
	}

	res, err := r.s.CreateLink(req.LinkID, req.URL, req.BrandID, req.CreatorID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, res)
}

func (r *router) deleteLink(c echo.Context) error {
	req := reqLinkGet{
		LinkID: c.Param("linkid"),
	}
	if err := req.valid(); err != nil {
		return err
	}
	if err := r.s.DeleteLink(req.LinkID); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

type (
	reqBrandGet struct {
		BrandID string `valid:"brandID,has" json:"-"`
	}
)

func (r *router) getBrandImage(c echo.Context) error {
	req := reqBrandGet{
		BrandID: c.Param("brandid"),
	}
	if err := req.valid(); err != nil {
		return err
	}
	img, contentType, err := r.s.GetBrandImage(req.BrandID)
	if err != nil {
		return err
	}
	defer func() {
		if err := img.Close(); err != nil {
			r.s.logger.Error("Failed to close brand image", map[string]string{
				"actiontype": "getbrandimage",
				"error":      err.Error(),
			})
		}
	}()
	return c.Stream(http.StatusOK, contentType, img)
}

func (r *router) getBrandGroup(c echo.Context) error {
	amount, err := strconv.Atoi(c.QueryParam("amount"))
	if err != nil {
		return governor.NewErrorUser("Amount invalid", http.StatusBadRequest, err)
	}
	offset, err := strconv.Atoi(c.QueryParam("offset"))
	if err != nil {
		return governor.NewErrorUser("Offset invalid", http.StatusBadRequest, err)
	}

	req := reqGetGroup{
		Amount: amount,
		Offset: offset,
	}
	if err := req.valid(); err != nil {
		return err
	}

	res, err := r.s.GetBrandGroup(req.Amount, req.Offset)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, res)
}

type (
	reqBrandPost struct {
		BrandID   string `valid:"brandID" json:"-"`
		CreatorID string `valid:"creatorID,has" json:"-"`
	}
)

func (r *router) createBrand(c echo.Context) error {
	img, err := image.LoadImage(c, "image")
	if err != nil {
		return err
	}

	req := reqBrandPost{
		BrandID:   c.FormValue("brandid"),
		CreatorID: c.Get("userid").(string),
	}
	if err := req.valid(); err != nil {
		return err
	}

	res, err := r.s.CreateBrand(req.BrandID, img, req.CreatorID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, res)
}

func (r *router) deleteBrand(c echo.Context) error {
	req := reqBrandGet{
		BrandID: c.Param("brandid"),
	}
	if err := req.valid(); err != nil {
		return err
	}
	if err := r.s.DeleteBrand(req.BrandID); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (r *router) gateModOrAdmin(c echo.Context) (string, error) {
	return "website", nil
}

func (r *router) getLinkImageCC(c echo.Context) (string, error) {
	req := reqLinkGet{
		LinkID: c.Param("linkid"),
	}
	if err := req.valid(); err != nil {
		return "", err
	}

	objinfo, err := r.s.StatLinkImage(req.LinkID)
	if err != nil {
		return "", err
	}

	return objinfo.ETag, nil
}

func (r *router) getBrandImageCC(c echo.Context) (string, error) {
	req := reqBrandGet{
		BrandID: c.Param("brandid"),
	}
	if err := req.valid(); err != nil {
		return "", err
	}

	objinfo, err := r.s.StatBrandImage(req.BrandID)
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
	g.GET("/brand/:brandid/image", r.getBrandImage, gate.ModOrAdminF(r.s.gate, r.gateModOrAdmin), cachecontrol.Control(true, false, min15, r.getBrandImageCC))
	g.GET("/brand", r.getBrandGroup, gate.ModOrAdminF(r.s.gate, r.gateModOrAdmin))
	g.POST("/brand", r.createBrand, gate.ModOrAdminF(r.s.gate, r.gateModOrAdmin))
	g.DELETE("/brand/:brandid", r.deleteBrand, gate.ModOrAdminF(r.s.gate, r.gateModOrAdmin))
	return nil
}
