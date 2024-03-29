package courier

import (
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/util/rank"
)

type (
	//forge:valid
	reqLinkGet struct {
		LinkID string `valid:"linkID,has" json:"-"`
	}
)

func (s *router) getLink(c *governor.Context) {
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

type (
	//forge:valid
	reqGetGroup struct {
		CreatorID string `valid:"creatorID,has" json:"-"`
		Amount    int    `valid:"amount" json:"-"`
		Offset    int    `valid:"offset" json:"-"`
	}
)

func (s *router) getLinkGroup(c *governor.Context) {
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
	//forge:valid
	reqLinkPost struct {
		CreatorID string `valid:"creatorID,has" json:"-"`
		LinkID    string `valid:"linkID" json:"linkid"`
		URL       string `valid:"URL" json:"url"`
	}
)

func (s *router) createLink(c *governor.Context) {
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

	res, err := s.s.createLink(c.Ctx(), req.CreatorID, req.LinkID, req.URL)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusCreated, res)
}

type (
	//forge:valid
	reqLinkDelete struct {
		CreatorID string `valid:"creatorID,has" json:"-"`
		LinkID    string `valid:"linkID,has" json:"-"`
	}
)

func (s *router) deleteLink(c *governor.Context) {
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

func (s *router) courierOwner(c *governor.Context, userid string) (string, bool, bool) {
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
	m.GetCtx("/link/id/{linkid}", s.getLink, s.rt)
	m.GetCtx("/link/c/{creatorid}", s.getLinkGroup, gate.MemberF(s.s.gate, s.courierOwner, scopeLinkRead), s.rt)
	m.PostCtx("/link/c/{creatorid}", s.createLink, gate.MemberF(s.s.gate, s.courierOwner, scopeLinkWrite), s.rt)
	m.DeleteCtx("/link/c/{creatorid}/id/{linkid}", s.deleteLink, gate.MemberF(s.s.gate, s.courierOwner, scopeLinkWrite), s.rt)
}
