package courier

import (
	"io"
	"net/http"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/barcode"
	"xorkevin.dev/governor/service/image"
	"xorkevin.dev/governor/service/objstore"
)

const (
	mediaTypePNG = "image/png"
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
	if cachedURL, err := s.kvlinks.Get(linkid); err == nil {
		return cachedURL, nil
	}
	res, err := s.GetLink(linkid)
	if err != nil {
		return "", err
	}
	if err := s.kvlinks.Set(linkid, res.URL, s.cacheTime); err != nil {
		s.logger.Error("failed to cache linkid url", map[string]string{
			"linkid":     linkid,
			"error":      err.Error(),
			"actiontype": "linkcache",
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
	qrimage, objinfo, err := s.linkImgDir.Get(linkid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return nil, "", governor.NewErrorUser("Link qr code image not found", http.StatusNotFound, err)
		}
		return nil, "", governor.NewError("Failed to get link qr code image", http.StatusInternalServerError, err)
	}
	return qrimage, objinfo.ContentType, nil
}

type (
	resLinkGroup struct {
		Links []resGetLink `json:"links"`
	}
)

// GetLinkGroup retrieves a group of links
func (s *service) GetLinkGroup(limit, offset int, creatorid string) (*resLinkGroup, error) {
	links, err := s.repo.GetLinkGroup(limit, offset, creatorid)
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

type (
	resCreateLink struct {
		LinkID string `json:"linkid"`
	}
)

// CreateLink creates a new link
func (s *service) CreateLink(linkid, url, creatorid string) (*resCreateLink, error) {
	m := s.repo.NewLinkEmptyPtr()
	if len(linkid) == 0 {
		ml, err := s.repo.NewLinkAuto(url, creatorid)
		if err != nil {
			return nil, err
		}
		m = ml
	} else {
		ml, err := s.repo.NewLink(linkid, url, creatorid)
		if err != nil {
			return nil, err
		}
		m = ml
	}
	if err := s.repo.InsertLink(m); err != nil {
		if governor.ErrorStatus(err) == http.StatusBadRequest {
			return nil, governor.NewErrorUser("", 0, err)
		}
		return nil, err
	}
	qrimage, err := barcode.GenerateQR(s.linkPrefix+"/"+m.LinkID, barcode.QRECHigh)
	if err != nil {
		return nil, governor.NewError("Failed to generate qr code image", http.StatusInternalServerError, err)
	}
	qrpng, err := qrimage.ToPng(image.PngBest)
	if err != nil {
		return nil, governor.NewError("Failed to encode qr code image", http.StatusInternalServerError, err)
	}
	if err := s.linkImgDir.Put(m.LinkID, mediaTypePNG, int64(qrpng.Len()), qrpng); err != nil {
		return nil, governor.NewError("Failed to save qr code image", http.StatusInternalServerError, err)
	}

	return &resCreateLink{
		LinkID: m.LinkID,
	}, nil
}

// DeleteLink deletes a link
func (s *service) DeleteLink(linkid string) error {
	m, err := s.repo.GetLink(linkid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return governor.NewErrorUser("", 0, err)
		}
		return err
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
	if err := s.linkImgDir.Del(linkid); err != nil {
		return governor.NewError("Failed to delete qr code image", http.StatusInternalServerError, err)
	}
	return nil
}
