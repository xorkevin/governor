package profile

import (
	"github.com/labstack/echo"
	"net/http"
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

func (r *router) createProfile(c echo.Context) error {
	rprofile := reqProfileModel{}
	if err := c.Bind(&rprofile); err != nil {
		return err
	}
	rprofile.Userid = c.Get("userid").(string)
	if err := rprofile.valid(); err != nil {
		return err
	}

	res, err := r.s.CreateProfile(rprofile.Userid, rprofile.Email, rprofile.Bio)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, res)
}

func (r *router) updateProfile(c echo.Context) error {
	rprofile := reqProfileModel{}
	if err := c.Bind(&rprofile); err != nil {
		return err
	}
	rprofile.Userid = c.Get("userid").(string)
	if err := rprofile.valid(); err != nil {
		return err
	}

	if err := r.s.UpdateProfile(rprofile.Userid, rprofile.Email, rprofile.Bio); err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

type (
	reqProfileGetID struct {
		Userid string `valid:"userid,has" json:"userid"`
	}
)

func (r *router) updateImage(c echo.Context) error {
	img, thumb64, err := image.LoadJpeg(c, "image", image.Options{
		Width:  384,
		Height: 384,
		Fill:   true,
	})
	if err != nil {
		return err
	}
	imgSize := int64(img.Len())

	ruser := reqProfileGetID{
		Userid: c.Get("userid").(string),
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	if err := r.s.UpdateImage(ruser.Userid, img, imgSize, thumb64); err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

func (r *router) deleteProfile(c echo.Context) error {
	rprofile := reqProfileGetID{
		Userid: c.Param("id"),
	}
	if err := rprofile.valid(); err != nil {
		return err
	}

	if err := r.s.DeleteProfile(rprofile.Userid); err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

func (r *router) getOwnProfile(c echo.Context) error {
	ruser := reqProfileGetID{
		Userid: c.Get("userid").(string),
	}
	res, err := r.s.GetProfile(ruser.Userid)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, res)
}

func (r *router) getProfile(c echo.Context) error {
	rprofile := reqProfileGetID{
		Userid: c.Param("id"),
	}
	if err := rprofile.valid(); err != nil {
		return err
	}

	res, err := r.s.GetProfile(rprofile.Userid)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, res)
}

func (r *router) getProfileImage(c echo.Context) error {
	rprofile := reqProfileGetID{
		Userid: c.Param("id"),
	}
	if err := rprofile.valid(); err != nil {
		return err
	}

	image, contentType, err := r.s.GetProfileImage(rprofile.Userid)
	if err != nil {
		return err
	}
	return c.Stream(http.StatusOK, contentType, image)
}

func (r *router) getProfileImageCC(c echo.Context) (string, error) {
	rprofile := reqProfileGetID{
		Userid: c.Param("id"),
	}
	if err := rprofile.valid(); err != nil {
		return "", err
	}

	objinfo, err := r.s.StatProfileImage(rprofile.Userid)
	if err != nil {
		return "", err
	}

	return objinfo.ETag, nil
}

func (r *router) mountProfileRoutes(g *echo.Group) error {
	g.POST("", r.createProfile, gate.User(r.s.gate))
	g.PUT("", r.updateProfile, gate.User(r.s.gate))
	g.PUT("/image", r.updateImage, gate.User(r.s.gate))
	g.DELETE("/:id", r.deleteProfile, gate.OwnerOrAdmin(r.s.gate, "id"))
	g.GET("", r.getOwnProfile, gate.User(r.s.gate))
	g.GET("/:id", r.getProfile)
	g.GET("/:id/image", r.getProfileImage, cachecontrol.Control(true, false, min15, r.getProfileImageCC))
	return nil
}
