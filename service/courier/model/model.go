package couriermodel

import (
	"database/sql"
	"fmt"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/db"
	"github.com/hackform/governor/util/uid"
	"net/http"
	"strings"
	"time"
)

const (
	uidRandSize   = 8
	linkTableName = "courierlinks"
	moduleID      = "couriermodel"
	moduleIDLink  = moduleID + ".Link"
)

type (
	// Repo is a courier repository
	Repo interface {
		NewLink(linkid, url, creatorid string) (*LinkModel, *governor.Error)
		NewLinkAuto(url, creatorid string) (*LinkModel, *governor.Error)
		//GetLinkByID(id string) (*LinkModel, *governor.Error)
		//InsertLink(m *LinkModel) *governor.Error
		//DeleteLink(m *LinkModel) *governor.Error
		Setup() *governor.Error
	}

	repo struct {
		db *sql.DB
	}

	// LinkModel is the db link model
	LinkModel struct {
		LinkID       string `json:"linkid"`
		URL          string `json:"url"`
		CreatorID    string `json:"creatorid"`
		CreationTime int64  `json:"creation_time"`
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

const (
	moduleIDSetup = moduleID + ".Setup"
)

var (
	sqlLinksSetup = fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (linkid VARCHAR(64) PRIMARY KEY, url VARCHAR(2048) NOT NULL, creatorid VARCHAR(64), creation_time BIGINT NOT NULL);", linkTableName)
)

// Setup creates a new User table
func (r *repo) Setup() *governor.Error {
	_, err := r.db.Exec(sqlLinksSetup)
	if err != nil {
		return governor.NewError(moduleIDSetup, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}
