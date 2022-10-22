package oauth

import (
	"net/http"
	"strings"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/cachecontrol"
	"xorkevin.dev/governor/service/image"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/kerrors"
)

type (
	//forge:valid
	reqAppGet struct {
		ClientID string `valid:"clientID,has" json:"-"`
	}
)

func (s *router) getApp(c governor.Context) {
	req := reqAppGet{
		ClientID: c.Param("clientid"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.getApp(c.Ctx(), req.ClientID)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (s *router) getAppLogo(c governor.Context) {
	req := reqAppGet{
		ClientID: c.Param("clientid"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	img, contentType, err := s.s.getLogo(c.Ctx(), req.ClientID)
	if err != nil {
		c.WriteError(err)
		return
	}
	defer func() {
		if err := img.Close(); err != nil {
			s.s.log.Err(c.Ctx(), kerrors.WithMsg(err, "Failed to close app logo"), nil)
		}
	}()
	c.WriteFile(http.StatusOK, contentType, img)
}

type (
	//forge:valid
	reqGetAppGroup struct {
		CreatorID string `valid:"userid,opt" json:"-"`
		Amount    int    `valid:"amount" json:"-"`
		Offset    int    `valid:"offset" json:"-"`
	}
)

func (s *router) getAppGroup(c governor.Context) {
	req := reqGetAppGroup{
		CreatorID: c.Query("creatorid"),
		Amount:    c.QueryInt("amount", -1),
		Offset:    c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := s.s.getApps(c.Ctx(), req.Amount, req.Offset, req.CreatorID)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqGetAppBulk struct {
		ClientIDs []string `valid:"clientIDs,has" json:"-"`
	}
)

func (s *router) getAppBulk(c governor.Context) {
	req := reqGetAppBulk{
		ClientIDs: strings.Split(c.Query("ids"), ","),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := s.s.getAppsBulk(c.Ctx(), req.ClientIDs)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqAppPost struct {
		Name        string `valid:"name" json:"name"`
		URL         string `valid:"URL" json:"url"`
		RedirectURI string `valid:"redirect" json:"redirect_uri"`
		CreatorID   string `valid:"userid,has" json:"-"`
	}
)

func (s *router) createApp(c governor.Context) {
	var req reqAppPost
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	req.CreatorID = gate.GetCtxUserid(c)
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := s.s.createApp(c.Ctx(), req.Name, req.URL, req.RedirectURI, req.CreatorID)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusCreated, res)
}

type (
	//forge:valid
	reqAppPut struct {
		ClientID    string `valid:"clientID,has" json:"-"`
		Name        string `valid:"name" json:"name"`
		URL         string `valid:"URL" json:"url"`
		RedirectURI string `valid:"redirect" json:"redirect_uri"`
	}
)

func (s *router) updateApp(c governor.Context) {
	var req reqAppPut
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	req.ClientID = c.Param("clientid")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := s.s.updateApp(c.Ctx(), req.ClientID, req.Name, req.URL, req.RedirectURI); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (s *router) updateAppLogo(c governor.Context) {
	img, err := image.LoadImage(c, "image")
	if err != nil {
		c.WriteError(err)
		return
	}

	req := reqAppGet{
		ClientID: c.Param("clientid"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := s.s.updateLogo(c.Ctx(), req.ClientID, img); err != nil {
		c.WriteError(err)
		return
	}

	c.WriteStatus(http.StatusNoContent)
}

func (s *router) rotateAppKey(c governor.Context) {
	req := reqAppGet{
		ClientID: c.Param("clientid"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.rotateAppKey(c.Ctx(), req.ClientID)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (s *router) deleteApp(c governor.Context) {
	req := reqAppGet{
		ClientID: c.Param("clientid"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := s.s.deleteApp(c.Ctx(), req.ClientID); err != nil {
		c.WriteError(err)
		return
	}

	c.WriteStatus(http.StatusNoContent)
}

func (s *router) getAppLogoCC(c governor.Context) (string, error) {
	req := reqAppGet{
		ClientID: c.Param("clientid"),
	}
	if err := req.valid(); err != nil {
		return "", err
	}

	objinfo, err := s.s.statLogo(c.Ctx(), req.ClientID)
	if err != nil {
		return "", err
	}

	return objinfo.ETag, nil
}

func (s *router) mountAppRoutes(r governor.Router) {
	m := governor.NewMethodRouter(r)
	scopeAppRead := s.s.scopens + ".app:read"
	scopeAppWrite := s.s.scopens + ".app:write"
	m.GetCtx("/id/{clientid}", s.getApp, s.rt)
	m.GetCtx("/id/{clientid}/image", s.getAppLogo, cachecontrol.ControlCtx(true, nil, 60, s.getAppLogoCC), s.rt)
	m.GetCtx("", s.getAppGroup, gate.Member(s.s.gate, s.s.rolens, scopeAppRead), s.rt)
	m.GetCtx("/ids", s.getAppBulk, s.rt)
	m.PostCtx("", s.createApp, gate.Member(s.s.gate, s.s.rolens, scopeAppWrite), s.rt)
	m.PutCtx("/id/{clientid}", s.updateApp, gate.Member(s.s.gate, s.s.rolens, scopeAppWrite), s.rt)
	m.PutCtx("/id/{clientid}/image", s.updateAppLogo, gate.Member(s.s.gate, s.s.rolens, scopeAppWrite), s.rt)
	m.PutCtx("/id/{clientid}/rotate", s.rotateAppKey, gate.Member(s.s.gate, s.s.rolens, scopeAppWrite), s.rt)
	m.DeleteCtx("/id/{clientid}", s.deleteApp, gate.Member(s.s.gate, s.s.rolens, scopeAppWrite), s.rt)
}
