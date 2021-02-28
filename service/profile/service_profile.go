package profile

import (
	"io"
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/image"
	"xorkevin.dev/governor/service/objstore"
)

type (
	resProfileUpdate struct {
		Userid string `json:"userid"`
	}

	resProfileModel struct {
		Userid string `json:"userid"`
		Email  string `json:"contact_email"`
		Bio    string `json:"bio"`
		Image  string `json:"image"`
	}

	resProfiles struct {
		Profiles []resProfileModel `json:"profiles"`
	}
)

func (s *service) CreateProfile(userid, email, bio string) (*resProfileUpdate, error) {
	m, err := s.profiles.New(userid, email, bio)
	if err != nil {
		return nil, err
	}

	if err := s.profiles.Insert(m); err != nil {
		if governor.ErrorStatus(err) == http.StatusBadRequest {
			return nil, governor.NewErrorUser("", 0, err)
		}
		return nil, err
	}

	return &resProfileUpdate{
		Userid: userid,
	}, nil
}

func (s *service) UpdateProfile(userid, email, bio string) error {
	m, err := s.profiles.GetByID(userid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return governor.NewErrorUser("", 0, err)
		}
		return err
	}

	m.Email = email
	m.Bio = bio

	if err := s.profiles.Update(m); err != nil {
		return err
	}
	return nil
}

const (
	imgSize      = 384
	imgQuality   = 85
	thumbSize    = 8
	thumbQuality = 0
)

func (s *service) UpdateImage(userid string, img image.Image) error {
	m, err := s.profiles.GetByID(userid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return governor.NewErrorUser("", 0, err)
		}
		return err
	}

	img.ResizeFill(imgSize, imgSize)
	thumb := img.Duplicate()
	thumb.ResizeLimit(thumbSize, thumbSize)
	thumb64, err := thumb.ToBase64(thumbQuality)
	if err != nil {
		return governor.NewError("Failed to encode thumbnail to base64", http.StatusInternalServerError, err)
	}
	imgJpeg, err := img.ToJpeg(imgQuality)
	if err != nil {
		return governor.NewError("Failed to encode image to jpeg", http.StatusInternalServerError, err)
	}

	if err := s.profileDir.Put(userid, image.MediaTypeJpeg, int64(imgJpeg.Len()), imgJpeg); err != nil {
		return governor.NewError("Failed to save profile picture", http.StatusInternalServerError, err)
	}

	m.Image = thumb64
	if err := s.profiles.Update(m); err != nil {
		return err
	}
	return nil
}

func (s *service) DeleteProfile(userid string) error {
	m, err := s.profiles.GetByID(userid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return governor.NewErrorUser("", 0, err)
		}
		return err
	}

	if err := s.profileDir.Del(userid); err != nil {
		if governor.ErrorStatus(err) != http.StatusNotFound {
			return governor.NewError("Unable to delete profile picture", http.StatusInternalServerError, err)
		}
	}

	if err := s.profiles.Delete(m); err != nil {
		return err
	}
	return nil
}

func (s *service) GetProfile(userid string) (*resProfileModel, error) {
	m, err := s.profiles.GetByID(userid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return nil, governor.NewErrorUser("", 0, err)
		}
		return nil, err
	}
	return &resProfileModel{
		Userid: m.Userid,
		Email:  m.Email,
		Bio:    m.Bio,
		Image:  m.Image,
	}, nil
}

func (s *service) StatProfileImage(userid string) (*objstore.ObjectInfo, error) {
	objinfo, err := s.profileDir.Stat(userid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return nil, governor.NewErrorUser("Profile image not found", 0, err)
		}
		return nil, governor.NewError("Failed to get profile image", http.StatusInternalServerError, err)
	}
	return objinfo, nil
}

func (s *service) GetProfileImage(userid string) (io.ReadCloser, string, error) {
	obj, objinfo, err := s.profileDir.Get(userid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return nil, "", governor.NewErrorUser("Profile image not found", 0, err)
		}
		return nil, "", governor.NewError("Failed to get profile image", http.StatusInternalServerError, err)
	}
	return obj, objinfo.ContentType, nil
}

func (s *service) GetProfilesBulk(userids []string) (*resProfiles, error) {
	m, err := s.profiles.GetBulk(userids)
	if err != nil {
		return nil, err
	}

	res := make([]resProfileModel, 0, len(m))
	for _, i := range m {
		res = append(res, resProfileModel{
			Userid: i.Userid,
			Email:  i.Email,
			Bio:    i.Bio,
			Image:  i.Image,
		})
	}

	return &resProfiles{
		Profiles: res,
	}, nil
}
