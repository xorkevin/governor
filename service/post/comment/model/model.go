package commentmodel

import (
	"database/sql"
	"fmt"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/post/vote/model"
	"github.com/hackform/governor/util/score"
	"github.com/hackform/governor/util/uid"
	"github.com/lib/pq"
	"net/http"
	"time"
)

const (
	uidTimeSize   = 8
	uidRandSize   = 8
	tableName     = "comments"
	moduleID      = "commentmodel"
	moduleIDModel = moduleID + ".Model"
)

type (
	// Model is the db Comment model
	Model struct {
		Commentid    []byte `json:"commentid"`
		Parentid     []byte `json:"parentid"`
		Postid       []byte `json:"postid"`
		Userid       string `json:"userid"`
		Content      string `json:"content"`
		Up           int32  `json:"up"`
		Down         int32  `json:"down"`
		Absolute     int32  `json:"absolute"`
		Score        int64  `json:"score"`
		CreationTime int64  `json:"creation_time"`
	}

	// ModelSlice is an array of comments
	ModelSlice []Model
)

const (
	moduleIDModNew = moduleIDModel + ".New"
)

// New creates a new Comment Model
func New(userid, postid, parentid, content string) (*Model, *governor.Error) {
	post, err := ParseB64ToUID(postid)
	if err != nil {
		err.AddTrace(moduleIDModNew)
		err.SetErrorUser()
		return nil, err
	}
	parent, err := ParseB64ToUID(parentid)
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
		Commentid:    mUID.Bytes(),
		Parentid:     parent.Bytes(),
		Postid:       post.Bytes(),
		Userid:       userid,
		Content:      content,
		Up:           0,
		Down:         0,
		Absolute:     0,
		Score:        score.Confidence(0, 0),
		CreationTime: time.Now().Unix(),
	}, nil
}

const (
	moduleIDModB64 = moduleIDModel + ".IDBase64"
)

// ParseUIDToB64 converts a UID commentid into base64
func ParseUIDToB64(commentid []byte) (*uid.UID, *governor.Error) {
	return uid.FromBytesTRSplit(commentid)
}

// IDBase64 returns the commentid as a base64 encoded string
func (m *Model) IDBase64() (string, *governor.Error) {
	u, err := ParseUIDToB64(m.Commentid)
	if err != nil {
		err.AddTrace(moduleIDModB64)
		return "", err
	}
	return u.Base64(), nil
}

const (
	moduleIDModParentB64 = moduleIDModel + ".ParentIDBase64"
)

// ParentIDBase64 returns the parentid as a base64 encoded string
func (m *Model) ParentIDBase64() (string, *governor.Error) {
	u, err := ParseUIDToB64(m.Parentid)
	if err != nil {
		err.AddTrace(moduleIDModParentB64)
		return "", err
	}
	return u.Base64(), nil
}

const (
	moduleIDModPostB64 = moduleIDModel + ".PostIDBase64"
)

// PostIDBase64 returns the postid as a base64 encoded string
func (m *Model) PostIDBase64() (string, *governor.Error) {
	u, err := ParseUIDToB64(m.Postid)
	if err != nil {
		err.AddTrace(moduleIDModPostB64)
		return "", err
	}
	return u.Base64(), nil
}

const (
	moduleIDModRescore = moduleIDModel + ".Rescore"
)

// Rescore updates the score
func (m *Model) Rescore(db *sql.DB) *governor.Error {
	if u, d, err := votemodel.GetScoreByID(db, m.Commentid); err == nil {
		m.Up = u
		m.Down = -d
		m.Absolute = u + d
		m.Score = score.Confidence(m.Up, m.Down)
	} else {
		err.AddTrace(moduleIDModRescore)
		return err
	}
	return nil
}

const (
	moduleIDModGet64 = moduleIDModel + ".GetByIDB64"
)

var (
	sqlGetByIDB64 = fmt.Sprintf("SELECT commentid, parentid, postid, userid, content, up, down, absolute, score, creation_time FROM %s WHERE commentid=$1 AND postid=$2;", tableName)
)

