package courier

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/barcode"
	"io"
	"time"
)

const (
	cachePrefixCourierLink = moduleID + ".courierlink:"
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
func (c *courierService) GetLink(linkid string) (*resGetLink, *governor.Error) {
	m, err := c.repo.GetLink(linkid)
	if err != nil {
		err.AddTrace(moduleID)
		return nil, err
	}
	return &resGetLink{
		LinkID:       m.LinkID,
		URL:          m.URL,
		CreatorID:    m.CreatorID,
		CreationTime: m.CreationTime,
	}, nil
}

func (c *courierService) GetLinkFast(linkid string) (string, *governor.Error) {
	cacheLinkID := cachePrefixCourierLink + linkid
	if cachedURL, err := c.cache.Cache().Get(cacheLinkID).Result(); err == nil {
		return cachedURL, nil
	}
	res, err := c.GetLink(linkid)
	if err != nil {
		return "", err
	}
	if err := c.cache.Cache().Set(cacheLinkID, res.URL, time.Duration(c.cacheTime*b1)).Err(); err != nil {
		c.logger.Error(err.Error(), moduleID, "fail cache linkid url", 0, nil)
	}
	return res.URL, nil
}

func (c *courierService) GetLinkImage(linkid string) (io.Reader, string, *governor.Error) {
	qrimage, objinfo, err := c.linkImageBucket.Get(linkid + "-qr")
	if err != nil {
		if err.Code() == 2 {
			err.SetErrorUser()
		}
		err.AddTrace(moduleID)
		return nil, "", err
	}
	return qrimage, objinfo.ContentType, nil
}

type (
	resLinkGroup struct {
		Links []resGetLink `json:"links"`
	}
)

// GetLinkGroup retrieves a group of links
func (c *courierService) GetLinkGroup(limit, offset int, creatorid string) (*resLinkGroup, *governor.Error) {
	links, err := c.repo.GetLinkGroup(limit, offset, creatorid)
	if err != nil {
		err.AddTrace(moduleID)
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
func (c *courierService) CreateLink(linkid, url, creatorid string) (*resCreateLink, *governor.Error) {
	m := c.repo.NewLinkEmptyPtr()
	if len(linkid) == 0 {
		ml, err := c.repo.NewLinkAuto(url, creatorid)
		if err != nil {
			err.AddTrace(moduleID)
			return nil, err
		}
		m = ml
	} else {
		ml, err := c.repo.NewLink(linkid, url, creatorid)
		if err != nil {
			err.AddTrace(moduleID)
			return nil, err
		}
		m = ml
	}
	if err := c.repo.InsertLink(m); err != nil {
		err.AddTrace(moduleID)
		return nil, err
	}
	qrimage, err := c.barcode.GenerateBarcode(barcode.TransportQRCode, c.linkPrefix+"/"+linkid)
	if err != nil {
		err.AddTrace(moduleID)
		return nil, err
	}
	if err := c.linkImageBucket.Put(linkid+"-qr", mediaTypePNG, int64(qrimage.Len()), qrimage); err != nil {
		err.AddTrace(moduleID)
		c.logger.Error(err.Error(), moduleID, "fail add link qrcode image to objstore", 0, map[string]string{
			"linkid": linkid,
		})
		return nil, err
	}

	return &resCreateLink{
		LinkID: m.LinkID,
	}, nil
}

// DeleteLink deletes a link
func (c *courierService) DeleteLink(linkid string) *governor.Error {
	m, err := c.repo.GetLink(linkid)
	if err != nil {
		err.AddTrace(moduleID)
		return err
	}
	if err := c.repo.DeleteLink(m); err != nil {
		err.AddTrace(moduleID)
		return err
	}
	if err := c.linkImageBucket.Remove(linkid + "-qr"); err != nil {
		c.logger.Error(err.Error(), moduleID, "fail remove link qrcode image from objstore", 0, map[string]string{
			"linkid": linkid,
		})
	}
	return nil
}
