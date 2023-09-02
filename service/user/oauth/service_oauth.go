package oauth

import (
	"context"
	"errors"
	"io"
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/dbsql"
	"xorkevin.dev/governor/service/image"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/objstore"
	"xorkevin.dev/governor/service/user/oauth/oauthappmodel"
	"xorkevin.dev/governor/util/kjson"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	// errNotFound is returned when an oauth app is not found
	errNotFound struct{}
)

func (e errNotFound) Error() string {
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

func (s *Service) getApps(ctx context.Context, limit, offset int, creatorid string) (*resApps, error) {
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

func (s *Service) getAppsBulk(ctx context.Context, clientids []string) (*resApps, error) {
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

func (s *Service) getCachedClient(ctx context.Context, clientid string) (*oauthappmodel.Model, error) {
	if clientstr, err := s.kvclient.Get(ctx, clientid); err != nil {
		if !errors.Is(err, kvstore.ErrNotFound) {
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to get oauth client from cache"))
		}
	} else if clientstr == cacheValTombstone {
		return nil, kerrors.WithKind(err, errNotFound{}, "OAuth app not found")
	} else {
		cm := &oauthappmodel.Model{}
		if err := kjson.Unmarshal([]byte(clientstr), cm); err != nil {
			s.log.Err(ctx, kerrors.WithMsg(err, "Malformed oauth client cache json"))
		} else {
			return cm, nil
		}
	}

	m, err := s.apps.GetByID(ctx, clientid)
	if err != nil {
		if errors.Is(err, dbsql.ErrNotFound) {
			if err := s.kvclient.Set(ctx, clientid, cacheValTombstone, s.keyCache); err != nil {
				s.log.Err(ctx, kerrors.WithMsg(err, "Failed to set oauth client in cache"))
			}
			return nil, kerrors.WithKind(err, errNotFound{}, "OAuth app not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get oauth app")
	}

	if clientbytes, err := kjson.Marshal(m); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to marshal client to json"))
	} else if err := s.kvclient.Set(ctx, clientid, string(clientbytes), s.keyCache); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to set oauth client in cache"))
	}

	return m, nil
}

type (
	resCreate struct {
		ClientID string `json:"client_id"`
		Key      string `json:"key"`
	}
)

func (s *Service) createApp(ctx context.Context, name, url, redirectURI, creatorID string) (*resCreate, error) {
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

func (s *Service) rotateAppKey(ctx context.Context, clientid string) (*resCreate, error) {
	m, err := s.apps.GetByID(ctx, clientid)
	if err != nil {
		if errors.Is(err, dbsql.ErrNotFound) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "OAuth app not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get oauth app")
	}
	key, err := s.apps.RehashKey(ctx, m)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to rotate client key")
	}
	// must make a best effort attempt to clear the cache
	ctx = klog.ExtendCtx(context.Background(), ctx)
	s.clearCache(ctx, clientid)
	return &resCreate{
		ClientID: clientid,
		Key:      key,
	}, nil
}

func (s *Service) updateApp(ctx context.Context, clientid string, name, url, redirectURI string) error {
	m, err := s.apps.GetByID(ctx, clientid)
	if err != nil {
		if errors.Is(err, dbsql.ErrNotFound) {
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
	// must make a best effort attempt to clear the cache
	ctx = klog.ExtendCtx(context.Background(), ctx)
	s.clearCache(ctx, clientid)
	return nil
}

const (
	imgSize      = 256
	thumbSize    = 8
	thumbQuality = 0
)

func (s *Service) updateLogo(ctx context.Context, clientid string, img image.Image) error {
	m, err := s.apps.GetByID(ctx, clientid)
	if err != nil {
		if errors.Is(err, dbsql.ErrNotFound) {
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
	// must make a best effort attempt to clear the cache
	ctx = klog.ExtendCtx(context.Background(), ctx)
	s.clearCache(ctx, clientid)
	return nil
}

func (s *Service) deleteApp(ctx context.Context, clientid string) error {
	m, err := s.apps.GetByID(ctx, clientid)
	if err != nil {
		if errors.Is(err, dbsql.ErrNotFound) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "OAuth app not found")
		}
		return kerrors.WithMsg(err, "Failed to get oauth app")
	}

	if err := s.logoImgDir.Del(ctx, clientid); err != nil {
		if !errors.Is(err, objstore.ErrNotFound) {
			return kerrors.WithMsg(err, "Unable to delete app logo")
		}
	}

	if err := s.apps.Delete(ctx, m); err != nil {
		return kerrors.WithMsg(err, "Failed to delete oauth app")
	}
	// must make a best effort attempt to clear the cache
	ctx = klog.ExtendCtx(context.Background(), ctx)
	s.clearCache(ctx, clientid)
	return nil
}

func (s *Service) getApp(ctx context.Context, clientid string) (*resApp, error) {
	m, err := s.getCachedClient(ctx, clientid)
	if err != nil {
		if errors.Is(err, errNotFound{}) {
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

func (s *Service) statLogo(ctx context.Context, clientid string) (*objstore.ObjectInfo, error) {
	objinfo, err := s.logoImgDir.Stat(ctx, clientid)
	if err != nil {
		if errors.Is(err, objstore.ErrNotFound) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "OAuth app logo not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get oauth app logo")
	}
	return objinfo, nil
}

func (s *Service) getLogo(ctx context.Context, clientid string) (io.ReadCloser, string, error) {
	obj, objinfo, err := s.logoImgDir.Get(ctx, clientid)
	if err != nil {
		if errors.Is(err, objstore.ErrNotFound) {
			return nil, "", governor.ErrWithRes(err, http.StatusNotFound, "", "OAuth app logo not found")
		}
		return nil, "", kerrors.WithMsg(err, "Failed to get app logo")
	}
	return obj, objinfo.ContentType, nil
}

func (s *Service) clearCache(ctx context.Context, clientid string) {
	if err := s.kvclient.Del(ctx, clientid); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to clear oauth client from cache"))
	}
}
