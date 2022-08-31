package profile

import (
	"net/http"
	"strings"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/cachecontrol"
	"xorkevin.dev/governor/service/image"
	"xorkevin.dev/governor/service/user/gate"
)

//go:generate forge validation -o validation_profile_gen.go reqProfileGetID reqProfileModel reqGetProfiles

type (
	reqProfileModel struct {
		Userid string `valid:"userid,has" json:"-"`
		Email  string `valid:"email" json:"contact_email"`
		Bio    string `valid:"bio" json:"bio"`
	}
)

func (m *router) createProfile(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqProfileModel{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.CreateProfile(c.Ctx(), req.Userid, req.Email, req.Bio)
	if err != nil {
		c.WriteError(err)
		return
	}

	c.WriteJSON(http.StatusCreated, res)
}

func (m *router) updateProfile(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqProfileModel{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := m.s.UpdateProfile(c.Ctx(), req.Userid, req.Email, req.Bio); err != nil {
		c.WriteError(err)
		return
	}

	c.WriteStatus(http.StatusNoContent)
}

type (
	reqProfileGetID struct {
		Userid string `valid:"userid,has" json:"userid"`
	}
)

func (m *router) updateImage(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	img, err := image.LoadImage(m.s.logger, c, "image")
	if err != nil {
		c.WriteError(err)
		return
	}

	req := reqProfileGetID{
		Userid: gate.GetCtxUserid(c),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := m.s.UpdateImage(c.Ctx(), req.Userid, img); err != nil {
		c.WriteError(err)
		return
	}

	c.WriteStatus(http.StatusNoContent)
}

func (m *router) deleteProfile(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqProfileGetID{
		Userid: c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := m.s.DeleteProfile(c.Ctx(), req.Userid); err != nil {
		c.WriteError(err)
		return
	}

	c.WriteStatus(http.StatusNoContent)
}

func (m *router) getOwnProfile(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqProfileGetID{
		Userid: gate.GetCtxUserid(c),
	}
	res, err := m.s.GetProfile(c.Ctx(), req.Userid)
	if err != nil {
		c.WriteError(err)
		return
	}

	c.WriteJSON(http.StatusOK, res)
}

func (m *router) getProfile(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqProfileGetID{
		Userid: c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.GetProfile(c.Ctx(), req.Userid)
	if err != nil {
		c.WriteError(err)
		return
	}

	c.WriteJSON(http.StatusOK, res)
}

func (m *router) getProfileImage(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqProfileGetID{
		Userid: c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	image, contentType, err := m.s.GetProfileImage(c.Ctx(), req.Userid)
	if err != nil {
		c.WriteError(err)
		return
	}
	defer func() {
		if err := image.Close(); err != nil {
			m.s.logger.Error("Failed to close profile image", map[string]string{
				"actiontype": "profile_get_image",
				"error":      err.Error(),
			})
		}
	}()
	c.WriteFile(http.StatusOK, contentType, image)
}

type (
	reqGetProfiles struct {
		Userids []string `valid:"userids,has" json:"-"`
	}
)

func (m *router) getProfilesBulk(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqGetProfiles{
		Userids: strings.Split(c.Query("ids"), ","),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.GetProfilesBulk(c.Ctx(), req.Userids)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (m *router) getProfileImageCC(c governor.Context) (string, error) {
	req := reqProfileGetID{
		Userid: c.Param("id"),
	}
	if err := req.valid(); err != nil {
		return "", err
	}

	objinfo, err := m.s.StatProfileImage(c.Ctx(), req.Userid)
	if err != nil {
		return "", err
	}

	return objinfo.ETag, nil
}

func (m *router) mountProfileRoutes(r governor.Router) {
	scopeProfileRead := m.s.scopens + ":read"
	scopeProfileWrite := m.s.scopens + ":write"
	r.Post("", m.createProfile, gate.User(m.s.gate, scopeProfileWrite), m.rt)
	r.Put("", m.updateProfile, gate.User(m.s.gate, scopeProfileWrite), m.rt)
	r.Put("/image", m.updateImage, gate.User(m.s.gate, scopeProfileWrite), m.rt)
	r.Delete("/id/{id}", m.deleteProfile, gate.OwnerOrAdminParam(m.s.gate, "id", scopeProfileWrite), m.rt)
	r.Get("", m.getOwnProfile, gate.User(m.s.gate, scopeProfileRead), m.rt)
	r.Get("/id/{id}", m.getProfile, m.rt)
	r.Get("/id/{id}/image", m.getProfileImage, cachecontrol.Control(m.s.logger, true, nil, 60, m.getProfileImageCC), m.rt)
	r.Get("/ids", m.getProfilesBulk, m.rt)
}
