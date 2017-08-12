package votemodel

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
	tableName     = "votes"
	moduleID      = "votemodel"
	moduleIDModel = moduleID + ".Model"
)

type (
	// Model is the db Vote model
	Model struct {
		ModelInfo
		Postid []byte `json:"postid"`
		Group  string `json:"group"`
		Time   int64  `json:"time"`
	}

	// ModelInfo is the core vote information
	ModelInfo struct {
		Itemid []byte `json:"itemid"`
		Userid []byte `json:"userid"`
		Score  int16  `json:"score"`
	}
)

const (
	moduleIDModNew = moduleIDModel + ".New"
)

// New creates a new Vote Model
func New(itemid, postid, group, userid string, score int16) (*Model, *governor.Error) {
	item, err := ParseB64ToUID(itemid)
	if err != nil {
		err.AddTrace(moduleIDModNew)
		err.SetErrorUser()
		return nil, err
	}
	post, err := ParseB64ToUID(postid)
	if err != nil {
		err.AddTrace(moduleIDModNew)
		err.SetErrorUser()
		return nil, err
	}
	user, err := ParseB64ToUID(userid)
	if err != nil {
		err.AddTrace(moduleIDModNew)
		err.SetErrorUser()
		return nil, err
	}

	return &Model{
		ModelInfo: ModelInfo{
			Itemid: item.Bytes(),
			Userid: user.Bytes(),
			Score:  score,
		},
		Postid: post.Bytes(),
		Group:  group,
		Time:   time.Now().Unix(),
	}, nil
}

// NewUp creates a new upvote
func NewUp(itemid, postid, group, userid string) (*Model, *governor.Error) {
	return New(itemid, postid, group, userid, 1)
}

// NewDown creates a new downvote
func NewDown(itemid, postid, group, userid string) (*Model, *governor.Error) {
	return New(itemid, postid, group, userid, -1)
}

var (
	postidNullVal = []byte{0, 0}
)

// NewPost creates a new Vote Model for a post
func NewPost(postid, group, userid string, score int16) (*Model, *governor.Error) {
	post, err := ParseB64ToUID(postid)
	if err != nil {
		err.AddTrace(moduleIDModNew)
		err.SetErrorUser()
		return nil, err
	}
	user, err := ParseB64ToUID(userid)
	if err != nil {
		err.AddTrace(moduleIDModNew)
		err.SetErrorUser()
		return nil, err
	}

	return &Model{
		ModelInfo: ModelInfo{
			Itemid: post.Bytes(),
			Userid: user.Bytes(),
			Score:  score,
		},
		Postid: postidNullVal,
		Group:  group,
		Time:   time.Now().Unix(),
	}, nil
}

// NewUpPost creates a new upvote for a post
func NewUpPost(postid, group, userid string) (*Model, *governor.Error) {
	return NewPost(postid, group, userid, 1)
}

// NewDownPost creates a new downvote for a post
func NewDownPost(postid, group, userid string) (*Model, *governor.Error) {
	return NewPost(postid, group, userid, -1)
}

// Voteid returns the voteid for a vote
func (m *Model) Voteid() []byte {
	return append(m.Itemid, m.Userid...)
}

// Voteid returns the voteid for a vote
func (m *ModelInfo) Voteid() []byte {
	return append(m.Itemid, m.Userid...)
}

const (
	moduleIDModB64 = moduleIDModel + ".IDBase64"
)

// ParseUIDToB64 converts a UID into base64
func ParseUIDToB64(postid []byte) (*uid.UID, *governor.Error) {
	return uid.FromBytesTRSplit(postid)
}

// IDBase64 returns the item as a base64 encoded string
func (m *Model) IDBase64() (string, *governor.Error) {
	u, err := ParseUIDToB64(m.Itemid)
	if err != nil {
		err.AddTrace(moduleIDModB64)
		return "", err
	}
	return u.Base64(), nil
}

const (
	moduleIDModPostB64 = moduleIDModel + ".PostIDBase64"
)