// ParseB64ToUID converts a commentid in base64 into a UID
func ParseB64ToUID(idb64 string) (*uid.UID, *governor.Error) {
	return uid.FromBase64TRSplit(idb64)
}

// GetByIDB64 returns a comment model with the given base64 id
func GetByIDB64(db *sql.DB, commentid, postid string) (*Model, *governor.Error) {
	c, err := ParseB64ToUID(commentid)
	if err != nil {
		err.AddTrace(moduleIDModGet64)
		err.SetErrorUser()
		return nil, err
	}
	p, err := ParseB64ToUID(postid)
	if err != nil {
		err.AddTrace(moduleIDModGet64)
		err.SetErrorUser()
		return nil, err
	}
	mComment := &Model{}
	if err := db.QueryRow(sqlGetByIDB64, c.Bytes(), p.Bytes()).Scan(&mComment.Commentid, &mComment.Parentid, &mComment.Postid, &mComment.Userid, &mComment.Content, &mComment.Up, &mComment.Down, &mComment.Absolute, &mComment.Score, &mComment.CreationTime); err != nil {
		if err == sql.ErrNoRows {
			return nil, governor.NewError(moduleIDModGet64, "no comment found with that id", 2, http.StatusNotFound)
		}
		return nil, governor.NewError(moduleIDModGet64, err.Error(), 0, http.StatusInternalServerError)
	}
	return mComment, nil
}

const (
	moduleIDModGetChildren = moduleIDModel + ".GetChildren"
)

var (
	sqlGetChildren = fmt.Sprintf("SELECT commentid, parentid, postid, userid, content, up, down, absolute, score, creation_time FROM %s WHERE parentid=$1 AND postid=$2 ORDER BY score DESC LIMIT $3 OFFSET $4;", tableName)
)

// GetChildren returns a list of child comments of an item
// parentid could be that of a post or a comment
func GetChildren(db *sql.DB, parentid, postid string, limit, offset int) (ModelSlice, *governor.Error) {
	c, err := ParseB64ToUID(parentid)
	if err != nil {
		err.AddTrace(moduleIDModGet64)
		err.SetErrorUser()
		return nil, err
	}
	p, err := ParseB64ToUID(postid)
	if err != nil {
		err.AddTrace(moduleIDModGet64)
		err.SetErrorUser()
		return nil, err
	}

	m := make(ModelSlice, 0, limit)
	if rows, err := db.Query(sqlGetChildren, c.Bytes(), p.Bytes(), limit, offset); err == nil {
		defer func() {
			err := rows.Close()
			if err != nil {
				fmt.Println(err.Error())
			}
		}()
		for rows.Next() {
			i := Model{}
			if err = rows.Scan(&i.Commentid, &i.Parentid, &i.Postid, &i.Userid, &i.Content, &i.Up, &i.Down, &i.Absolute, &i.Score, &i.CreationTime); err != nil {
				return nil, governor.NewError(moduleIDModGetChildren, err.Error(), 0, http.StatusInternalServerError)
			}
			m = append(m, i)
		}
		if err = rows.Err(); err != nil {
			return nil, governor.NewError(moduleIDModGetChildren, err.Error(), 0, http.StatusInternalServerError)
		}
	} else {
		return nil, governor.NewError(moduleIDModGetChildren, err.Error(), 0, http.StatusInternalServerError)
	}
	return m, nil
}

const (
	moduleIDModGetResponses = moduleIDModel + ".GetResponses"
)

var (
	sqlGetResponses = fmt.Sprintf("SELECT commentid, parentid, postid, userid, content, up, down, absolute, score, creation_time FROM %s WHERE postid=$1 ORDER BY score DESC LIMIT $2 OFFSET $3;", tableName)
)

