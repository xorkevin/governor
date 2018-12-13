package couriermodel

import (
	"database/sql"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/db"
	"github.com/hackform/governor/util/uid"
	"net/http"
	"strings"
	"time"
)

//go:generate go run ../../../gen/model.go -- modellink_gen.go link courierlinks LinkModel LinkModel qLink

const (
	uidRandSize  = 8
	moduleID     = "couriermodel"
	moduleIDLink = moduleID + ".Link"
)

type (
	// Repo is a courier repository
	Repo interface {
		NewLink(linkid, url, creatorid string) (*LinkModel, *governor.Error)
		NewLinkAuto(url, creatorid string) (*LinkModel, *governor.Error)
		NewLinkEmpty() LinkModel
		NewLinkEmptyPtr() *LinkModel
		GetLinkGroup(limit, offset int, creatorid string) ([]LinkModel, *governor.Error)
		GetLink(linkid string) (*LinkModel, *governor.Error)
		InsertLink(m *LinkModel) *governor.Error
		DeleteLink(m *LinkModel) *governor.Error
		Setup() *governor.Error
	}

	repo struct {
		db *sql.DB
	}

	// LinkModel is the db link model
	LinkModel struct {
		LinkID       string `model:"linkid,VARCHAR(63) PRIMARY KEY" query:"linkid"`
		URL          string `model:"url,VARCHAR(2047) NOT NULL" query:"url"`
		CreatorID    string `model:"creatorid,VARCHAR(31) NOT NULL" query:"creatorid"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL" query:"creation_time,getgroup"`
	}

	qLink struct {
		LinkID       string `query:"linkid"`
		URL          string `query:"url"`
		CreationTime int64  `query:"creation_time,getgroupeq,creatorid"`
	}
)

// New creates a new courier repo
func New(config governor.Config, l governor.Logger, d db.Database) Repo {
	l.Info("initialized courier repo", moduleID, "initialize courier repo", 0, nil)
	return &repo{
		db: d.DB(),
	}
}

const (
	moduleIDLinkNew = moduleIDLink + ".New"
)

// NewLink creates a new link model
func (r *repo) NewLink(linkid, url, creatorid string) (*LinkModel, *governor.Error) {
	return &LinkModel{
		LinkID:       linkid,
		URL:          url,
		CreatorID:    creatorid,
		CreationTime: time.Now().Unix(),
	}, nil
}

// NewLinkAuto creates a new courier model with the link id randomly generated
func (r *repo) NewLinkAuto(url, creatorid string) (*LinkModel, *governor.Error) {
	mUID, err := uid.NewU(0, uidRandSize)
	if err != nil {
		err.AddTrace(moduleIDLinkNew)
		return nil, err
	}
	rawb64 := strings.TrimRight(mUID.Base64(), "=")
	return r.NewLink(rawb64, url, creatorid)
}

// NewLinkEmpty creates an empty link model
func (r *repo) NewLinkEmpty() LinkModel {
	return LinkModel{}
}

// NewLinkEmptyPtr creates an empty link model reference
func (r *repo) NewLinkEmptyPtr() *LinkModel {
	return &LinkModel{}
}

const (
	moduleIDLinkGetGroup = moduleIDLink + ".GetGroup"
)

// GetLinkGroup retrieves a group of links
func (r *repo) GetLinkGroup(limit, offset int, creatorid string) ([]LinkModel, *governor.Error) {
	if len(creatorid) > 0 {
		m, err := linkModelGetqLinkEqCreatorIDOrdCreationTime(r.db, creatorid, false, limit, offset)
		if err != nil {
			return nil, governor.NewError(moduleIDLinkGetGroup, err.Error(), 0, http.StatusInternalServerError)
		}
		links := make([]LinkModel, 0, len(m))
		for _, i := range m {
			links = append(links, LinkModel{
				LinkID:       i.LinkID,
				URL:          i.URL,
				CreatorID:    creatorid,
				CreationTime: i.CreationTime,
			})
		}
		return links, nil
	}

	m, err := linkModelGetLinkModelOrdCreationTime(r.db, false, limit, offset)
	if err != nil {
		return nil, governor.NewError(moduleIDLinkGetGroup, err.Error(), 0, http.StatusInternalServerError)
	}
	return m, nil
}

const (
	moduleIDLinkGet = moduleIDLink + ".Get"
)

// GetLink returns a link model with the given id
func (r *repo) GetLink(linkid string) (*LinkModel, *governor.Error) {
	var m *LinkModel
	if mLink, code, err := linkModelGet(r.db, linkid); err != nil {
		if code == 2 {
			return nil, governor.NewError(moduleIDLinkGet, "no link found with that id", 2, http.StatusNotFound)
		}
		return nil, governor.NewError(moduleIDLinkGet, err.Error(), 0, http.StatusInternalServerError)
	} else {
		m = mLink
	}
	return m, nil
}

const (
	moduleIDLinkIns = moduleIDLink + ".Insert"
)

// InsertLink inserts the link model into the db
func (r *repo) InsertLink(m *LinkModel) *governor.Error {
	if code, err := linkModelInsert(r.db, m); err != nil {
		if code == 3 {
			return governor.NewError(moduleIDLinkIns, err.Error(), 3, http.StatusBadRequest)
		}
		return governor.NewError(moduleIDLinkIns, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

const (
	moduleIDLinkDel = moduleIDLink + ".Del"
)

// DeleteLink deletes the link model in the db
func (r *repo) DeleteLink(m *LinkModel) *governor.Error {
	if err := linkModelDelete(r.db, m); err != nil {
		return governor.NewError(moduleIDLinkDel, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

const (
	moduleIDSetup = moduleID + ".Setup"
)

// Setup creates new Courier tables
func (r *repo) Setup() *governor.Error {
	if err := linkModelSetup(r.db); err != nil {
		return governor.NewError(moduleIDSetup, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}
