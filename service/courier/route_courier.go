package courier

import (
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/cachecontrol"
	"xorkevin.dev/governor/service/image"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/util/rank"
)

//go:generate forge validation -o validation_courier_gen.go reqLinkGet reqGetGroup reqLinkPost reqLinkDelete reqBrandGet reqBrandPost

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
		CreatorID string `valid:"creatorID,has" json:"-"`
		Amount    int    `valid:"amount" json:"-"`
		Offset    int    `valid:"offset" json:"-"`
	}
)

func (m *router) getLinkGroup(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqGetGroup{
		CreatorID: c.Param("creatorid"),
		Amount:    c.QueryInt("amount", -1),
		Offset:    c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.GetLinkGroup(req.CreatorID, req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	reqLinkPost struct {
		CreatorID string `valid:"creatorID,has" json:"-"`
		LinkID    string `valid:"linkID" json:"linkid"`
		URL       string `valid:"URL" json:"url"`
		BrandID   string `valid:"brandID,has" json:"brandid"`
	}
)

func (m *router) createLink(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqLinkPost{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	req.CreatorID = c.Param("creatorid")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.CreateLink(req.CreatorID, req.LinkID, req.URL, req.BrandID)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusCreated, res)
}

type (
	reqLinkDelete struct {
		CreatorID string `valid:"creatorID,has" json:"-"`
		LinkID    string `valid:"linkID,has" json:"-"`
	}
)

func (m *router) deleteLink(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqLinkDelete{
		LinkID:    c.Param("linkid"),
		CreatorID: c.Param("creatorid"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := m.s.DeleteLink(req.CreatorID, req.LinkID); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	reqBrandGet struct {
		CreatorID string `valid:"creatorID,has" json:"-"`
		BrandID   string `valid:"brandID,has" json:"-"`
	}
)

func (m *router) getBrandImage(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqBrandGet{
		CreatorID: c.Param("creatorid"),
		BrandID:   c.Param("brandid"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	img, contentType, err := m.s.GetBrandImage(req.CreatorID, req.BrandID)
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
	req := reqGetGroup{
		CreatorID: c.Param("creatorid"),
		Amount:    c.QueryInt("amount", -1),
		Offset:    c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.GetBrandGroup(req.CreatorID, req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	reqBrandPost struct {
		CreatorID string `valid:"creatorID,has" json:"-"`
		BrandID   string `valid:"brandID" json:"-"`
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
		CreatorID: c.Param("creatorid"),
		BrandID:   c.FormValue("brandid"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.CreateBrand(req.CreatorID, req.BrandID, img)
	if err != nil {
		c.WriteError(err)
		return
	}

	c.WriteJSON(http.StatusCreated, res)
}

func (m *router) deleteBrand(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqBrandGet{
		CreatorID: c.Param("creatorid"),
		BrandID:   c.Param("brandid"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := m.s.DeleteBrand(req.CreatorID, req.BrandID); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (m *router) getLinkImageCC(c governor.Context) (string, error) {
	req := reqLinkGet{
		LinkID: c.Param("linkid"),
	}
	if err := req.valid(); err != nil {
		return "", err
	}

	objinfo, err := m.s.StatLinkImage(req.LinkID)
	if err != nil {
		return "", err
	}

	return objinfo.ETag, nil
}

func (m *router) getBrandImageCC(c governor.Context) (string, error) {
	req := reqBrandGet{
		CreatorID: c.Param("creatorid"),
		BrandID:   c.Param("brandid"),
	}
	if err := req.valid(); err != nil {
		return "", err
	}

	objinfo, err := m.s.StatBrandImage(req.CreatorID, req.BrandID)
	if err != nil {
		return "", err
	}

	return objinfo.ETag, nil
}

func (m *router) courierOwner(c governor.Context, userid string) (string, error) {
	creatorid := c.Param("creatorid")
	if err := validhasCreatorID(creatorid); err != nil {
		return "", err
	}
	if creatorid == userid {
		return "", nil
	}
	if !rank.IsValidOrgName(creatorid) {
		return "", governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Invalid org id",
			Status:  http.StatusBadRequest,
		}))
	}
	return creatorid, nil
}

const (
	scopeLinkRead   = "gov.courier.link:read"
	scopeLinkWrite  = "gov.courier.link:write"
	scopeBrandRead  = "gov.courier.brand:read"
	scopeBrandWrite = "gov.courier.brand:write"
)

func (m *router) mountRoutes(r governor.Router) {
	r.Get("/link/id/{linkid}", m.getLink)
	r.Get("/link/id/{linkid}/image", m.getLinkImage, cachecontrol.Control(m.s.logger, true, false, 60, m.getLinkImageCC))
	r.Get("/link/c/{creatorid}", m.getLinkGroup, gate.MemberF(m.s.gate, m.courierOwner, scopeLinkRead))
	r.Post("/link/c/{creatorid}", m.createLink, gate.MemberF(m.s.gate, m.courierOwner, scopeLinkWrite))
	r.Delete("/link/c/{creatorid}/id/{linkid}", m.deleteLink, gate.MemberF(m.s.gate, m.courierOwner, scopeLinkWrite))
	r.Get("/brand/c/{creatorid}/id/{brandid}/image", m.getBrandImage, gate.MemberF(m.s.gate, m.courierOwner, scopeBrandRead), cachecontrol.Control(m.s.logger, true, false, 60, m.getBrandImageCC))
	r.Get("/brand/c/{creatorid}", m.getBrandGroup, gate.MemberF(m.s.gate, m.courierOwner, scopeBrandRead))
	r.Post("/brand/c/{creatorid}", m.createBrand, gate.MemberF(m.s.gate, m.courierOwner, scopeBrandWrite))
	r.Delete("/brand/c/{creatorid}/id/{brandid}", m.deleteBrand, gate.MemberF(m.s.gate, m.courierOwner, scopeBrandWrite))
}
