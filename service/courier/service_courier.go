package courier

import (
	"context"
	"errors"
	"io"
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/barcode"
	"xorkevin.dev/governor/service/courier/model"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/image"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/objstore"
	"xorkevin.dev/kerrors"
)

const (
	cacheValTombstone = "-"
)

type (
	resGetLink struct {
		LinkID       string `json:"linkid"`
		URL          string `json:"url"`
		CreatorID    string `json:"creatorid"`
		CreationTime int64  `json:"creation_time"`
	}
)

// GetLink retrieves a link by id
func (s *service) GetLink(ctx context.Context, linkid string) (*resGetLink, error) {
	m, err := s.repo.GetLink(ctx, linkid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "Link not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get link")
	}
	return &resGetLink{
		LinkID:       m.LinkID,
		URL:          m.URL,
		CreatorID:    m.CreatorID,
		CreationTime: m.CreationTime,
	}, nil
}

func (s *service) GetLinkFast(ctx context.Context, linkid string) (string, error) {
	if cachedURL, err := s.kvlinks.Get(ctx, linkid); err != nil {
		if !errors.Is(err, kvstore.ErrNotFound{}) {
			s.logger.Error("Failed to get linkid url from cache", map[string]string{
				"error":      err.Error(),
				"actiontype": "courier_get_cache_link",
			})
		}
	} else if cachedURL == cacheValTombstone {
		return "", governor.ErrWithRes(nil, http.StatusNotFound, "", "Link not found")
	} else {
		return cachedURL, nil
	}
	res, err := s.repo.GetLink(ctx, linkid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			if err := s.kvlinks.Set(ctx, linkid, cacheValTombstone, s.cacheTime); err != nil {
				s.logger.Error("Failed to cache linkid url", map[string]string{
					"error":      err.Error(),
					"actiontype": "courier_set_cache_link",
				})
			}
			return "", governor.ErrWithRes(err, http.StatusNotFound, "", "Link not found")
		}
		return "", kerrors.WithMsg(err, "Failed to get link")
	}
	if err := s.kvlinks.Set(ctx, linkid, res.URL, s.cacheTime); err != nil {
		s.logger.Error("Failed to cache linkid url", map[string]string{
			"linkid":     linkid,
			"error":      err.Error(),
			"actiontype": "courier_set_cache_link",
		})
	}
	return res.URL, nil
}

func (s *service) StatLinkImage(ctx context.Context, linkid string) (*objstore.ObjectInfo, error) {
	objinfo, err := s.linkImgDir.Stat(ctx, linkid)
	if err != nil {
		if errors.Is(err, objstore.ErrNotFound{}) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "Link qr code image not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get link qr code image")
	}
	return objinfo, nil
}

func (s *service) GetLinkImage(ctx context.Context, linkid string) (io.ReadCloser, string, error) {
	qrimg, objinfo, err := s.linkImgDir.Get(ctx, linkid)
	if err != nil {
		if errors.Is(err, objstore.ErrNotFound{}) {
			return nil, "", governor.ErrWithRes(err, http.StatusNotFound, "", "Link qr code image not found")
		}
		return nil, "", kerrors.WithMsg(err, "Failed to get link qr code image")
	}
	return qrimg, objinfo.ContentType, nil
}

type (
	resLinkGroup struct {
		Links []resGetLink `json:"links"`
	}
)

// GetLinkGroup retrieves a group of links
func (s *service) GetLinkGroup(ctx context.Context, creatorid string, limit, offset int) (*resLinkGroup, error) {
	links, err := s.repo.GetLinkGroup(ctx, creatorid, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get links")
	}
	res := make([]resGetLink, 0, len(links))
	for _, i := range links {
		res = append(res, resGetLink{
			LinkID:       i.LinkID,
			URL:          i.URL,
			CreatorID:    i.CreatorID,
			CreationTime: i.CreationTime,
		})
	}
	return &resLinkGroup{
		Links: res,
	}, nil
}

const (
	qrScale = 8
)

type (
	resCreateLink struct {
		LinkID string `json:"linkid"`
	}
)

// CreateLink creates a new link
func (s *service) CreateLink(ctx context.Context, creatorid, linkid, url, brandid string) (*resCreateLink, error) {
	var m *model.LinkModel
	if len(linkid) == 0 {
		var err error
		m, err = s.repo.NewLinkAuto(creatorid, url)
		if err != nil {
			return nil, kerrors.WithMsg(err, "Failed to generate link id")
		}
	} else {
		m = s.repo.NewLink(creatorid, linkid, url)
	}

	var brand image.Image
	if brandid != "" {
		if err := func() error {
			brandimg, objinfo, err := s.brandImgDir.Subdir(creatorid).Get(ctx, brandid)
			if err != nil {
				return kerrors.WithMsg(err, "Failed to get brand image")
			}
			defer func() {
				if err := brandimg.Close(); err != nil {
					s.logger.Error("Failed to close brand image", map[string]string{
						"error":      err.Error(),
						"actiontype": "courier_close_link_brand_image",
					})
				}
			}()
			if objinfo.ContentType != image.MediaTypePng {
				return kerrors.WithMsg(err, "Invalid brand image media type")
			}
			brand, err = image.FromPng(brandimg)
			if err != nil {
				return kerrors.WithMsg(err, "Failed to parse brand image")
			}
			return nil
		}(); err != nil {
			return nil, err
		}
	}

	qrimg, err := barcode.GenerateQR(s.linkPrefix+"/"+m.LinkID, barcode.QRECHigh, qrScale)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to generate qr code image")
	}

	if brand != nil {
		size := qrimg.Size()
		w := size.W / 3
		h := size.H / 3
		brand.ResizeFit(w, h)
		qrimg.Draw(brand, w, h, true)
	}

	qrpng, err := qrimg.ToPng(image.PngBest)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to encode qr code image")
	}

	if err := s.repo.InsertLink(ctx, m); err != nil {
		if errors.Is(err, db.ErrUnique{}) {
			return nil, governor.ErrWithRes(err, http.StatusBadRequest, "", "Link id already taken")
		}
		return nil, kerrors.WithMsg(err, "Failed to create link")
	}
	if err := s.linkImgDir.Put(ctx, m.LinkID, image.MediaTypePng, int64(qrpng.Len()), nil, qrpng); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to save qr code image")
	}

	return &resCreateLink{
		LinkID: m.LinkID,
	}, nil
}

