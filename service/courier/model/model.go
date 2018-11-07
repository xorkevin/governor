package couriermodel

import (
	"database/sql"
	"fmt"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/db"
	"github.com/hackform/governor/util/uid"
	"github.com/lib/pq"
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
		NewLinkEmpty() LinkModel
		NewLinkEmptyPtr() *LinkModel
		GetLinkGroup(limit, offset int, agedesc bool, creatorid string) ([]LinkModel, *governor.Error)
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

const (
	sqlLinkGetGroup = "SELECT linkid, url, creatorid, creation_time FROM %s %s ORDER BY creation_time %s LIMIT $1 OFFSET $2;"
)

// GetLinkGroup retrieves a group of links
func (r *repo) GetLinkGroup(limit, offset int, agedesc bool, creatorid string) ([]LinkModel, *governor.Error) {
	m := make([]LinkModel, 0, limit)
	arguments := []interface{}{limit, offset}

	dir := "ASC"
	if agedesc {
		dir = "DESC"
	}

	cond := ""
	if len(creatorid) > 0 {
		cond = "WHERE creatorid=$3"
		arguments = append(arguments, creatorid)
	}

	rows, err := r.db.Query(fmt.Sprintf(sqlLinkGetGroup, linkTableName, cond, dir), arguments...)
	if err != nil {
		return nil, governor.NewError(moduleIDLinkGetGroup, err.Error(), 0, http.StatusInternalServerError)
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		i := LinkModel{}
		if err := rows.Scan(&i.LinkID, &i.URL, &i.CreatorID, &i.CreationTime); err != nil {
			return nil, governor.NewError(moduleIDLinkGetGroup, err.Error(), 0, http.StatusInternalServerError)
		}
		m = append(m, i)
	}
	if err := rows.Err(); err != nil {
		return nil, governor.NewError(moduleIDLinkGetGroup, err.Error(), 0, http.StatusInternalServerError)
	}
	return m, nil
}

const (
	moduleIDLinkGet = moduleIDLink + ".Get"
)

var (
	sqlLinkGet = fmt.Sprintf("SELECT linkid, url, creatorid, creation_time FROM %s WHERE linkid=$1;", linkTableName)
)

// GetLink returns a link model with the given id
func (r *repo) GetLink(linkid string) (*LinkModel, *governor.Error) {
	m := &LinkModel{}
	if err := r.db.QueryRow(sqlLinkGet, linkid).Scan(&m.LinkID, &m.URL, &m.CreatorID, &m.CreationTime); err != nil {
		if err == sql.ErrNoRows {
			return nil, governor.NewError(moduleIDLinkGet, "no link found with that id", 2, http.StatusNotFound)
		}
		return nil, governor.NewError(moduleIDLinkGet, err.Error(), 0, http.StatusInternalServerError)
	}
	return m, nil
}

const (
	moduleIDLinkIns = moduleIDLink + ".Insert"
)

var (
	sqlLinkInsert = fmt.Sprintf("INSERT INTO %s (linkid, url, creatorid, creation_time) VALUES ($1, $2, $3, $4);", linkTableName)
)

// InsertLink inserts the link model into the db
func (r *repo) InsertLink(m *LinkModel) *governor.Error {
	_, err := r.db.Exec(sqlLinkInsert, m.LinkID, m.URL, m.CreatorID, m.CreationTime)
	if err != nil {
		if postgresErr, ok := err.(*pq.Error); ok {
			switch postgresErr.Code {
			case "23505": // unique_violation
				return governor.NewError(moduleIDLinkIns, err.Error(), 3, http.StatusBadRequest)
			default:
				return governor.NewError(moduleIDLinkIns, err.Error(), 0, http.StatusInternalServerError)
			}
		}
	}
	return nil
}

const (
	moduleIDLinkDel = moduleIDLink + ".Del"
)

var (
	sqlLinkDelete = fmt.Sprintf("DELETE FROM %s WHERE linkid=$1;", linkTableName)
)

// DeleteLink deletes the link model in the db
func (r *repo) DeleteLink(m *LinkModel) *governor.Error {
	if _, err := r.db.Exec(sqlLinkDelete, m.LinkID); err != nil {
		return governor.NewError(moduleIDLinkDel, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

const (
	moduleIDSetup = moduleID + ".Setup"
)

var (
	sqlLinksSetup = fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (linkid VARCHAR(64) PRIMARY KEY, url VARCHAR(2048) NOT NULL, creatorid VARCHAR(64) NOT NULL, creation_time BIGINT NOT NULL);", linkTableName)
)

// Setup creates new Courier tables
func (r *repo) Setup() *governor.Error {
	_, err := r.db.Exec(sqlLinksSetup)
	if err != nil {
		return governor.NewError(moduleIDSetup, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}