// PostIDBase64 returns the post as a base64 encoded string
func (m *Model) PostIDBase64() (string, *governor.Error) {
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

// Up makes the vote an upvote
func (m *Model) Up() {
	m.Score = 1
}

// Down makes the vote a downvote
func (m *Model) Down() {
	m.Score = -1
}

func (m *Model) IsUp() bool {
	return m.Score == 1
}

func (m *Model) IsDown() bool {
	return m.Score == -1
}

const (
	moduleIDModGet64 = moduleIDModel + ".GetByIDB64"
)

var (
	sqlGetByID = fmt.Sprintf("SELECT itemid, postid, group_tag, userid, score, time FROM %s WHERE voteid=$1;", tableName)
)

// ParseB64ToUID converts a base64 into a UID
func ParseB64ToUID(idb64 string) (*uid.UID, *governor.Error) {
	return uid.FromBase64TRSplit(idb64)
}

// GetByIDB64 returns a vote model with the given base64 ids
func GetByIDB64(db *sql.DB, itemid, userid string) (*Model, *governor.Error) {
	item, err := ParseB64ToUID(itemid)
	if err != nil {
		err.AddTrace(moduleIDModGet64)
		err.SetErrorUser()
		return nil, err
	}
	user, err := ParseB64ToUID(userid)
	if err != nil {
		err.AddTrace(moduleIDModGet64)
		err.SetErrorUser()
		return nil, err
	}

	v := ModelInfo{
		Itemid: item.Bytes(),
		Userid: user.Bytes(),
		Score:  0,
	}

	mVote := &Model{}
	if err := db.QueryRow(sqlGetByID, v.Voteid()).Scan(&mVote.Itemid, &mVote.Postid, &mVote.Group, &mVote.Userid, &mVote.Score, &mVote.Time); err != nil {
		if err == sql.ErrNoRows {
			return nil, governor.NewError(moduleIDModGet64, "no vote found with the ids", 2, http.StatusNotFound)
		}
		return nil, governor.NewError(moduleIDModGet64, err.Error(), 0, http.StatusInternalServerError)
	}
	return mVote, nil
}

const (
	moduleIDModGetScore = moduleIDModel + ".GetScoreByIDB64"
)

var (
	sqlGetScore = fmt.Sprintf("SELECT score FROM %s WHERE itemid=$1;", tableName)
)

// GetScoreByIDB64 returns the score of an item
func GetScoreByIDB64(db *sql.DB, itemid string) (int32, int32, *governor.Error) {
	item, err := ParseB64ToUID(itemid)
	if err != nil {
		err.AddTrace(moduleIDModGetScore)
		err.SetErrorUser()
		return 0, 0, err
	}

	return GetScoreByID(db, item.Bytes())
}

// GetScoreByID returns the score of an item
func GetScoreByID(db *sql.DB, itemid []byte) (int32, int32, *governor.Error) {
	var u int32
	var d int32

	if rows, err := db.Query(sqlGetScore, itemid); err == nil {
		defer rows.Close()
		for rows.Next() {
			var j int16
			if err = rows.Scan(&j); err != nil {
				return 0, 0, governor.NewError(moduleIDModGetScore, err.Error(), 0, http.StatusInternalServerError)
			}
			if j > 0 {
				u += int32(j)
			} else {
				d += int32(j)
			}
		}
		if err = rows.Err(); err != nil {
			return 0, 0, governor.NewError(moduleIDModGetScore, err.Error(), 0, http.StatusInternalServerError)
		}
	} else {
		return 0, 0, governor.NewError(moduleIDModGetScore, err.Error(), 0, http.StatusInternalServerError)
	}

	return u, d, nil
}

const (
	moduleIDModGetVotesGroup = moduleIDModel + ".GetVotesGroupByUser"
)

var (
	sqlGetVotesGroup = fmt.Sprintf("SELECT itemid, userid, score FROM %s WHERE userid=$1 AND group_tag=$2 AND postid=$3;", tableName)
)

// GetVotesGroupByUser returns the votes of a user for a group
func GetVotesGroupByUser(db *sql.DB, userid, group string) ([]ModelInfo, *governor.Error) {
	user, err := ParseB64ToUID(userid)
	if err != nil {
		err.AddTrace(moduleIDModGetVotesGroup)
		err.SetErrorUser()
		return nil, err
	}

	m := []ModelInfo{}

	if rows, err := db.Query(sqlGetVotesGroup, user.Bytes(), group, postidNullVal); err == nil {
		defer rows.Close()
		for rows.Next() {
			i := ModelInfo{}
			if err = rows.Scan(&i.Itemid, &i.Userid, &i.Score); err != nil {
				return nil, governor.NewError(moduleIDModGetVotesGroup, err.Error(), 0, http.StatusInternalServerError)
			}
			m = append(m, i)
		}
		if err = rows.Err(); err != nil {
			return nil, governor.NewError(moduleIDModGetVotesGroup, err.Error(), 0, http.StatusInternalServerError)
		}
	} else {
		return nil, governor.NewError(moduleIDModGetVotesGroup, err.Error(), 0, http.StatusInternalServerError)
	}

	return m, nil
}

const (
	moduleIDModGetVotesThread = moduleIDModel + ".GetVotesThreadByUser"
)

var (
	sqlGetVotesThread = fmt.Sprintf("SELECT itemid, userid, score FROM %s WHERE userid=$1 AND postid=$2;", tableName)
)

// GetVotesThreadByUser returns the votes of a user for a thread
func GetVotesThreadByUser(db *sql.DB, userid, postid string) ([]ModelInfo, *governor.Error) {
	user, err := ParseB64ToUID(userid)
	if err != nil {
		err.AddTrace(moduleIDModGetVotesThread)
		err.SetErrorUser()
		return nil, err
	}
	post, err := ParseB64ToUID(postid)
	if err != nil {
		err.AddTrace(moduleIDModGetVotesThread)
		err.SetErrorUser()
		return nil, err
	}

	m := []ModelInfo{}

	if rows, err := db.Query(sqlGetVotesThread, user.Bytes(), post.Bytes()); err == nil {
		defer rows.Close()
		for rows.Next() {
			i := ModelInfo{}
			if err = rows.Scan(&i.Itemid, &i.Userid, &i.Score); err != nil {
				return nil, governor.NewError(moduleIDModGetVotesThread, err.Error(), 0, http.StatusInternalServerError)
			}
			m = append(m, i)
		}
		if err = rows.Err(); err != nil {
			return nil, governor.NewError(moduleIDModGetVotesThread, err.Error(), 0, http.StatusInternalServerError)
		}
	} else {
		return nil, governor.NewError(moduleIDModGetVotesThread, err.Error(), 0, http.StatusInternalServerError)
	}

	return m, nil
}

const (
	moduleIDModIns = moduleIDModel + ".Insert"
)

var (
	sqlInsert = fmt.Sprintf("INSERT INTO %s (voteid, itemid, postid, group_tag, userid, score, time) VALUES ($1, $2, $3, $4, $5, $6, $7);", tableName)
)

// Insert inserts the model into the db
func (m *Model) Insert(db *sql.DB) *governor.Error {
	_, err := db.Exec(sqlInsert, m.Voteid(), m.Itemid, m.Postid, m.Group, m.Userid, m.Score, m.Time)
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
	sqlUpdate = fmt.Sprintf("UPDATE %s SET (voteid, itemid, postid, group_tag, userid, score, time) = ($1, $2, $3, $4, $5, $6, $7) WHERE voteid=$1;", tableName)
)

// Update updates the model in the db
func (m *Model) Update(db *sql.DB) *governor.Error {
	_, err := db.Exec(sqlUpdate, m.Voteid(), m.Itemid, m.Postid, m.Group, m.Userid, m.Score, m.Time)
	if err != nil {
		return governor.NewError(moduleIDModUp, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

const (
	moduleIDModDel = moduleIDModel + ".Delete"
)

var (
	sqlDelete = fmt.Sprintf("DELETE FROM %s WHERE voteid = $1;", tableName)
)

// Delete deletes the model in the db
func (m *Model) Delete(db *sql.DB) *governor.Error {
	_, err := db.Exec(sqlDelete, m.Voteid())
	if err != nil {
		return governor.NewError(moduleIDModDel, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

const (
	moduleIDSetup = moduleID + ".Setup"
)

var (
	sqlSetup = fmt.Sprintf("CREATE TABLE %s (voteid BYTEA PRIMARY KEY, itemid BYTEA NOT NULL, postid BYTEA NOT NULL, group_tag VARCHAR(255) NOT NULL, userid BYTEA NOT NULL, score SMALLINT NOT NULL, time BIGINT NOT NULL);", tableName)
)

// Setup creates a new Post table
func Setup(db *sql.DB) *governor.Error {
	_, err := db.Exec(sqlSetup)
	if err != nil {
		return governor.NewError(moduleIDSetup, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}
