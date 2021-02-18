package oauth

import (
	"encoding/json"
	"io"
	"net/http"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/image"
	"xorkevin.dev/governor/service/objstore"
	"xorkevin.dev/governor/service/user/oauth/model"
)

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

func (s *service) GetAppsBulk(clientids []string) (*resApps, error) {
	m, err := s.apps.GetBulk(clientids)
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

func (s *service) getCachedClient(clientid string) (*model.Model, error) {
	if clientstr, err := s.kvclient.Get(clientid); err != nil {
		if governor.ErrorStatus(err) != http.StatusNotFound {
			s.logger.Error("Failed to get oauth client from cache", map[string]string{
				"error":      err.Error(),
				"actiontype": "getcacheclient",
			})
		}
	} else if clientstr == cacheValTombstone {
		return nil, governor.NewError("App not found", http.StatusNotFound, nil)
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
		if governor.ErrorStatus(err) == http.StatusNotFound {
			if err := s.kvclient.Set(clientid, cacheValTombstone, s.keyCacheTime); err != nil {
				s.logger.Error("Failed to set oauth client in cache", map[string]string{
					"error":      err.Error(),
					"actiontype": "setcacheclient",
				})
			}
		}
		return nil, err
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
	m, err := s.getCachedClient(clientid)
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
	if err := s.kvclient.Del(clientid); err != nil {
		s.logger.Error("Failed to clear oauth client from cache", map[string]string{
			"error":      err.Error(),
			"actiontype": "clearcacheclient",
		})
	}
}