// GetResponses returns a list of top comments of an item
func GetResponses(db *sql.DB, postid string, limit, offset int) (ModelSlice, *governor.Error) {
	p, err := ParseB64ToUID(postid)
	if err != nil {
		err.AddTrace(moduleIDModGetResponses)
		err.SetErrorUser()
		return nil, err
	}

	m := make(ModelSlice, 0, limit)
	if rows, err := db.Query(sqlGetResponses, p.Bytes(), limit, offset); err == nil {
		defer func() {
			err := rows.Close()
			if err != nil {
				fmt.Println(err.Error())
			}
		}()
		for rows.Next() {
			i := Model{}
			if err = rows.Scan(&i.Commentid, &i.Parentid, &i.Postid, &i.Userid, &i.Content, &i.Up, &i.Down, &i.Absolute, &i.Score, &i.CreationTime); err != nil {
				return nil, governor.NewError(moduleIDModGetResponses, err.Error(), 0, http.StatusInternalServerError)
			}
			m = append(m, i)
		}
		if err = rows.Err(); err != nil {
			return nil, governor.NewError(moduleIDModGetResponses, err.Error(), 0, http.StatusInternalServerError)
		}
	} else {
		return nil, governor.NewError(moduleIDModGetResponses, err.Error(), 0, http.StatusInternalServerError)
	}
	return m, nil
}

const (
	moduleIDModIns = moduleIDModel + ".Insert"
)

var (
	sqlInsert = fmt.Sprintf("INSERT INTO %s (commentid, parentid, postid, userid, content, up, down, absolute, score, creation_time) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);", tableName)
)

// Insert inserts the model into the db
func (m *Model) Insert(db *sql.DB) *governor.Error {
	_, err := db.Exec(sqlInsert, m.Commentid, m.Parentid, m.Postid, m.Userid, m.Content, m.Up, m.Down, m.Absolute, m.Score, m.CreationTime)
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
	sqlUpdate = fmt.Sprintf("UPDATE %s SET (commentid, parentid, postid, userid, content, up, down, absolute, score, creation_time) = ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) WHERE commentid=$1;", tableName)
)

// Update updates the model in the db
func (m *Model) Update(db *sql.DB) *governor.Error {
	_, err := db.Exec(sqlUpdate, m.Commentid, m.Parentid, m.Postid, m.Userid, m.Content, m.Up, m.Down, m.Absolute, m.Score, m.CreationTime)
	if err != nil {
		return governor.NewError(moduleIDModUp, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

const (
	moduleIDModDel = moduleIDModel + ".Delete"
)

var (
	sqlDelete = fmt.Sprintf("DELETE FROM %s WHERE commentid=$1;", tableName)
)

// Delete deletes the model in the db
func (m *Model) Delete(db *sql.DB) *governor.Error {
	_, err := db.Exec(sqlDelete, m.Commentid)
	if err != nil {
		return governor.NewError(moduleIDModDel, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

const (
	moduleIDModDelPost = moduleIDModel + ".DeletePostComments"
)

var (
	sqlDeletePost = fmt.Sprintf("DELETE FROM %s WHERE postid=$1;", tableName)
)

// DeletePostComments deletes all the comments of a post
func DeletePostComments(db *sql.DB, postid []byte) *governor.Error {
	_, err := db.Exec(sqlDeletePost, postid)
	if err != nil {
		return governor.NewError(moduleIDModDelPost, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

const (
	moduleIDSetup = moduleID + ".Setup"
)

var (
	sqlSetup = fmt.Sprintf("CREATE TABLE %s (commentid BYTEA PRIMARY KEY, parentid BYTEA NOT NULL, postid BYTEA NOT NULL, userid VARCHAR(31) NOT NULL, content VARCHAR(131072) NOT NULL, up INT NOT NULL, down INT NOT NULL, absolute INT NOT NULL, score BIGINT NOT NULL, creation_time BIGINT NOT NULL);", tableName)
)

// Setup creates a new Post table
func Setup(db *sql.DB) *governor.Error {
	_, err := db.Exec(sqlSetup)
	if err != nil {
		return governor.NewError(moduleIDSetup, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}
