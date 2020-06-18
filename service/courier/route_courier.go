package courier

import (
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

func (m *router) getLink(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqLinkGet{
		LinkID: c.Param("linkid"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	url, err := m.s.GetLinkFast(req.LinkID)
	if err != nil {
		if len(m.s.fallbackLink) > 0 {
			c.Redirect(http.StatusTemporaryRedirect, m.s.fallbackLink)
			return
		}
		c.WriteError(err)
		return
	}
	c.Redirect(http.StatusTemporaryRedirect, url)
}

func (m *router) getLinkImage(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqLinkGet{
		LinkID: c.Param("linkid"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	img, contentType, err := m.s.GetLinkImage(req.LinkID)
	if err != nil {
		c.WriteError(err)
		return
	}
	defer func() {
		if err := img.Close(); err != nil {
			m.s.logger.Error("failed to close link image", map[string]string{
				"actiontype": "getlinkimage",
				"error":      err.Error(),
			})
		}
	}()
	c.WriteFile(http.StatusOK, contentType, img)
}

type (
	reqGetGroup struct {
		Amount int `valid:"amount" json:"-"`
		Offset int `valid:"offset" json:"-"`
	}
)

func (m *router) getLinkGroup(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	amount, err := strconv.Atoi(c.Query("amount"))
	if err != nil {
		c.WriteError(governor.NewErrorUser("Amount invalid", http.StatusBadRequest, err))
		return
	}
	offset, err := strconv.Atoi(c.Query("offset"))
	if err != nil {
		c.WriteError(governor.NewErrorUser("Offset invalid", http.StatusBadRequest, err))
		return
	}

	req := reqGetGroup{
		Amount: amount,
		Offset: offset,
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.GetLinkGroup(amount, offset, c.Query("creatorid"))
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	reqLinkPost struct {
		LinkID    string `valid:"linkID" json:"linkid"`
		URL       string `valid:"URL" json:"url"`
		BrandID   string `valid:"brandID,has" json:"brandid"`
		CreatorID string `valid:"creatorID,has" json:"-"`
	}
)

func (m *router) createLink(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqLinkPost{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	req.CreatorID = c.Get(gate.CtxUserid).(string)
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.CreateLink(req.LinkID, req.URL, req.BrandID, req.CreatorID)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusCreated, res)
}

func (m *router) deleteLink(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqLinkGet{
		LinkID: c.Param("linkid"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := m.s.DeleteLink(req.LinkID); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	reqBrandGet struct {
		BrandID string `valid:"brandID,has" json:"-"`
	}
)

func (m *router) getBrandImage(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqBrandGet{
		BrandID: c.Param("brandid"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	img, contentType, err := m.s.GetBrandImage(req.BrandID)
	if err != nil {
		c.WriteError(err)
		return
	}
	defer func() {
		if err := img.Close(); err != nil {
			m.s.logger.Error("failed to close brand image", map[string]string{
				"actiontype": "getbrandimage",
				"error":      err.Error(),
			})
		}
	}()
	c.WriteFile(http.StatusOK, contentType, img)
}

func (m *router) getBrandGroup(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	amount, err := strconv.Atoi(c.Query("amount"))
	if err != nil {
		c.WriteError(governor.NewErrorUser("Amount invalid", http.StatusBadRequest, err))
		return
	}
	offset, err := strconv.Atoi(c.Query("offset"))
	if err != nil {
		c.WriteError(governor.NewErrorUser("Offset invalid", http.StatusBadRequest, err))
		return
	}

	req := reqGetGroup{
		Amount: amount,
		Offset: offset,
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.GetBrandGroup(req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	reqBrandPost struct {
		BrandID   string `valid:"brandID" json:"-"`
		CreatorID string `valid:"creatorID,has" json:"-"`
	}
)

func (m *router) createBrand(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	img, err := image.LoadImage(m.s.logger, c, "image")
	if err != nil {
		c.WriteError(err)
		return
	}

	req := reqBrandPost{
		BrandID:   c.FormValue("brandid"),
		CreatorID: c.Get(gate.CtxUserid).(string),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.CreateBrand(req.BrandID, img, req.CreatorID)
	if err != nil {
		c.WriteError(err)
		return
	}

	c.WriteJSON(http.StatusCreated, res)
}

func (m *router) deleteBrand(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqBrandGet{
		BrandID: c.Param("brandid"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := m.s.DeleteBrand(req.BrandID); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (r *router) getLinkImageCC(c governor.Context) (string, error) {
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

func (r *router) getBrandImageCC(c governor.Context) (string, error) {
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

func (m *router) mountRoutes(r governor.Router) {
	r.Get("/link/{linkid}", m.getLink)
	r.Get("/link/{linkid}/image", m.getLinkImage, cachecontrol.Control(m.s.logger, true, false, 60, m.getLinkImageCC))
	r.Get("/link", m.getLinkGroup, gate.Member(m.s.gate, "courier"))
	r.Post("/link", m.createLink, gate.Member(m.s.gate, "courier"))
	r.Delete("/link/{linkid}", m.deleteLink, gate.Member(m.s.gate, "courier"))
	r.Get("/brand/{brandid}/image", m.getBrandImage, gate.Member(m.s.gate, "courier"), cachecontrol.Control(m.s.logger, true, false, 60, m.getBrandImageCC))
	r.Get("/brand", m.getBrandGroup, gate.Member(m.s.gate, "courier"))
	r.Post("/brand", m.createBrand, gate.Member(m.s.gate, "courier"))
	r.Delete("/brand/{brandid}", m.deleteBrand, gate.Member(m.s.gate, "courier"))
}
