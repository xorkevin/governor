package postmodel

import (
	"database/sql"
	"fmt"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/model"
	"github.com/hackform/governor/util/uid"
	"github.com/lib/pq"
	"net/http"
	"time"
)

const (
	uidTimeSize   = 8
	uidRandSize   = 8
	tableName     = "posts"
	moduleID      = "postmodel"
	moduleIDModel = moduleID + ".Model"
)

type (
	// Model is the db Post model
	Model struct {
		Postid       []byte `json:"postid"`
		Userid       []byte `json:"userid"`
		Tags         string `json:"group_tags"`
		Content      string `json:"content"`
		CreationTime int64  `json:"creation_time"`
	}
)

const (
	moduleIDModNew = moduleIDModel + ".New"
)

// New creates a new Post Model
func New(userid, tags, content string) (*Model, *governor.Error) {
	u, err := ParseB64ToUID(userid)
	if err != nil {
		err.AddTrace(moduleIDModNew)
		err.SetErrorUser()
		return nil, err
	}

	mUID, err := uid.NewU(uidTimeSize, uidRandSize)
	if err != nil {
		err.AddTrace(moduleIDModNew)
		return nil, err
	}

	return &Model{
		Postid:       mUID.Bytes(),
		Userid:       u.Bytes(),
		Tags:         tags,
		Content:      content,
		CreationTime: time.Now().Unix(),
	}, nil
}

const (
	moduleIDModSetUserIDB64 = moduleIDModel + ".SetUserIDB64"
)

// SetUserIDB64 sets the userid of the model from a base64 value
func (m *Model) SetUserIDB64(idb64 string) *governor.Error {
	u, err := usermodel.ParseB64ToUID(idb64)
	if err != nil {
		err.AddTrace(moduleIDModSetUserIDB64)
		return err
	}
	m.Userid = u.Bytes()
	return nil
}

const (
	moduleIDModB64 = moduleIDModel + ".IDBase64"
)

// ParseUIDToB64 converts a UID postid into base64
func ParseUIDToB64(postid []byte) (*uid.UID, *governor.Error) {
	return uid.FromBytesTRSplit(postid)
}

// IDBase64 returns the postid as a base64 encoded string
func (m *Model) IDBase64() (string, *governor.Error) {
	u, err := ParseUIDToB64(m.Postid)
	if err != nil {
		err.AddTrace(moduleIDModB64)
		return "", err
	}
	return u.Base64(), nil
}

const (
	moduleIDModUserB64 = moduleIDModel + ".UserIDBase64"
)

// UserIDBase64 returns the userid as a base64 encoded string
func (m *Model) UserIDBase64() (string, *governor.Error) {
	u, err := usermodel.ParseUIDToB64(m.Userid)
	if err != nil {
		err.AddTrace(moduleIDModUserB64)
		return "", err
	}
	return u.Base64(), nil
}

const (
	moduleIDModGet64 = moduleIDModel + ".GetByIDB64"
)

var (
	sqlGetByIDB64 = fmt.Sprintf("SELECT postid, userid, group_tags, content, creation_time FROM %s WHERE postid=$1;", tableName)
)

// ParseB64ToUID converts a postid in base64 into a UID
func ParseB64ToUID(idb64 string) (*uid.UID, *governor.Error) {
	return uid.FromBase64TRSplit(idb64)
}

// GetByIDB64 returns a post model with the given base64 id
func GetByIDB64(db *sql.DB, idb64 string) (*Model, *governor.Error) {
	u, err := ParseB64ToUID(idb64)
	if err != nil {
		err.AddTrace(moduleIDModGet64)
		return nil, err
	}
	mPost := &Model{}
	if err := db.QueryRow(sqlGetByIDB64, u.Bytes()).Scan(&mPost.Postid, &mPost.Userid, &mPost.Content, &mPost.CreationTime); err != nil {
		if err == sql.ErrNoRows {
			return nil, governor.NewError(moduleIDModGet64, "no post found with that id", 2, http.StatusNotFound)
		}
		return nil, governor.NewError(moduleIDModGet64, err.Error(), 0, http.StatusInternalServerError)
	}
	return mPost, nil
}

const (
	moduleIDModIns = moduleIDModel + ".Insert"
)

var (
	sqlInsert = fmt.Sprintf("INSERT INTO %s (postid, userid, group_tags, content, creation_time) VALUES ($1, $2, $3, $4);", tableName)
)

// Insert inserts the model into the db
func (m *Model) Insert(db *sql.DB) *governor.Error {
	_, err := db.Exec(sqlInsert, m.Postid, m.Userid, m.Content, m.CreationTime)
	if err != nil {
		if postgresErr, ok := err.(*pq.Error); ok {
			switch postgresErr.Code {
			case "23505": // unique_violation
				return governor.NewErrorUser(moduleIDModIns, err.Error(), 3, http.StatusBadRequest)
			default:
				return governor.NewError(moduleIDModIns, err.Error(), 0, http.StatusInternalServerError)
			}
		}
	}
	return nil
}

const (
	moduleIDModUp = moduleIDModel + ".Update"
)

var (
	sqlUpdate = fmt.Sprintf("UPDATE %s SET (postid, userid, group_tags, content, creation_time) = ($1, $2, $3, $4) WHERE postid = $1;", tableName)
)

// Update updates the model in the db
func (m *Model) Update(db *sql.DB) *governor.Error {
	_, err := db.Exec(sqlUpdate, m.Postid, m.Userid, m.Content, m.CreationTime)
	if err != nil {
		return governor.NewError(moduleIDModUp, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

const (
	moduleIDModDel = moduleIDModel + ".Delete"
)

var (
	sqlDelete = fmt.Sprintf("DELETE FROM %s WHERE postid = $1;", tableName)
)

// Delete deletes the model in the db
func (m *Model) Delete(db *sql.DB) *governor.Error {
	_, err := db.Exec(sqlDelete, m.Postid)
	if err != nil {
		return governor.NewError(moduleIDModDel, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

const (
	moduleIDSetup = moduleID + ".Setup"
)

var (
	sqlSetup = fmt.Sprintf("CREATE TABLE %s (postid BYTEA PRIMARY KEY, userid BYTEA NOT NULL, group_tags VARCHAR(4096) NOT NULL, content VARCHAR(65536) NOT NULL, creation_time BIGINT NOT NULL);", tableName)
)

// Setup creates a new Post table
func Setup(db *sql.DB) *governor.Error {
	_, err := db.Exec(sqlSetup)
	if err != nil {
		return governor.NewError(moduleIDSetup, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}
