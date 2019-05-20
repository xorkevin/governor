package courier

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/barcode"
	"io"
	"net/http"
	"time"
)

const (
	cachePrefixCourierLink = "courier.courierlink:"
	mediaTypePNG           = "image/png"
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
func (c *courierService) GetLink(linkid string) (*resGetLink, error) {
	m, err := c.repo.GetLink(linkid)
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

func (c *courierService) GetLinkFast(linkid string) (string, error) {
	cacheLinkID := cachePrefixCourierLink + linkid
	if cachedURL, err := c.cache.Cache().Get(cacheLinkID).Result(); err == nil {
		return cachedURL, nil
	}
	res, err := c.GetLink(linkid)
	if err != nil {
		return "", err
	}
	if err := c.cache.Cache().Set(cacheLinkID, res.URL, time.Duration(c.cacheTime*b1)).Err(); err != nil {
		c.logger.Error("Fail cache linkid url", nil)
	}
	return res.URL, nil
}

func (c *courierService) GetLinkImage(linkid string) (io.Reader, string, error) {
	qrimage, objinfo, err := c.linkImageBucket.Get(linkid + "-qr")
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return nil, "", governor.NewErrorUser("Link qr code image does not exist", http.StatusNotFound, err)
		}
		return nil, "Failed to get the link qr code image", err
	}
	return qrimage, objinfo.ContentType, nil
}

type (
	resLinkGroup struct {
		Links []resGetLink `json:"links"`
	}
)

// GetLinkGroup retrieves a group of links
func (c *courierService) GetLinkGroup(limit, offset int, creatorid string) (*resLinkGroup, error) {
	links, err := c.repo.GetLinkGroup(limit, offset, creatorid)
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
func (c *courierService) CreateLink(linkid, url, creatorid string) (*resCreateLink, error) {
	m := c.repo.NewLinkEmptyPtr()
	if len(linkid) == 0 {
		ml, err := c.repo.NewLinkAuto(url, creatorid)
		if err != nil {
			return nil, err
		}
		m = ml
	} else {
		ml, err := c.repo.NewLink(linkid, url, creatorid)
		if err != nil {
			return nil, err
		}
		m = ml
	}
	if err := c.repo.InsertLink(m); err != nil {
		if governor.ErrorStatus(err) == http.StatusBadRequest {
			return nil, governor.NewErrorUser("", 0, err)
		}
		return nil, err
	}
	qrimage, err := c.barcode.GenerateBarcode(barcode.TransportQRCode, c.linkPrefix+"/"+linkid)
	if err != nil {
		return nil, governor.NewError("Failed to generate qr code image", http.StatusInternalServerError, err)
	}
	if err := c.linkImageBucket.Put(linkid+"-qr", mediaTypePNG, int64(qrimage.Len()), qrimage); err != nil {
		c.logger.Error("fail add link qrcode image to objstore", map[string]string{
			"linkid": linkid,
			"err":    err.Error(),
		})
		return nil, governor.NewError("Failed to put qr code image in objstore", http.StatusInternalServerError, err)
	}

	return &resCreateLink{
		LinkID: m.LinkID,
	}, nil
}

// DeleteLink deletes a link
func (c *courierService) DeleteLink(linkid string) error {
	m, err := c.repo.GetLink(linkid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return governor.NewErrorUser("", 0, err)
		}
		return err
	}
	if err := c.repo.DeleteLink(m); err != nil {
		return err
	}
	if err := c.linkImageBucket.Remove(linkid + "-qr"); err != nil {
		return governor.NewError("Failed to remove qr code image from objstore", http.StatusInternalServerError, err)
	}
	return nil
}
