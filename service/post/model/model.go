package postmodel

import (
	"database/sql"
	"fmt"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/post/vote/model"
	"github.com/hackform/governor/service/user/model"
	"github.com/hackform/governor/util/score"
	"github.com/hackform/governor/util/uid"
	"github.com/lib/pq"
	"net/http"
	"time"
)

const (
	uidTimeSize          = 8
	uidRandSize          = 8
	postScoreEpoch int64 = 1500000000
	tableName            = "posts"
	moduleID             = "postmodel"
	moduleIDModel        = moduleID + ".Model"
)

type (
	// Model is the db Post model
	Model struct {
		ModelInfo
		Content string `json:"content"`
		Locked  bool   `json:"locked"`
	}

	// ModelInfo is metadata of a post
	ModelInfo struct {
		Postid       []byte `json:"postid"`
		Userid       []byte `json:"userid"`
		Tag          string `json:"group_tag"`
		Title        string `json:"title"`
		Up           int32  `json:"up"`
		Down         int32  `json:"down"`
		Absolute     int32  `json:"absolute"`
		Score        int64  `json:"score"`
		CreationTime int64  `json:"creation_time"`
	}

	// ModelSlice is an array of posts
	ModelSlice []ModelInfo
)

const (
	moduleIDModNew = moduleIDModel + ".New"
)

// New creates a new Post Model
func New(userid, tag, title, content string) (*Model, *governor.Error) {
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

	t := time.Now().Unix()

	return &Model{
		ModelInfo: ModelInfo{
			Postid:       mUID.Bytes(),
			Userid:       u.Bytes(),
			Tag:          tag,
			Title:        title,
			Up:           0,
			Down:         0,
			Absolute:     0,
			Score:        score.Log(0, 0, t, postScoreEpoch),
			CreationTime: t,
		},
		Content: content,
		Locked:  false,
	}, nil
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
	moduleIDModRescore = moduleIDModel + ".Rescore"
)

// Rescore updates the score
func (m *Model) Rescore(db *sql.DB) *governor.Error {
	if u, d, err := votemodel.GetScoreByID(db, m.Postid); err == nil {
		m.Up = u
		m.Down = -d
		m.Absolute = u + d
		m.Score = score.Log(m.Up, m.Down, m.CreationTime, postScoreEpoch)
	} else {
		err.AddTrace(moduleIDModRescore)
		return err
	}
	return nil
}

// IsLocked returns if the post is locked
func (m *Model) IsLocked() bool {
	return m.Locked
}

// Lock locks the post
func (m *Model) Lock() {
	m.Locked = true
}

// Unlock unlocks the post
func (m *Model) Unlock() {
	m.Locked = false
}

const (
	moduleIDModGet64 = moduleIDModel + ".GetByIDB64"
)

var (
	sqlGetByIDB64 = fmt.Sprintf("SELECT postid, userid, group_tag, title, content, locked, up, down, absolute, score, creation_time FROM %s WHERE postid=$1;", tableName)
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
		err.SetErrorUser()
		return nil, err
	}
	mPost := &Model{}
	if err := db.QueryRow(sqlGetByIDB64, u.Bytes()).Scan(&mPost.Postid, &mPost.Userid, &mPost.Tag, &mPost.Title, &mPost.Content, &mPost.Locked, &mPost.Up, &mPost.Down, &mPost.Absolute, &mPost.Score, &mPost.CreationTime); err != nil {
		if err == sql.ErrNoRows {
			return nil, governor.NewError(moduleIDModGet64, "no post found with that id", 2, http.StatusNotFound)
		}
		return nil, governor.NewError(moduleIDModGet64, err.Error(), 0, http.StatusInternalServerError)
	}
	return mPost, nil
}

const (
	moduleIDModGetGroup = moduleIDModel + ".GetGroup"
)

var (
	sqlGetGroup = fmt.Sprintf("SELECT postid, userid, group_tag, title, up, down, absolute, score, creation_time FROM %s WHERE group_tag=$1 ORDER BY score DESC LIMIT $2 OFFSET $3;", tableName)
)

// GetGroup returns a list of posts from a group
func GetGroup(db *sql.DB, tag string, limit, offset int) (ModelSlice, *governor.Error) {
	m := ModelSlice{}
	rows, err := db.Query(sqlGetGroup, tag, limit, offset)
	if err != nil {
		return nil, governor.NewError(moduleIDModGetGroup, err.Error(), 0, http.StatusInternalServerError)
	}
	defer rows.Close()
	for rows.Next() {
		i := ModelInfo{}
		if err := rows.Scan(&i.Postid, &i.Userid, &i.Tag, &i.Title, &i.Up, &i.Down, &i.Absolute, &i.Score, &i.CreationTime); err != nil {
			return nil, governor.NewError(moduleIDModGetGroup, err.Error(), 0, http.StatusInternalServerError)
		}
		m = append(m, i)
	}
	if err := rows.Err(); err != nil {
		return nil, governor.NewError(moduleIDModGetGroup, err.Error(), 0, http.StatusInternalServerError)
	}
	return m, nil
}

const (
	moduleIDModIns = moduleIDModel + ".Insert"
)

var (
	sqlInsert = fmt.Sprintf("INSERT INTO %s (postid, userid, group_tag, title, content, locked, up, down, absolute, score, creation_time) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);", tableName)
)

// Insert inserts the model into the db
func (m *Model) Insert(db *sql.DB) *governor.Error {
	_, err := db.Exec(sqlInsert, m.Postid, m.Userid, m.Tag, m.Title, m.Content, m.Locked, m.Up, m.Down, m.Absolute, m.Score, m.CreationTime)
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
	sqlUpdate = fmt.Sprintf("UPDATE %s SET (postid, userid, group_tag, title, content, locked, up, down, absolute, score, creation_time) = ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) WHERE postid=$1;", tableName)
)

// Update updates the model in the db
func (m *Model) Update(db *sql.DB) *governor.Error {
	_, err := db.Exec(sqlUpdate, m.Postid, m.Userid, m.Tag, m.Title, m.Content, m.Locked, m.Up, m.Down, m.Absolute, m.Score, m.CreationTime)
	if err != nil {
		return governor.NewError(moduleIDModUp, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

const (
	moduleIDModDel = moduleIDModel + ".Delete"
)

var (
	sqlDelete = fmt.Sprintf("DELETE FROM %s WHERE postid=$1;", tableName)
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
	sqlSetup = fmt.Sprintf("CREATE TABLE %s (postid BYTEA PRIMARY KEY, userid BYTEA NOT NULL, group_tag VARCHAR(255) NOT NULL, title VARCHAR(1024) NOT NULL, content VARCHAR(131072) NOT NULL, locked BOOLEAN NOT NULL, up INT NOT NULL, down INT NOT NULL, absolute INT NOT NULL, score BIGINT NOT NULL, creation_time BIGINT NOT NULL);", tableName)
)

// Setup creates a new Post table
func Setup(db *sql.DB) *governor.Error {
	_, err := db.Exec(sqlSetup)
	if err != nil {
		return governor.NewError(moduleIDSetup, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}
