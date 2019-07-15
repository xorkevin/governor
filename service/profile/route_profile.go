package profile

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/image"
	"github.com/hackform/governor/service/user/gate"
	"github.com/labstack/echo"
	"io"
	"net/http"
)

//go:generate forge validation -o validation_profile_gen.go reqProfileGetID reqProfileModel

type (
	reqProfileModel struct {
		Userid string `valid:"userid,has" json:"-"`
		Email  string `valid:"email" json:"contact_email"`
		Bio    string `valid:"bio" json:"bio"`
	}
)

func (p *profileRouter) createProfile(c echo.Context) error {
	rprofile := reqProfileModel{}
	if err := c.Bind(&rprofile); err != nil {
		return err
	}
	rprofile.Userid = c.Get("userid").(string)
	if err := rprofile.valid(); err != nil {
		return err
	}

	res, err := p.service.CreateProfile(rprofile.Userid, rprofile.Email, rprofile.Bio)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, res)
}

func (p *profileRouter) updateProfile(c echo.Context) error {
	rprofile := reqProfileModel{}
	if err := c.Bind(&rprofile); err != nil {
		return err
	}
	rprofile.Userid = c.Get("userid").(string)
	if err := rprofile.valid(); err != nil {
		return err
	}

	if err := p.service.UpdateProfile(rprofile.Userid, rprofile.Email, rprofile.Bio); err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

type (
	reqProfileGetID struct {
		Userid string `valid:"userid,has" json:"userid"`
	}
)

func (p *profileRouter) updateImage(c echo.Context) error {
	img := c.Get("image").(io.Reader)
	imgSize := c.Get("imagesize").(int64)
	thumb64 := c.Get("thumbnail").(string)

	ruser := reqProfileGetID{
		Userid: c.Get("userid").(string),
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	if err := p.service.UpdateImage(ruser.Userid, img, imgSize, thumb64); err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

func (p *profileRouter) deleteProfile(c echo.Context) error {
	rprofile := reqProfileGetID{
		Userid: c.Param("id"),
	}
	if err := rprofile.valid(); err != nil {
		return err
	}

	if err := p.service.DeleteProfile(rprofile.Userid); err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

func (p *profileRouter) getOwnProfile(c echo.Context) error {
	ruser := reqProfileGetID{
		Userid: c.Get("userid").(string),
	}
	res, err := p.service.GetProfile(ruser.Userid)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, res)
}

func (p *profileRouter) getProfile(c echo.Context) error {
	rprofile := reqProfileGetID{
		Userid: c.Param("id"),
	}
	if err := rprofile.valid(); err != nil {
		return err
	}

	res, err := p.service.GetProfile(rprofile.Userid)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, res)
}

func (p *profileRouter) getProfileImage(c echo.Context) error {
	rprofile := reqProfileGetID{
		Userid: c.Param("id"),
	}
	if err := rprofile.valid(); err != nil {
		return err
	}

	image, contentType, err := p.service.GetProfileImage(rprofile.Userid)
	if err != nil {
		return err
	}
	return c.Stream(http.StatusOK, contentType, image)
}

func (p *profileRouter) getProfileImageCC(c echo.Context) (string, error) {
	rprofile := reqProfileGetID{
		Userid: c.Param("id"),
	}
	if err := rprofile.valid(); err != nil {
		return "", err
	}

	objinfo, err := p.service.StatProfileImage(rprofile.Userid)
	if err != nil {
		return "", err
	}

	return objinfo.ETag, nil
}

func (p *profileRouter) mountProfileRoutes(conf governor.Config, r *echo.Group) error {
	r.POST("", p.createProfile, gate.User(p.service.gate))
	r.PUT("", p.updateProfile, gate.User(p.service.gate))
	r.PUT("/image", p.updateImage, gate.User(p.service.gate), p.service.img.LoadJpeg("image", image.Options{
		Width:          384,
		Height:         384,
		ThumbWidth:     32,
		ThumbHeight:    32,
		Quality:        85,
		ThumbQuality:   50,
		Crop:           true,
		ContextField:   "image",
		SizeField:      "imagesize",
		ThumbnailField: "thumbnail",
	}))
	r.DELETE("/:id", p.deleteProfile, gate.OwnerOrAdmin(p.service.gate, "id"))
	r.GET("", p.getOwnProfile, gate.User(p.service.gate))
	r.GET("/:id", p.getProfile)
	r.GET("/:id/image", p.getProfileImage, p.service.cc.Control(true, false, min15, p.getProfileImageCC))
	return nil
}
