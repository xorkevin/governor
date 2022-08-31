package profile

import (
	"context"
	"errors"
	"io"
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/image"
	"xorkevin.dev/governor/service/objstore"
	"xorkevin.dev/kerrors"
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

func (s *service) CreateProfile(ctx context.Context, userid, email, bio string) (*resProfileUpdate, error) {
	m := s.profiles.New(userid, email, bio)

	if err := s.profiles.Insert(ctx, m); err != nil {
		if errors.Is(err, db.ErrUnique{}) {
			return nil, governor.ErrWithRes(err, http.StatusConflict, "", "Profile already created")
		}
		return nil, kerrors.WithMsg(err, "Failed to create profile")
	}

	return &resProfileUpdate{
		Userid: userid,
	}, nil
}

func (s *service) UpdateProfile(ctx context.Context, userid, email, bio string) error {
	m, err := s.profiles.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "No profile found with that id")
		}
		return kerrors.WithMsg(err, "Failed to get profile")
	}

	m.Email = email
	m.Bio = bio

	if err := s.profiles.Update(ctx, m); err != nil {
		return kerrors.WithMsg(err, "Failed to update profile")
	}
	return nil
}

const (
	imgSize      = 384
	imgQuality   = 85
	thumbSize    = 8
	thumbQuality = 0
)

func (s *service) UpdateImage(ctx context.Context, userid string, img image.Image) error {
	m, err := s.profiles.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "No profile found with that id")
		}
		return kerrors.WithMsg(err, "Failed to get profile")
	}

	img.ResizeFill(imgSize, imgSize)
	thumb := img.Duplicate()
	thumb.ResizeLimit(thumbSize, thumbSize)
	thumb64, err := thumb.ToBase64(thumbQuality)
	if err != nil {
		return kerrors.WithMsg(err, "Failed to encode image thumbnail")
	}
	imgJpeg, err := img.ToJpeg(imgQuality)
	if err != nil {
		return kerrors.WithMsg(err, "Failed to encode image")
	}

	if err := s.profileDir.Put(ctx, userid, image.MediaTypeJpeg, int64(imgJpeg.Len()), nil, imgJpeg); err != nil {
		return kerrors.WithMsg(err, "Failed to save profile picture")
	}

	m.Image = thumb64
	if err := s.profiles.Update(ctx, m); err != nil {
		return kerrors.WithMsg(err, "Failed to update profile")
	}
	return nil
}

func (s *service) DeleteProfile(ctx context.Context, userid string) error {
	m, err := s.profiles.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "No profile found with that id")
		}
		return kerrors.WithMsg(err, "Failed to get profile")
	}

	if err := s.profileDir.Del(ctx, userid); err != nil {
		if !errors.Is(err, objstore.ErrNotFound{}) {
			return kerrors.WithMsg(err, "Failed to delete profile picture")
		}
	}

	if err := s.profiles.Delete(ctx, m); err != nil {
		return kerrors.WithMsg(err, "Failed to delete profile")
	}
	return nil
}

func (s *service) GetProfile(ctx context.Context, userid string) (*resProfileModel, error) {
	m, err := s.profiles.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "No profile found with that id")
		}
		return nil, kerrors.WithMsg(err, "Failed to get profile")
	}
	return &resProfileModel{
		Userid: m.Userid,
		Email:  m.Email,
		Bio:    m.Bio,
		Image:  m.Image,
	}, nil
}

func (s *service) StatProfileImage(ctx context.Context, userid string) (*objstore.ObjectInfo, error) {
	objinfo, err := s.profileDir.Stat(ctx, userid)
	if err != nil {
		if errors.Is(err, objstore.ErrNotFound{}) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "Profile image not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get profile image")
	}
	return objinfo, nil
}

func (s *service) GetProfileImage(ctx context.Context, userid string) (io.ReadCloser, string, error) {
	obj, objinfo, err := s.profileDir.Get(ctx, userid)
	if err != nil {
		if errors.Is(err, objstore.ErrNotFound{}) {
			return nil, "", governor.ErrWithRes(err, http.StatusNotFound, "", "Profile image not found")
		}
		return nil, "", kerrors.WithMsg(err, "Failed to get profile image")
	}
	return obj, objinfo.ContentType, nil
}

func (s *service) GetProfilesBulk(ctx context.Context, userids []string) (*resProfiles, error) {
	m, err := s.profiles.GetBulk(ctx, userids)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get profiles")
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
