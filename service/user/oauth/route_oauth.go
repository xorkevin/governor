package oauth

import (
	"net/http"
	"strconv"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/cachecontrol"
	"xorkevin.dev/governor/service/user/gate"
)

//go:generate forge validation -o validation_oauth_gen.go reqAppGet reqGetGroup reqAppPost

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
	reqGetGroup struct {
		Amount int `valid:"amount" json:"-"`
		Offset int `valid:"offset" json:"-"`
	}
)

func (m *router) getAppGroup(w http.ResponseWriter, r *http.Request) {
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

	res, err := m.s.GetApps(amount, offset, c.Query("creatorid"))
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
		CreatorID   string `valid:"creatorID,has" json:"-"`
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

func (r *router) getAppLogoCC(c governor.Context) (string, error) {
	req := reqAppGet{
		ClientID: c.Param("clientid"),
	}
	if err := req.valid(); err != nil {
		return "", err
	}

	objinfo, err := r.s.StatLogo(req.ClientID)
	if err != nil {
		return "", err
	}

	return objinfo.ETag, nil
}

const (
	scopeAppRead  = "gov.user.oauth.app:read"
	scopeAppWrite = "gov.user.oauth.app:write"
)

func (m *router) mountRoutes(r governor.Router) {
	r.Get("/app/{clientid}", m.getApp)
	r.Get("/app/{clientid}/image", m.getAppLogo, cachecontrol.Control(m.s.logger, true, false, 60, m.getAppLogoCC))
	r.Get("/app", m.getAppGroup, gate.Member(m.s.gate, "oauth", scopeAppRead))
	r.Post("/app", m.getAppGroup, gate.Member(m.s.gate, "oauth", scopeAppWrite))
}
