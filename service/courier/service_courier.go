package courier

import (
	"io"
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/barcode"
	"xorkevin.dev/governor/service/courier/model"
	"xorkevin.dev/governor/service/image"
	"xorkevin.dev/governor/service/objstore"
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
func (s *service) GetLink(linkid string) (*resGetLink, error) {
	m, err := s.repo.GetLink(linkid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return nil, governor.NewErrorUser("", 0, err)
		}
		return nil, err
	}
	return &resGetLink{
		LinkID:       m.LinkID,
		URL:          m.URL,
		CreatorID:    m.CreatorID,
		CreationTime: m.CreationTime,
	}, nil
}

func (s *service) GetLinkFast(linkid string) (string, error) {
	if cachedURL, err := s.kvlinks.Get(linkid); err != nil {
		if governor.ErrorStatus(err) != http.StatusNotFound {
			s.logger.Error("Failed to get linkid url from cache", map[string]string{
				"error":      err.Error(),
				"actiontype": "getcachelink",
			})
		}
	} else if cachedURL == cacheValTombstone {
		return "", governor.NewErrorUser("No link found with that id", http.StatusNotFound, nil)
	} else {
		return cachedURL, nil
	}
	res, err := s.GetLink(linkid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			if err := s.kvlinks.Set(linkid, cacheValTombstone, s.cacheTime); err != nil {
				s.logger.Error("Failed to cache linkid url", map[string]string{
					"linkid":     linkid,
					"error":      err.Error(),
					"actiontype": "setcachelink",
				})
			}
		}
		return "", err
	}
	if err := s.kvlinks.Set(linkid, res.URL, s.cacheTime); err != nil {
		s.logger.Error("Failed to cache linkid url", map[string]string{
			"linkid":     linkid,
			"error":      err.Error(),
			"actiontype": "setcachelink",
		})
	}
	return res.URL, nil
}

func (s *service) StatLinkImage(linkid string) (*objstore.ObjectInfo, error) {
	objinfo, err := s.linkImgDir.Stat(linkid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return nil, governor.NewErrorUser("Link qr code image not found", http.StatusNotFound, err)
		}
		return nil, governor.NewError("Failed to get link qr code image", http.StatusInternalServerError, err)
	}
	return objinfo, nil
}

func (s *service) GetLinkImage(linkid string) (io.ReadCloser, string, error) {
	qrimg, objinfo, err := s.linkImgDir.Get(linkid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return nil, "", governor.NewErrorUser("Link qr code image not found", http.StatusNotFound, err)
		}
		return nil, "", governor.NewError("Failed to get link qr code image", http.StatusInternalServerError, err)
	}
	return qrimg, objinfo.ContentType, nil
}

type (
	resLinkGroup struct {
		Links []resGetLink `json:"links"`
	}
)

