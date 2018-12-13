package profile

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/image"
	"io"
)

type (
	resProfileUpdate struct {
		Userid string `json:"userid"`
	}

	resProfileModel struct {
		Email string `json:"contact_email"`
		Bio   string `json:"bio"`
		Image string `json:"image"`
	}
)

func (p *profileService) CreateProfile(userid, email, bio string) (*resProfileUpdate, *governor.Error) {
	m, err := p.repo.New(userid, email, bio)
	if err != nil {
		return nil, err
	}

	if err := p.repo.Insert(m); err != nil {
		if err.Code() == 3 {
			err.SetErrorUser()
		}
		err.AddTrace(moduleID)
		return nil, err
	}

	return &resProfileUpdate{
		Userid: userid,
	}, nil
}

func (p *profileService) UpdateProfile(userid, email, bio string) *governor.Error {
	m, err := p.repo.GetByID(userid)
	if err != nil {
		if err.Code() == 2 {
			err.SetErrorUser()
		}
		err.AddTrace(moduleID)
		return err
	}

	m.Email = email
	m.Bio = bio

	if err := p.repo.Update(m); err != nil {
		err.AddTrace(moduleID)
		return err
	}
	return nil
}

func (p *profileService) UpdateImage(userid string, img io.Reader, imgSize int64, thumb64 string) *governor.Error {
	m, err := p.repo.GetByID(userid)
	if err != nil {
		if err.Code() == 2 {
			err.SetErrorUser()
		}
		err.AddTrace(moduleID)
		return err
	}

	if err := p.obj.Put(userid+"-profile", image.MediaTypeJpeg, imgSize, img); err != nil {
		err.AddTrace(moduleID)
		return err
	}

	m.Image = thumb64
	if err := p.repo.Update(m); err != nil {
		err.AddTrace(moduleID)
		return err
	}
	return nil
}

func (p *profileService) DeleteProfile(userid string) *governor.Error {
	m, err := p.repo.GetByID(userid)
	if err != nil {
		if err.Code() == 2 {
			err.SetErrorUser()
		}
		err.AddTrace(moduleID)
		return err
	}

	if err := p.repo.Delete(m); err != nil {
		err.AddTrace(moduleID)
		return err
	}
	return nil
}

func (p *profileService) GetProfile(userid string) (*resProfileModel, *governor.Error) {
	m, err := p.repo.GetByID(userid)
	if err != nil {
		if err.Code() == 2 {
			err.SetErrorUser()
		}
		err.AddTrace(moduleID)
		return nil, err
	}
	return &resProfileModel{
		Email: m.Email,
		Bio:   m.Bio,
		Image: m.Image,
	}, nil
}
