package profile

import (
	"net/http"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/cachecontrol"
	"xorkevin.dev/governor/service/image"
	"xorkevin.dev/governor/service/user/gate"
)

//go:generate forge validation -o validation_profile_gen.go reqProfileGetID reqProfileModel

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
	req.Userid = c.Get(gate.CtxUserid).(string)
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.CreateProfile(req.Userid, req.Email, req.Bio)
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
	req.Userid = c.Get(gate.CtxUserid).(string)
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := m.s.UpdateProfile(req.Userid, req.Email, req.Bio); err != nil {
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
		Userid: c.Get(gate.CtxUserid).(string),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := m.s.UpdateImage(req.Userid, img); err != nil {
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

	if err := m.s.DeleteProfile(req.Userid); err != nil {
		c.WriteError(err)
		return
	}

	c.WriteStatus(http.StatusNoContent)
}

func (m *router) getOwnProfile(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqProfileGetID{
		Userid: c.Get(gate.CtxUserid).(string),
	}
	res, err := m.s.GetProfile(req.Userid)
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

	res, err := m.s.GetProfile(req.Userid)
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

	image, contentType, err := m.s.GetProfileImage(req.Userid)
	if err != nil {
		c.WriteError(err)
		return
	}
	defer func() {
		if err := image.Close(); err != nil {
			m.s.logger.Error("failed to close profile image", map[string]string{
				"actiontype": "getprofileimage",
				"error":      err.Error(),
			})
		}
	}()
	c.WriteFile(http.StatusOK, contentType, image)
}

func (m *router) getProfileImageCC(c governor.Context) (string, error) {
	req := reqProfileGetID{
		Userid: c.Param("id"),
	}
	if err := req.valid(); err != nil {
		return "", err
	}

	objinfo, err := m.s.StatProfileImage(req.Userid)
	if err != nil {
		return "", err
	}

	return objinfo.ETag, nil
}

func (m *router) mountProfileRoutes(r governor.Router) {
	r.Post("", m.createProfile, gate.User(m.s.gate))
	r.Put("", m.updateProfile, gate.User(m.s.gate))
	r.Put("/image", m.updateImage, gate.User(m.s.gate))
	r.Delete("/{id}", m.deleteProfile, gate.OwnerOrAdminParam(m.s.gate, "id"))
	r.Get("", m.getOwnProfile, gate.User(m.s.gate))
	r.Get("/{id}", m.getProfile)
	r.Get("/{id}/image", m.getProfileImage, cachecontrol.Control(m.s.logger, true, false, 60, m.getProfileImageCC))
}
