package courier

import (
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/cachecontrol"
	"xorkevin.dev/governor/service/image"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/util/rank"
	"xorkevin.dev/kerrors"
)

//go:generate forge validation -o validation_courier_gen.go reqLinkGet reqGetGroup reqLinkPost reqLinkDelete reqBrandGet reqBrandPost

type (
	reqLinkGet struct {
		LinkID string `valid:"linkID,has" json:"-"`
	}
)

func (s *router) getLink(c governor.Context) {
	req := reqLinkGet{
		LinkID: c.Param("linkid"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	url, err := s.s.getLinkFast(c.Ctx(), req.LinkID)
	if err != nil {
		if len(s.s.fallbackLink) > 0 {
			c.Redirect(http.StatusTemporaryRedirect, s.s.fallbackLink)
			return
		}
		c.WriteError(err)
		return
	}
	c.Redirect(http.StatusTemporaryRedirect, url)
}

func (s *router) getLinkImage(c governor.Context) {
	req := reqLinkGet{
		LinkID: c.Param("linkid"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	img, contentType, err := s.s.getLinkImage(c.Ctx(), req.LinkID)
	if err != nil {
		c.WriteError(err)
		return
	}
	defer func() {
		if err := img.Close(); err != nil {
			s.s.log.Err(c.Ctx(), kerrors.WithMsg(err, "Failed to close link image"), nil)
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

func (s *router) getLinkGroup(c governor.Context) {
	req := reqGetGroup{
		CreatorID: c.Param("creatorid"),
		Amount:    c.QueryInt("amount", -1),
		Offset:    c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := s.s.getLinkGroup(c.Ctx(), req.CreatorID, req.Amount, req.Offset)
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

func (s *router) createLink(c governor.Context) {
	var req reqLinkPost
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	req.CreatorID = c.Param("creatorid")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := s.s.createLink(c.Ctx(), req.CreatorID, req.LinkID, req.URL, req.BrandID)
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

func (s *router) deleteLink(c governor.Context) {
	req := reqLinkDelete{
		LinkID:    c.Param("linkid"),
		CreatorID: c.Param("creatorid"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := s.s.deleteLink(c.Ctx(), req.CreatorID, req.LinkID); err != nil {
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

func (s *router) getBrandImage(c governor.Context) {
	req := reqBrandGet{
		CreatorID: c.Param("creatorid"),
		BrandID:   c.Param("brandid"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	img, contentType, err := s.s.getBrandImage(c.Ctx(), req.CreatorID, req.BrandID)
	if err != nil {
		c.WriteError(err)
		return
	}
	defer func() {
		if err := img.Close(); err != nil {
			s.s.log.Err(c.Ctx(), kerrors.WithMsg(err, "Failed to close brand image"), nil)
		}
	}()
	c.WriteFile(http.StatusOK, contentType, img)
}

func (s *router) getBrandGroup(c governor.Context) {
	req := reqGetGroup{
		CreatorID: c.Param("creatorid"),
		Amount:    c.QueryInt("amount", -1),
		Offset:    c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := s.s.getBrandGroup(c.Ctx(), req.CreatorID, req.Amount, req.Offset)
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

func (s *router) createBrand(c governor.Context) {
	img, err := image.LoadImage(c, "image")
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

	res, err := s.s.createBrand(c.Ctx(), req.CreatorID, req.BrandID, img)
	if err != nil {
		c.WriteError(err)
		return
	}

	c.WriteJSON(http.StatusCreated, res)
}

func (s *router) deleteBrand(c governor.Context) {
	req := reqBrandGet{
		CreatorID: c.Param("creatorid"),
		BrandID:   c.Param("brandid"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := s.s.deleteBrand(c.Ctx(), req.CreatorID, req.BrandID); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (s *router) getLinkImageCC(c governor.Context) (string, error) {
	req := reqLinkGet{
		LinkID: c.Param("linkid"),
	}
	if err := req.valid(); err != nil {
		return "", err
	}

	objinfo, err := s.s.statLinkImage(c.Ctx(), req.LinkID)
	if err != nil {
		return "", err
	}

	return objinfo.ETag, nil
}

func (s *router) getBrandImageCC(c governor.Context) (string, error) {
	req := reqBrandGet{
		CreatorID: c.Param("creatorid"),
		BrandID:   c.Param("brandid"),
	}
	if err := req.valid(); err != nil {
		return "", err
	}

	objinfo, err := s.s.statBrandImage(c.Ctx(), req.CreatorID, req.BrandID)
	if err != nil {
		return "", err
	}

	return objinfo.ETag, nil
}

func (s *router) courierOwner(c governor.Context, userid string) (string, bool, bool) {
	creatorid := c.Param("creatorid")
	if err := validhasCreatorID(creatorid); err != nil {
		return "", false, false
	}
	if creatorid == userid {
		return "", true, true
	}
	if !rank.IsValidOrgName(creatorid) {
		return "", false, false
	}
	return creatorid, false, true
}

func (s *router) mountRoutes(r governor.Router) {
	m := governor.NewMethodRouter(r)
	scopeLinkRead := s.s.scopens + ".link:read"
	scopeLinkWrite := s.s.scopens + ".link:write"
	scopeBrandRead := s.s.scopens + ".brand:read"
	scopeBrandWrite := s.s.scopens + ".brand:write"
	m.GetCtx("/link/id/{linkid}", s.getLink, s.rt)
	m.GetCtx("/link/id/{linkid}/image", s.getLinkImage, cachecontrol.ControlCtx(true, nil, 60, s.getLinkImageCC), s.rt)
	m.GetCtx("/link/c/{creatorid}", s.getLinkGroup, gate.MemberF(s.s.gate, s.courierOwner, scopeLinkRead), s.rt)
	m.PostCtx("/link/c/{creatorid}", s.createLink, gate.MemberF(s.s.gate, s.courierOwner, scopeLinkWrite), s.rt)
	m.DeleteCtx("/link/c/{creatorid}/id/{linkid}", s.deleteLink, gate.MemberF(s.s.gate, s.courierOwner, scopeLinkWrite), s.rt)
	m.GetCtx("/brand/c/{creatorid}/id/{brandid}/image", s.getBrandImage, gate.MemberF(s.s.gate, s.courierOwner, scopeBrandRead), cachecontrol.ControlCtx(true, nil, 60, s.getBrandImageCC), s.rt)
	m.GetCtx("/brand/c/{creatorid}", s.getBrandGroup, gate.MemberF(s.s.gate, s.courierOwner, scopeBrandRead), s.rt)
	m.PostCtx("/brand/c/{creatorid}", s.createBrand, gate.MemberF(s.s.gate, s.courierOwner, scopeBrandWrite), s.rt)
	m.DeleteCtx("/brand/c/{creatorid}/id/{brandid}", s.deleteBrand, gate.MemberF(s.s.gate, s.courierOwner, scopeBrandWrite), s.rt)
}
