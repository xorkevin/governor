package oauth

import (
	"context"
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
	"xorkevin.dev/kerrors"
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

func (s *service) GetApps(ctx context.Context, limit, offset int, creatorid string) (*resApps, error) {
	m, err := s.apps.GetApps(ctx, limit, offset, creatorid)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get oauth apps")
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

func (s *service) GetAppsBulk(ctx context.Context, clientids []string) (*resApps, error) {
	m, err := s.apps.GetBulk(ctx, clientids)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get oauth apps")
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

func (s *service) getCachedClient(ctx context.Context, clientid string) (*model.Model, error) {
	if clientstr, err := s.kvclient.Get(ctx, clientid); err != nil {
		if !errors.Is(err, kvstore.ErrNotFound{}) {
			s.logger.Error("Failed to get oauth client from cache", map[string]string{
				"error":      err.Error(),
				"actiontype": "oauth_get_cache_client",
			})
		}
	} else if clientstr == cacheValTombstone {
		return nil, kerrors.WithKind(err, ErrNotFound{}, "OAuth app not found")
	} else {
		cm := &model.Model{}
		if err := json.Unmarshal([]byte(clientstr), cm); err != nil {
			s.logger.Error("Malformed oauth client cache json", map[string]string{
				"error":      err.Error(),
				"actiontype": "oauth_unmarshal_client_json",
			})
		} else {
			return cm, nil
		}
	}

	m, err := s.apps.GetByID(ctx, clientid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			if err := s.kvclient.Set(ctx, clientid, cacheValTombstone, s.keyCacheTime); err != nil {
				s.logger.Error("Failed to set oauth client in cache", map[string]string{
					"error":      err.Error(),
					"actiontype": "oauth_set_cache_client",
				})
			}
			return nil, kerrors.WithKind(err, ErrNotFound{}, "OAuth app not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get oauth app")
	}

	if clientbytes, err := json.Marshal(m); err != nil {
		s.logger.Error("Failed to marshal client to json", map[string]string{
			"error":      err.Error(),
			"actiontype": "oauth_marshal_client_json",
		})
	} else if err := s.kvclient.Set(ctx, clientid, string(clientbytes), s.keyCacheTime); err != nil {
		s.logger.Error("Failed to set oauth client in cache", map[string]string{
			"error":      err.Error(),
			"actiontype": "oauth_set_cache_client",
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

func (s *service) CreateApp(ctx context.Context, name, url, redirectURI, creatorID string) (*resCreate, error) {
	m, key, err := s.apps.New(name, url, redirectURI, creatorID)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to create oauth app")
	}
	if err := s.apps.Insert(ctx, m); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to insert oauth app")
	}
	return &resCreate{
		ClientID: m.ClientID,
		Key:      key,
	}, nil
}

func (s *service) RotateAppKey(ctx context.Context, clientid string) (*resCreate, error) {
	m, err := s.apps.GetByID(ctx, clientid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "OAuth app not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get oauth app")
	}
	key, err := s.apps.RehashKey(ctx, m)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to rotate client key")
	}
	s.clearCache(clientid)
	return &resCreate{
		ClientID: clientid,
		Key:      key,
	}, nil
}

func (s *service) UpdateApp(ctx context.Context, clientid string, name, url, redirectURI string) error {
	m, err := s.apps.GetByID(ctx, clientid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "OAuth app not found")
		}
		return kerrors.WithMsg(err, "Failed to get oauth app")
	}
	m.Name = name
	m.URL = url
	m.RedirectURI = redirectURI
	if err := s.apps.UpdateProps(ctx, m); err != nil {
		return kerrors.WithMsg(err, "Failed to update oauth app")
	}
	s.clearCache(clientid)
	return nil
}

const (
	imgSize      = 256
	thumbSize    = 8
	thumbQuality = 0
)

func (s *service) UpdateLogo(ctx context.Context, clientid string, img image.Image) error {
	m, err := s.apps.GetByID(ctx, clientid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "OAuth app not found")
		}
		return kerrors.WithMsg(err, "Failed to get oauth app")
	}

	img.ResizeFill(imgSize, imgSize)
	thumb := img.Duplicate()
	thumb.ResizeLimit(thumbSize, thumbSize)
	thumb64, err := thumb.ToBase64(thumbQuality)
	if err != nil {
		return kerrors.WithMsg(err, "Failed to encode thumbnail to base64")
	}
	imgpng, err := img.ToPng(image.PngBest)
	if err != nil {
		return kerrors.WithMsg(err, "Failed to encode image to png")
	}
	if err := s.logoImgDir.Put(ctx, m.ClientID, image.MediaTypePng, int64(imgpng.Len()), nil, imgpng); err != nil {
		return kerrors.WithMsg(err, "Failed to save app logo")
	}

	m.Logo = thumb64
	if err := s.apps.UpdateProps(ctx, m); err != nil {
		return kerrors.WithMsg(err, "Failed to update oauth app")
	}
	s.clearCache(clientid)
	return nil
}

func (s *service) Delete(ctx context.Context, clientid string) error {
	m, err := s.apps.GetByID(ctx, clientid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "OAuth app not found")
		}
		return kerrors.WithMsg(err, "Failed to get oauth app")
	}

	if err := s.logoImgDir.Del(ctx, clientid); err != nil {
		if !errors.Is(err, objstore.ErrNotFound{}) {
			return kerrors.WithMsg(err, "Unable to delete app logo")
		}
	}

	if err := s.apps.Delete(ctx, m); err != nil {
		return kerrors.WithMsg(err, "Failed to delete oauth app")
	}
	s.clearCache(clientid)
	return nil
}

func (s *service) GetApp(ctx context.Context, clientid string) (*resApp, error) {
	m, err := s.getCachedClient(ctx, clientid)
	if err != nil {
		if errors.Is(err, ErrNotFound{}) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "OAuth app not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get oauth app")
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

func (s *service) StatLogo(ctx context.Context, clientid string) (*objstore.ObjectInfo, error) {
	objinfo, err := s.logoImgDir.Stat(ctx, clientid)
	if err != nil {
		if errors.Is(err, objstore.ErrNotFound{}) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "OAuth app logo not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get oauth app logo")
	}
	return objinfo, nil
}

func (s *service) GetLogo(ctx context.Context, clientid string) (io.ReadCloser, string, error) {
	obj, objinfo, err := s.logoImgDir.Get(ctx, clientid)
	if err != nil {
		if errors.Is(err, objstore.ErrNotFound{}) {
			return nil, "", governor.ErrWithRes(err, http.StatusNotFound, "", "OAuth app logo not found")
		}
		return nil, "", kerrors.WithMsg(err, "Failed to get app logo")
	}
	return obj, objinfo.ContentType, nil
}

func (s *service) clearCache(clientid string) {
	// must make a best effort attempt to clear the cache
	if err := s.kvclient.Del(context.Background(), clientid); err != nil {
		s.logger.Error("Failed to clear oauth client from cache", map[string]string{
			"error":      err.Error(),
			"actiontype": "oauth_clear_cache_client",
		})
	}
}
