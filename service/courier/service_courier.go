package courier

import (
	"github.com/hackform/governor"
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
	return nil
}
