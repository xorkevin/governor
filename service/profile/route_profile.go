package profile

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
	reqProfileModel struct {
		Userid string `valid:"userid,has" json:"-"`
		Email  string `valid:"email" json:"contact_email"`
		Bio    string `valid:"bio" json:"bio"`
	}
)

func (s *router) createProfile(c governor.Context) {
	var req reqProfileModel
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := s.s.createProfile(c.Ctx(), req.Userid, req.Email, req.Bio)
	if err != nil {
		c.WriteError(err)
		return
	}

	c.WriteJSON(http.StatusCreated, res)
}

func (s *router) updateProfile(c governor.Context) {
	var req reqProfileModel
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := s.s.updateProfile(c.Ctx(), req.Userid, req.Email, req.Bio); err != nil {
		c.WriteError(err)
		return
	}

	c.WriteStatus(http.StatusNoContent)
}

type (
	//forge:valid
	reqProfileGetID struct {
		Userid string `valid:"userid,has" json:"userid"`
	}
)

func (s *router) updateImage(c governor.Context) {
	img, err := image.LoadImage(c, "image")
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

	if err := s.s.updateImage(c.Ctx(), req.Userid, img); err != nil {
		c.WriteError(err)
		return
	}

	c.WriteStatus(http.StatusNoContent)
}

func (s *router) deleteProfile(c governor.Context) {
	req := reqProfileGetID{
		Userid: c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := s.s.deleteProfile(c.Ctx(), req.Userid); err != nil {
		c.WriteError(err)
		return
	}

	c.WriteStatus(http.StatusNoContent)
}

func (s *router) getOwnProfile(c governor.Context) {
	req := reqProfileGetID{
		Userid: gate.GetCtxUserid(c),
	}
	res, err := s.s.getProfile(c.Ctx(), req.Userid)
	if err != nil {
		c.WriteError(err)
		return
	}

	c.WriteJSON(http.StatusOK, res)
}

func (s *router) getProfile(c governor.Context) {
	req := reqProfileGetID{
		Userid: c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := s.s.getProfile(c.Ctx(), req.Userid)
	if err != nil {
		c.WriteError(err)
		return
	}

	c.WriteJSON(http.StatusOK, res)
}

func (s *router) getProfileImage(c governor.Context) {
	req := reqProfileGetID{
		Userid: c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	image, contentType, err := s.s.getProfileImage(c.Ctx(), req.Userid)
	if err != nil {
		c.WriteError(err)
		return
	}
	defer func() {
		if err := image.Close(); err != nil {
			s.s.log.Err(c.Ctx(), kerrors.WithMsg(err, "Failed to close profile image"), nil)
		}
	}()
	c.WriteFile(http.StatusOK, contentType, image)
}

type (
	//forge:valid
	reqGetProfiles struct {
		Userids []string `valid:"userids,has" json:"-"`
	}
)

func (s *router) getProfilesBulk(c governor.Context) {
	req := reqGetProfiles{
		Userids: strings.Split(c.Query("ids"), ","),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := s.s.getProfilesBulk(c.Ctx(), req.Userids)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (s *router) getProfileImageCC(c governor.Context) (string, error) {
	req := reqProfileGetID{
		Userid: c.Param("id"),
	}
	if err := req.valid(); err != nil {
		return "", err
	}

	objinfo, err := s.s.statProfileImage(c.Ctx(), req.Userid)
	if err != nil {
		return "", err
	}

	return objinfo.ETag, nil
}

func (s *router) mountProfileRoutes(r governor.Router) {
	m := governor.NewMethodRouter(r)
	scopeProfileRead := s.s.scopens + ":read"
	scopeProfileWrite := s.s.scopens + ":write"
	m.PostCtx("", s.createProfile, gate.User(s.s.gate, scopeProfileWrite), s.rt)
	m.PutCtx("", s.updateProfile, gate.User(s.s.gate, scopeProfileWrite), s.rt)
	m.PutCtx("/image", s.updateImage, gate.User(s.s.gate, scopeProfileWrite), s.rt)
	m.DeleteCtx("/id/{id}", s.deleteProfile, gate.OwnerOrAdminParam(s.s.gate, "id", scopeProfileWrite), s.rt)
	m.GetCtx("", s.getOwnProfile, gate.User(s.s.gate, scopeProfileRead), s.rt)
	m.GetCtx("/id/{id}", s.getProfile, s.rt)
	m.GetCtx("/id/{id}/image", s.getProfileImage, cachecontrol.ControlCtx(true, nil, 60, s.getProfileImageCC), s.rt)
	m.GetCtx("/ids", s.getProfilesBulk, s.rt)
}