// DeleteLink deletes a link
func (s *service) DeleteLink(ctx context.Context, creatorid, linkid string) error {
	m, err := s.repo.GetLink(ctx, linkid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "Link not found")
		}
		return kerrors.WithMsg(err, "Failed to get link")
	}
	if m.CreatorID != creatorid {
		return governor.ErrWithRes(nil, http.StatusNotFound, "", "Link not found")
	}
	if err := s.linkImgDir.Del(ctx, linkid); err != nil {
		if !errors.Is(err, objstore.ErrNotFound{}) {
			return kerrors.WithMsg(err, "Failed to delete qr code image")
		}
	}
	if err := s.repo.DeleteLink(ctx, m); err != nil {
		return kerrors.WithMsg(err, "Failed to delete link")
	}
	// must give a best effort attempt to clear the cache
	if err := s.kvlinks.Del(context.Background(), linkid); err != nil {
		s.logger.Error("Failed to delete linkid url", map[string]string{
			"error":      err.Error(),
			"actiontype": "courier_clear_link_cache",
			"linkid":     linkid,
		})
	}
	return nil
}

type (
	resGetBrand struct {
		BrandID      string `json:"brandid"`
		CreatorID    string `json:"creatorid"`
		CreationTime int64  `json:"creation_time"`
	}

	resBrandGroup struct {
		Brands []resGetBrand `json:"brands"`
	}
)

// GetBrandGroup gets a list of brand images
func (s *service) GetBrandGroup(ctx context.Context, creatorid string, limit, offset int) (*resBrandGroup, error) {
	brands, err := s.repo.GetBrandGroup(ctx, creatorid, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get links")
	}
	res := make([]resGetBrand, 0, len(brands))
	for _, i := range brands {
		res = append(res, resGetBrand{
			BrandID:      i.BrandID,
			CreatorID:    i.CreatorID,
			CreationTime: i.CreationTime,
		})
	}
	return &resBrandGroup{
		Brands: res,
	}, nil
}

func (s *service) StatBrandImage(ctx context.Context, creatorid, brandid string) (*objstore.ObjectInfo, error) {
	objinfo, err := s.brandImgDir.Subdir(creatorid).Stat(ctx, brandid)
	if err != nil {
		if errors.Is(err, objstore.ErrNotFound{}) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "Brand image not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get brand image")
	}
	return objinfo, nil
}

func (s *service) GetBrandImage(ctx context.Context, creatorid, brandid string) (io.ReadCloser, string, error) {
	brandimg, objinfo, err := s.brandImgDir.Subdir(creatorid).Get(ctx, brandid)
	if err != nil {
		if errors.Is(err, objstore.ErrNotFound{}) {
			return nil, "", governor.ErrWithRes(err, http.StatusNotFound, "", "Brand image not found")
		}
		return nil, "", kerrors.WithMsg(err, "Failed to get brand image")
	}
	return brandimg, objinfo.ContentType, nil
}

type (
	resCreateBrand struct {
		CreatorID string `json:"creatorid"`
		BrandID   string `json:"brandid"`
	}
)

// CreateBrand adds a brand image
func (s *service) CreateBrand(ctx context.Context, creatorid, brandid string, img image.Image) (*resCreateBrand, error) {
	m := s.repo.NewBrand(creatorid, brandid)
	if err := s.repo.InsertBrand(ctx, m); err != nil {
		if errors.Is(err, db.ErrUnique{}) {
			return nil, governor.ErrWithRes(err, http.StatusBadRequest, "", "Brand name must be unique")
		}
		return nil, kerrors.WithMsg(err, "Failed to create brand")
	}
	img.ResizeFill(256, 256)
	imgpng, err := img.ToPng(image.PngBest)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to encode png image")
	}
	if err := s.brandImgDir.Subdir(creatorid).Put(ctx, m.BrandID, image.MediaTypePng, int64(imgpng.Len()), nil, imgpng); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to save image")
	}
	return &resCreateBrand{
		CreatorID: creatorid,
		BrandID:   brandid,
	}, nil
}

// DeleteBrand removes a brand image
func (s *service) DeleteBrand(ctx context.Context, creatorid, brandid string) error {
	m, err := s.repo.GetBrand(ctx, creatorid, brandid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "Brand image not found")
		}
		return kerrors.WithMsg(err, "Failed to delete brand")
	}
	if err := s.brandImgDir.Del(ctx, brandid); err != nil {
		if !errors.Is(err, objstore.ErrNotFound{}) {
			return kerrors.WithMsg(err, "Failed to delete brand image")
		}
	}
	if err := s.repo.DeleteBrand(ctx, m); err != nil {
		return err
	}
	return nil
}
