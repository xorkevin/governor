package user

import (
	"net/http"
	"strconv"
	"strings"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/util/rank"
)

//go:generate forge validation -o validation_apikey_gen.go reqGetUserApikeys reqApikeyPost reqApikeyID reqApikeyUpdate reqApikeyCheck

type (
	reqGetUserApikeys struct {
		Userid string `valid:"userid,has" json:"-"`
		Amount int    `valid:"amount" json:"-"`
		Offset int    `valid:"offset" json:"-"`
	}
)

func (m *router) getUserApikeys(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	query := c.Query()
	amount, err := strconv.Atoi(query.Get("amount"))
	if err != nil {
		c.WriteError(governor.NewErrorUser("amount invalid", http.StatusBadRequest, nil))
		return
	}
	offset, err := strconv.Atoi(query.Get("offset"))
	if err != nil {
		c.WriteError(governor.NewErrorUser("offset invalid", http.StatusBadRequest, nil))
		return
	}
	req := reqGetUserApikeys{
		Userid: c.Get(gate.CtxUserid).(string),
		Amount: amount,
		Offset: offset,
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := m.s.GetUserApikeys(req.Userid, req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	reqApikeyPost struct {
		Userid   string `valid:"userid,has" json:"-"`
		AuthTags string `valid:"rank" json:"auth_tags"`
		Name     string `valid:"apikeyName" json:"name"`
		Desc     string `valid:"apikeyDesc" json:"desc"`
	}
)

func (m *router) createApikey(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqApikeyPost{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = c.Get(gate.CtxUserid).(string)
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	authTags, _ := rank.FromStringUser(req.AuthTags)
	res, err := m.s.CreateApikey(req.Userid, authTags, req.Name, req.Desc)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusCreated, res)
}

type (
	reqApikeyID struct {
		Userid string `valid:"userid,has" json:"-"`
		Keyid  string `valid:"apikeyid,has" json:"-"`
	}
)

func (r *reqApikeyID) validUserid() error {
	k := strings.SplitN(r.Keyid, "|", 2)
	if len(k) != 2 || r.Userid != k[0] {
		return governor.NewErrorUser("Invalid apikey id", http.StatusForbidden, nil)
	}
	return nil
}

func (m *router) deleteApikey(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqApikeyID{
		Userid: c.Get(gate.CtxUserid).(string),
		Keyid:  c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := req.validUserid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := m.s.DeleteApikey(req.Keyid); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	reqApikeyUpdate struct {
		Userid   string `valid:"userid,has" json:"-"`
		Keyid    string `valid:"apikeyid,has" json:"-"`
		AuthTags string `valid:"rank" json:"auth_tags"`
		Name     string `valid:"apikeyName" json:"name"`
		Desc     string `valid:"apikeyDesc" json:"desc"`
	}
)

func (r *reqApikeyUpdate) validUserid() error {
	k := strings.SplitN(r.Keyid, "|", 2)
	if len(k) != 2 || r.Userid != k[0] {
		return governor.NewErrorUser("Invalid apikey id", http.StatusForbidden, nil)
	}
	return nil
}

func (m *router) updateApikey(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqApikeyUpdate{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = c.Get(gate.CtxUserid).(string)
	req.Keyid = c.Param("id")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := req.validUserid(); err != nil {
		c.WriteError(err)
		return
	}
	authTags, _ := rank.FromStringUser(req.AuthTags)
	if err := m.s.UpdateApikey(req.Keyid, authTags, req.Name, req.Desc); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (m *router) rotateApikey(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqApikeyID{
		Userid: c.Get(gate.CtxUserid).(string),
		Keyid:  c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := req.validUserid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := m.s.RotateApikey(req.Keyid)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	reqApikeyCheck struct {
		AuthTags string `valid:"rank"`
	}
)

const (
	basicAuthRealm = "governor"
)

func (r *router) checkApikeyValidator(t gate.Intersector) bool {
	c := t.Ctx()
	query := c.Query()
	req := reqApikeyCheck{
		AuthTags: query.Get("authtags"),
	}
	if err := req.valid(); err != nil {
		return false
	}
	authTags, _ := rank.FromStringUser(req.AuthTags)

	roles, ok := t.Intersect(authTags)
	if !ok {
		return false
	}
	if roles.Len() != authTags.Len() {
		return false
	}
	return true
}

type (
	resApikeyOK struct {
		Message string `json:"message"`
	}
)

func (m *router) checkApikey(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	c.WriteJSON(http.StatusOK, resApikeyOK{
		Message: "OK",
	})
}

func (m *router) mountApikey(r governor.Router) {
	r.Get("", m.getUserApikeys, gate.User(m.s.gate))
	r.Post("", m.createApikey, gate.User(m.s.gate))
	r.Put("/id/{id}", m.updateApikey, gate.User(m.s.gate))
	r.Put("/id/{id}/rotate", m.rotateApikey, gate.User(m.s.gate))
	r.Delete("/id/{id}", m.deleteApikey, gate.User(m.s.gate))
	r.Any("/check", m.checkApikey, m.s.gate.WithApikey().Authenticate(m.checkApikeyValidator, "authentication"))
}
