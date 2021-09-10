package oauth

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/image"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/objstore"
	"xorkevin.dev/governor/service/user/oauth/model"
)

type (
	// ErrNotFound is returned when an apikey is not found
	ErrNotFound struct{}
)

func (e ErrNotFound) Error() string {
	return "OAuth app not found"
}

const (
	cacheValTombstone = "-"
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
		return nil, governor.ErrWithMsg(err, "Failed to get oauth apps")
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

func (s *service) GetAppsBulk(clientids []string) (*resApps, error) {
	m, err := s.apps.GetBulk(clientids)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get oauth apps")
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

func (s *service) getCachedClient(clientid string) (*model.Model, error) {
	if clientstr, err := s.kvclient.Get(clientid); err != nil {
		if !errors.Is(err, kvstore.ErrNotFound{}) {
			s.logger.Error("Failed to get oauth client from cache", map[string]string{
				"error":      err.Error(),
				"actiontype": "getcacheclient",
			})
		}
	} else if clientstr == cacheValTombstone {
		return nil, governor.ErrWithKind(err, ErrNotFound{}, "OAuth app not found")
	} else {
		cm := &model.Model{}
		if err := json.Unmarshal([]byte(clientstr), cm); err != nil {
			s.logger.Error("Malformed oauth client cache json", map[string]string{
				"error":      err.Error(),
				"actiontype": "unmarshalclientjson",
			})
		} else {
			return cm, nil
		}
	}

	m, err := s.apps.GetByID(clientid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			if err := s.kvclient.Set(clientid, cacheValTombstone, s.keyCacheTime); err != nil {
				s.logger.Error("Failed to set oauth client in cache", map[string]string{
					"error":      err.Error(),
					"actiontype": "setcacheclient",
				})
			}
			return nil, governor.ErrWithKind(err, ErrNotFound{}, "OAuth app not found")
		}
		return nil, governor.ErrWithMsg(err, "Failed to get oauth app")
	}

	if clientbytes, err := json.Marshal(m); err != nil {
		s.logger.Error("Failed to marshal client to json", map[string]string{
			"error":      err.Error(),
			"actiontype": "marshalclientjson",
		})
	} else if err := s.kvclient.Set(clientid, string(clientbytes), s.keyCacheTime); err != nil {
		s.logger.Error("Failed to set oauth client in cache", map[string]string{
			"error":      err.Error(),
			"actiontype": "setcacheclient",
		})
	}

	return m, nil
}

type (
	resCreate struct {
		ClientID string `json:"client_id"`
		Key      string `json:"key"`
	}
)

func (s *service) CreateApp(name, url, redirectURI, creatorID string) (*resCreate, error) {
	m, key, err := s.apps.New(name, url, redirectURI, creatorID)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to create oauth app")
	}
	if err := s.apps.Insert(m); err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to insert oauth app")
	}
	return &resCreate{
		ClientID: m.ClientID,
		Key:      key,
	}, nil
}

func (s *service) RotateAppKey(clientid string) (*resCreate, error) {
	m, err := s.apps.GetByID(clientid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "OAuth app not found",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to get oauth app")
	}
	key, err := s.apps.RehashKey(m)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to rotate client key")
	}
	if err := s.apps.Update(m); err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to update oauth app")
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
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "OAuth app not found",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to get oauth app")
	}
	m.Name = name
	m.URL = url
	m.RedirectURI = redirectURI
	if err := s.apps.Update(m); err != nil {
		return governor.ErrWithMsg(err, "Failed to update oauth app")
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
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "OAuth app not found",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to get oauth app")
	}

	img.ResizeFill(imgSize, imgSize)
	thumb := img.Duplicate()
	thumb.ResizeLimit(thumbSize, thumbSize)
	thumb64, err := thumb.ToBase64(thumbQuality)
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to encode thumbnail to base64")
	}
	imgpng, err := img.ToPng(image.PngBest)
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to encode image to png")
	}
	if err := s.logoImgDir.Put(m.ClientID, image.MediaTypePng, int64(imgpng.Len()), nil, imgpng); err != nil {
		return governor.ErrWithMsg(err, "Failed to save app logo")
	}

	m.Logo = thumb64
	if err := s.apps.Update(m); err != nil {
		return governor.ErrWithMsg(err, "Failed to update oauth app")
	}
	s.clearCache(clientid)
	return nil
}

func (s *service) Delete(clientid string) error {
	m, err := s.apps.GetByID(clientid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "OAuth app not found",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to get oauth app")
	}

	if err := s.logoImgDir.Del(clientid); err != nil {
		if !errors.Is(err, objstore.ErrNotFound{}) {
			return governor.ErrWithMsg(err, "Unable to delete app logo")
		}
	}

	if err := s.apps.Delete(m); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete oauth app")
	}
	s.clearCache(clientid)
	return nil
}

func (s *service) GetApp(clientid string) (*resApp, error) {
	m, err := s.getCachedClient(clientid)
	if err != nil {
		if errors.Is(err, ErrNotFound{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "OAuth app not found",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to get oauth app")
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
		if errors.Is(err, objstore.ErrNotFound{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "OAuth app logo not found",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to get oauth app logo")
	}
	return objinfo, nil
}

func (s *service) GetLogo(clientid string) (io.ReadCloser, string, error) {
	obj, objinfo, err := s.logoImgDir.Get(clientid)
	if err != nil {
		if errors.Is(err, objstore.ErrNotFound{}) {
			return nil, "", governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "OAuth app logo not found",
			}), governor.ErrOptInner(err))
		}
		return nil, "", governor.ErrWithMsg(err, "Failed to get app logo")
	}
	return obj, objinfo.ContentType, nil
}

func (s *service) clearCache(clientid string) {
	if err := s.kvclient.Del(clientid); err != nil {
		s.logger.Error("Failed to clear oauth client from cache", map[string]string{
			"error":      err.Error(),
			"actiontype": "clearcacheclient",
		})
	}
}