// GetLinkGroup retrieves a group of links
func (s *service) GetLinkGroup(creatorid string, limit, offset int) (*resLinkGroup, error) {
	links, err := s.repo.GetLinkGroup(creatorid, limit, offset)
	if err != nil {
		return nil, err
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
	qrScale = 12
)

type (
	resCreateLink struct {
		LinkID string `json:"linkid"`
	}
)

// CreateLink creates a new link
func (s *service) CreateLink(creatorid, linkid, url, brandid string) (*resCreateLink, error) {
	var m *model.LinkModel
	if len(linkid) == 0 {
		var err error
		m, err = s.repo.NewLinkAuto(creatorid, url)
		if err != nil {
			return nil, err
		}
	} else {
		m = s.repo.NewLink(creatorid, linkid, url)
	}

	if brandid != "" {
		objinfo, err := s.brandImgDir.Subdir(creatorid).Stat(brandid)
		if err != nil {
			if governor.ErrorStatus(err) == http.StatusNotFound {
				return nil, governor.NewErrorUser("Brand image not found", http.StatusNotFound, err)
			}
			return nil, governor.NewError("Failed to get brand image", http.StatusInternalServerError, err)
		}
		if objinfo.ContentType != image.MediaTypePng {
			return nil, governor.NewErrorUser("Invalid brand image media type", http.StatusBadRequest, err)
		}
	}

	if err := s.repo.InsertLink(m); err != nil {
		if governor.ErrorStatus(err) == http.StatusBadRequest {
			return nil, governor.NewErrorUser("", 0, err)
		}
		return nil, err
	}
	qrimg, err := barcode.GenerateQR(s.linkPrefix+"/"+m.LinkID, barcode.QRECHigh, qrScale)
	if err != nil {
		return nil, governor.NewError("Failed to generate qr code image", http.StatusInternalServerError, err)
	}

	if brandid != "" {
		brandimg, _, err := s.brandImgDir.Subdir(creatorid).Get(brandid)
		if err != nil {
			return nil, governor.NewError("Failed to get brand image", http.StatusInternalServerError, err)
		}
		defer func() {
			if err := brandimg.Close(); err != nil {
				s.logger.Error("Failed to close brand image", map[string]string{
					"actiontype": "getlinkbrandimage",
					"error":      err.Error(),
				})
			}
		}()
		brand, err := image.FromPng(brandimg)
		if err != nil {
			return nil, governor.NewError("Failed to parse brand image", http.StatusInternalServerError, err)
		}
		size := qrimg.Size()
		w := size.W / 3
		h := size.H / 3
		brand.ResizeFit(w, h)
		qrimg.Draw(brand, w, h, true)
	}

	qrpng, err := qrimg.ToPng(image.PngBest)
	if err != nil {
		return nil, governor.NewError("Failed to encode qr code image", http.StatusInternalServerError, err)
	}
	if err := s.linkImgDir.Put(m.LinkID, image.MediaTypePng, int64(qrpng.Len()), qrpng); err != nil {
		return nil, governor.NewError("Failed to save qr code image", http.StatusInternalServerError, err)
	}

	return &resCreateLink{
		LinkID: m.LinkID,
	}, nil
}

// DeleteLink deletes a link
func (s *service) DeleteLink(creatorid, linkid string) error {
	m, err := s.repo.GetLink(linkid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return governor.NewErrorUser("", 0, err)
		}
		return err
	}
	if m.CreatorID != creatorid {
		return governor.NewErrorUser("No link found with that id", http.StatusNotFound, nil)
	}
	if err := s.linkImgDir.Del(linkid); err != nil {
		return governor.NewError("Failed to delete qr code image", http.StatusInternalServerError, err)
	}
	if err := s.repo.DeleteLink(m); err != nil {
		return err
	}
	if err := s.kvlinks.Del(linkid); err != nil {
		s.logger.Error("failed to delete linkid url", map[string]string{
			"linkid":     linkid,
			"error":      err.Error(),
			"actiontype": "linkcache",
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
func (s *service) GetBrandGroup(creatorid string, limit, offset int) (*resBrandGroup, error) {
	brands, err := s.repo.GetBrandGroup(creatorid, limit, offset)
	if err != nil {
		return nil, err
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

func (s *service) StatBrandImage(creatorid, brandid string) (*objstore.ObjectInfo, error) {
	objinfo, err := s.brandImgDir.Subdir(creatorid).Stat(brandid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return nil, governor.NewErrorUser("Brand image not found", http.StatusNotFound, err)
		}
		return nil, governor.NewError("Failed to get brand image", http.StatusInternalServerError, err)
	}
	return objinfo, nil
}

func (s *service) GetBrandImage(creatorid, brandid string) (io.ReadCloser, string, error) {
	brandimg, objinfo, err := s.brandImgDir.Subdir(creatorid).Get(brandid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return nil, "", governor.NewErrorUser("Brand image not found", http.StatusNotFound, err)
		}
		return nil, "", governor.NewError("Failed to get brand image", http.StatusInternalServerError, err)
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
func (s *service) CreateBrand(creatorid, brandid string, img image.Image) (*resCreateBrand, error) {
	m := s.repo.NewBrand(creatorid, brandid)
	if err := s.repo.InsertBrand(m); err != nil {
		if governor.ErrorStatus(err) == http.StatusBadRequest {
			return nil, governor.NewErrorUser("", 0, err)
		}
		return nil, err
	}
	img.ResizeFill(256, 256)
	imgpng, err := img.ToPng(image.PngBest)
	if err != nil {
		return nil, governor.NewError("Failed to encode png image", http.StatusInternalServerError, err)
	}
	if err := s.brandImgDir.Subdir(creatorid).Put(m.BrandID, image.MediaTypePng, int64(imgpng.Len()), imgpng); err != nil {
		return nil, governor.NewError("Failed to save image", http.StatusInternalServerError, err)
	}
	return &resCreateBrand{
		CreatorID: creatorid,
		BrandID:   brandid,
	}, nil
}

// DeleteBrand removes a brand image
func (s *service) DeleteBrand(creatorid, brandid string) error {
	m, err := s.repo.GetBrand(creatorid, brandid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return governor.NewErrorUser("", 0, err)
		}
		return err
	}
	if err := s.brandImgDir.Del(brandid); err != nil {
		return governor.NewError("Failed to delete brand image", http.StatusInternalServerError, err)
	}
	if err := s.repo.DeleteBrand(m); err != nil {
		return err
	}
	return nil
}
