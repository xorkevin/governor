package couriermodel

import (
	"database/sql"
	"net/http"
	"strings"
	"time"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/uid"
)

//go:generate forge model -m LinkModel -t courierlinks -p link -o modellink_gen.go LinkModel qLink

const (
	uidSize = 8
)

type (
	// Repo is a courier repository
	Repo interface {
		NewLink(linkid, url, creatorid string) (*LinkModel, error)
		NewLinkAuto(url, creatorid string) (*LinkModel, error)
		NewLinkEmpty() LinkModel
		NewLinkEmptyPtr() *LinkModel
		GetLinkGroup(limit, offset int, creatorid string) ([]LinkModel, error)
		GetLink(linkid string) (*LinkModel, error)
		InsertLink(m *LinkModel) error
		DeleteLink(m *LinkModel) error
		Setup() error
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
	l.Info("initialize courier repo", nil)
	return &repo{
		db: d.DB(),
	}
}

// NewLink creates a new link model
func (r *repo) NewLink(linkid, url, creatorid string) (*LinkModel, error) {
	return &LinkModel{
		LinkID:       linkid,
		URL:          url,
		CreatorID:    creatorid,
		CreationTime: time.Now().Unix(),
	}, nil
}

// NewLinkAuto creates a new courier model with the link id randomly generated
func (r *repo) NewLinkAuto(url, creatorid string) (*LinkModel, error) {
	mUID, err := uid.New(uidSize)
	if err != nil {
		return nil, governor.NewError("Failed to create new uid", http.StatusInternalServerError, err)
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

// GetLinkGroup retrieves a group of links
func (r *repo) GetLinkGroup(limit, offset int, creatorid string) ([]LinkModel, error) {
	if len(creatorid) > 0 {
		m, err := linkModelGetqLinkEqCreatorIDOrdCreationTime(r.db, creatorid, false, limit, offset)
		if err != nil {
			return nil, governor.NewError("Failed to get links of a creator", http.StatusInternalServerError, err)
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
		return nil, governor.NewError("Failed to get links", http.StatusInternalServerError, err)
	}
	return m, nil
}

// GetLink returns a link model with the given id
func (r *repo) GetLink(linkid string) (*LinkModel, error) {
	var m *LinkModel
	if mLink, code, err := linkModelGet(r.db, linkid); err != nil {
		if code == 2 {
			return nil, governor.NewError("No link found with that id", http.StatusNotFound, err)
		}
		return nil, governor.NewError("Failed to get link", http.StatusInternalServerError, err)
	} else {
		m = mLink
	}
	return m, nil
}

// InsertLink inserts the link model into the db
func (r *repo) InsertLink(m *LinkModel) error {
	if code, err := linkModelInsert(r.db, m); err != nil {
		if code == 3 {
			return governor.NewError("Link id must be unique", http.StatusBadRequest, err)
		}
		return governor.NewError("Failed to insert link", http.StatusInternalServerError, err)
	}
	return nil
}

// DeleteLink deletes the link model in the db
func (r *repo) DeleteLink(m *LinkModel) error {
	if err := linkModelDelete(r.db, m); err != nil {
		return governor.NewError("Failed to delete link", http.StatusInternalServerError, err)
	}
	return nil
}

// Setup creates new Courier tables
func (r *repo) Setup() error {
	if err := linkModelSetup(r.db); err != nil {
		return governor.NewError("Failed to setup link model", http.StatusInternalServerError, err)
	}
	return nil
}
