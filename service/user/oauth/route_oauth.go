package oauth

import (
	"net/http"
	"strings"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/cachecontrol"
	"xorkevin.dev/governor/service/image"
	"xorkevin.dev/governor/service/user/gate"
)

//go:generate forge validation -o validation_oauth_gen.go reqAppGet reqGetAppGroup reqGetAppBulk reqAppPost reqAppPut

type (
	reqAppGet struct {
		ClientID string `valid:"clientID,has" json:"-"`
	}
)

func (m *router) getApp(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqAppGet{
		ClientID: c.Param("clientid"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := m.s.GetApp(req.ClientID)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (m *router) getAppLogo(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqAppGet{
		ClientID: c.Param("clientid"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	img, contentType, err := m.s.GetLogo(req.ClientID)
	if err != nil {
		c.WriteError(err)
		return
	}
	defer func() {
		if err := img.Close(); err != nil {
			m.s.logger.Error("failed to close app logo", map[string]string{
				"actiontype": "getapplogo",
				"error":      err.Error(),
			})
		}
	}()
	c.WriteFile(http.StatusOK, contentType, img)
}

type (
	reqGetAppGroup struct {
		CreatorID string `valid:"userid,opt" json:"-"`
		Amount    int    `valid:"amount" json:"-"`
		Offset    int    `valid:"offset" json:"-"`
	}
)

func (m *router) getAppGroup(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqGetAppGroup{
		CreatorID: c.Query("creatorid"),
		Amount:    c.QueryInt("amount", -1),
		Offset:    c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.GetApps(req.Amount, req.Offset, req.CreatorID)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	reqGetAppBulk struct {
		ClientIDs []string `valid:"clientIDs,has" json:"-"`
	}
)

func (m *router) getAppBulk(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqGetAppBulk{
		ClientIDs: strings.Split(c.Query("ids"), ","),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.GetAppsBulk(req.ClientIDs)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	reqAppPost struct {
		Name        string `valid:"name" json:"name"`
		URL         string `valid:"URL" json:"url"`
		RedirectURI string `valid:"redirect" json:"redirect_uri"`
		CreatorID   string `valid:"userid,has" json:"-"`
	}
)

func (m *router) createApp(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqAppPost{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	req.CreatorID = gate.GetCtxUserid(c)
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.CreateApp(req.Name, req.URL, req.RedirectURI, req.CreatorID)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusCreated, res)
}

type (
	reqAppPut struct {
		ClientID    string `valid:"clientID,has" json:"-"`
		Name        string `valid:"name" json:"name"`
		URL         string `valid:"URL" json:"url"`
		RedirectURI string `valid:"redirect" json:"redirect_uri"`
	}
)

func (m *router) updateApp(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqAppPut{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	req.ClientID = c.Param("clientid")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := m.s.UpdateApp(req.ClientID, req.Name, req.URL, req.RedirectURI); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (m *router) updateAppLogo(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	img, err := image.LoadImage(m.s.logger, c, "image")
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

	if err := m.s.UpdateLogo(req.ClientID, img); err != nil {
		c.WriteError(err)
		return
	}

	c.WriteStatus(http.StatusNoContent)
}

func (m *router) rotateAppKey(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqAppGet{
		ClientID: c.Param("clientid"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := m.s.RotateAppKey(req.ClientID)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (m *router) deleteApp(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqAppGet{
		ClientID: c.Param("clientid"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := m.s.Delete(req.ClientID); err != nil {
		c.WriteError(err)
		return
	}

	c.WriteStatus(http.StatusNoContent)
}

func (m *router) getAppLogoCC(c governor.Context) (string, error) {
	req := reqAppGet{
		ClientID: c.Param("clientid"),
	}
	if err := req.valid(); err != nil {
		return "", err
	}

	objinfo, err := m.s.StatLogo(req.ClientID)
	if err != nil {
		return "", err
	}

	return objinfo.ETag, nil
}

const (
	scopeAppRead  = "gov.user.oauth.app:read"
	scopeAppWrite = "gov.user.oauth.app:write"
)

func (m *router) mountAppRoutes(r governor.Router) {
	r.Get("/id/{clientid}", m.getApp)
	r.Get("/id/{clientid}/image", m.getAppLogo, cachecontrol.Control(m.s.logger, true, nil, 60, m.getAppLogoCC))
	r.Get("", m.getAppGroup, gate.Member(m.s.gate, "gov.oauth", scopeAppRead))
	r.Get("/ids", m.getAppBulk)
	r.Post("", m.createApp, gate.Member(m.s.gate, "gov.oauth", scopeAppWrite))
	r.Put("/id/{clientid}", m.updateApp, gate.Member(m.s.gate, "gov.oauth", scopeAppWrite))
	r.Put("/id/{clientid}/image", m.updateAppLogo, gate.Member(m.s.gate, "gov.oauth", scopeAppWrite))
	r.Put("/id/{clientid}/rotate", m.rotateAppKey, gate.Member(m.s.gate, "gov.oauth", scopeAppWrite))
	r.Delete("/id/{clientid}", m.deleteApp, gate.Member(m.s.gate, "gov.oauth", scopeAppWrite))
}
