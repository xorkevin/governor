package profile

import (
	"errors"
	"io"
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
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
	m := s.profiles.New(userid, email, bio)

	if err := s.profiles.Insert(m); err != nil {
		if errors.Is(err, db.ErrUnique{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusConflict,
				Message: "Profile already created",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to create profile")
	}

	return &resProfileUpdate{
		Userid: userid,
	}, nil
}

func (s *service) UpdateProfile(userid, email, bio string) error {
	m, err := s.profiles.GetByID(userid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "No profile found with that id",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to get profile")
	}

	m.Email = email
	m.Bio = bio

	if err := s.profiles.Update(m); err != nil {
		return governor.ErrWithMsg(err, "Failed to update profile")
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
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "No profile found with that id",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to get profile")
	}

	img.ResizeFill(imgSize, imgSize)
	thumb := img.Duplicate()
	thumb.ResizeLimit(thumbSize, thumbSize)
	thumb64, err := thumb.ToBase64(thumbQuality)
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to encode image thumbnail")
	}
	imgJpeg, err := img.ToJpeg(imgQuality)
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to encode image")
	}

	if err := s.profileDir.Put(userid, image.MediaTypeJpeg, int64(imgJpeg.Len()), nil, imgJpeg); err != nil {
		return governor.ErrWithMsg(err, "Failed to save profile picture")
	}

	m.Image = thumb64
	if err := s.profiles.Update(m); err != nil {
		return governor.ErrWithMsg(err, "Failed to update profile")
	}
	return nil
}

func (s *service) DeleteProfile(userid string) error {
	m, err := s.profiles.GetByID(userid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "No profile found with that id",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to get profile")
	}

	if err := s.profileDir.Del(userid); err != nil {
		if !errors.Is(err, objstore.ErrNotFound{}) {
			return governor.ErrWithMsg(err, "Failed to delete profile picture")
		}
	}

	if err := s.profiles.Delete(m); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete profile")
	}
	return nil
}

func (s *service) GetProfile(userid string) (*resProfileModel, error) {
	m, err := s.profiles.GetByID(userid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "No profile found with that id",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to get profile")
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
		if errors.Is(err, objstore.ErrNotFound{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "Profile image not found",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to get profile image")
	}
	return objinfo, nil
}

func (s *service) GetProfileImage(userid string) (io.ReadCloser, string, error) {
	obj, objinfo, err := s.profileDir.Get(userid)
	if err != nil {
		if errors.Is(err, objstore.ErrNotFound{}) {
			return nil, "", governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "Profile image not found",
			}), governor.ErrOptInner(err))
		}
		return nil, "", governor.ErrWithMsg(err, "Failed to get profile image")
	}
	return obj, objinfo.ContentType, nil
}

func (s *service) GetProfilesBulk(userids []string) (*resProfiles, error) {
	m, err := s.profiles.GetBulk(userids)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get profiles")
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
