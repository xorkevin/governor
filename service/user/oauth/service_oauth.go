package oauth

import (
	"io"
	"net/http"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/image"
	"xorkevin.dev/governor/service/objstore"
)

type (
	resApp struct {
		ClientID     string `json:"client_id"`
		Name         string `json:"name"`
		URL          string `json:"url"`
		RedirectURI  string `json:"redirect_uri"`
		Logo         string `json:"logo"`
		Time         int64  `json:"time"`
		CreationTime int64  `json:"creation_time"`
	}

	resApps struct {
		Apps []resApp `json:"apps"`
	}
)

func (s *service) GetApps(limit, offset int, creatorid string) (*resApps, error) {
	m, err := s.apps.GetApps(limit, offset, creatorid)
	if err != nil {
		return nil, err
	}
	res := make([]resApp, 0, len(m))
	for _, i := range m {
		res = append(res, resApp{
			ClientID:     i.ClientID,
			Name:         i.Name,
			URL:          i.URL,
			RedirectURI:  i.RedirectURI,
			Logo:         i.Logo,
			Time:         i.Time,
			CreationTime: i.CreationTime,
		})
	}
	return &resApps{
		Apps: res,
	}, nil
}

func (s *service) CheckKey(clientid, key string) error {
	m, err := s.apps.GetByID(clientid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return governor.NewError("Invalid key", http.StatusUnauthorized, nil)
		}
		return err
	}

	if ok, err := s.apps.ValidateKey(key, m); err != nil || !ok {
		return governor.NewError("Invalid key", http.StatusUnauthorized, nil)
	}
	return nil
}

type (
	resCreate struct {
		ClientID string `json:"client_id"`
		Key      string `json:"key"`
	}
)

func (s *service) CreateApp(name, url, redirectURI string) (*resCreate, error) {
	m, key, err := s.apps.New(name, url, redirectURI)
	if err != nil {
		return nil, err
	}
	if err := s.apps.Insert(m); err != nil {
		return nil, err
	}
	return &resCreate{
		ClientID: m.ClientID,
		Key:      key,
	}, nil
}

func (s *service) RotateAppKey(clientid string) (*resCreate, error) {
	m, err := s.apps.GetByID(clientid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return nil, governor.NewErrorUser("", 0, err)
		}
		return nil, err
	}
	key, err := s.apps.RehashKey(m)
	if err != nil {
		return nil, err
	}
	if err := s.apps.Update(m); err != nil {
		return nil, err
	}
	s.clearCache(clientid)
	return &resCreate{
		ClientID: clientid,
		Key:      key,
	}, nil
}

func (s *service) UpdateApp(clientid string, name, url, redirectURI string) error {
	m, err := s.apps.GetByID(clientid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return governor.NewErrorUser("", 0, err)
		}
		return err
	}
	m.Name = name
	m.URL = url
	m.RedirectURI = redirectURI
	if err := s.apps.Update(m); err != nil {
		return err
	}
	s.clearCache(clientid)
	return nil
}

const (
	imgSize      = 256
	thumbSize    = 8
	thumbQuality = 0
)

func (s *service) UpdateLogo(clientid string, img image.Image) error {
	m, err := s.apps.GetByID(clientid)
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
	imgpng, err := img.ToPng(image.PngBest)
	if err != nil {
		return governor.NewError("Failed to encode image to png", http.StatusInternalServerError, err)
	}
	if err := s.logoImgDir.Put(m.ClientID, image.MediaTypePng, int64(imgpng.Len()), imgpng); err != nil {
		return governor.NewError("Failed to save app logo", http.StatusInternalServerError, err)
	}

	m.Logo = thumb64
	if err := s.apps.Update(m); err != nil {
		return err
	}
	s.clearCache(clientid)
	return nil
}

func (s *service) Delete(clientid string) error {
	m, err := s.apps.GetByID(clientid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return governor.NewErrorUser("", 0, err)
		}
		return err
	}

	if err := s.logoImgDir.Del(clientid); err != nil {
		if governor.ErrorStatus(err) != http.StatusNotFound {
			return governor.NewError("Unable to delete app logo", http.StatusInternalServerError, err)
		}
	}

	if err := s.apps.Delete(m); err != nil {
		return err
	}
	s.clearCache(clientid)
	return nil
}

func (s *service) GetApp(clientid string) (*resApp, error) {
	m, err := s.apps.GetByID(clientid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return nil, governor.NewErrorUser("", 0, err)
		}
		return nil, err
	}
	return &resApp{
		ClientID:     m.ClientID,
		Name:         m.Name,
		URL:          m.URL,
		RedirectURI:  m.RedirectURI,
		Logo:         m.Logo,
		Time:         m.Time,
		CreationTime: m.CreationTime,
	}, nil
}

func (s *service) StatLogo(clientid string) (*objstore.ObjectInfo, error) {
	objinfo, err := s.logoImgDir.Stat(clientid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return nil, governor.NewErrorUser("App logo not found", 0, err)
		}
		return nil, governor.NewError("Failed to get app logo", http.StatusInternalServerError, err)
	}
	return objinfo, nil
}

func (s *service) GetLogo(clientid string) (io.ReadCloser, string, error) {
	obj, objinfo, err := s.logoImgDir.Get(clientid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return nil, "", governor.NewErrorUser("App logo not found", 0, err)
		}
		return nil, "", governor.NewError("Failed to get app logo", http.StatusInternalServerError, err)
	}
	return obj, objinfo.ContentType, nil
}

func (s *service) clearCache(clientid string) {
}
