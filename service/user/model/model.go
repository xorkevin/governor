package usermodel

import (
	"database/sql"
	"fmt"
	"github.com/hackform/governor"
	"github.com/hackform/governor/util/hash"
	"github.com/hackform/governor/util/rank"
	"github.com/hackform/governor/util/uid"
	"github.com/lib/pq"
	"net/http"
	"time"
)

const (
	uidTimeSize   = 8
	uidRandSize   = 8
	tableName     = "users"
	moduleID      = "usermodel"
	moduleIDModel = moduleID + ".Model"
)

type (
	// Model is the db User model
	Model struct {
		ID
		Auth
		Passhash
		Props
	}

	// ID is user identification
	ID struct {
		Userid   []byte `json:"userid"`
		Username string `json:"username"`
	}

	// Auth manages user permissions
	Auth struct {
		Tags string `json:"auth_tags"`
	}

	// Passhash controls the user password
	Passhash struct {
		Hash []byte `json:"pass_hash"`
	}

	// Props stores user info
	Props struct {
		Email        string `json:"email"`
		FirstName    string `json:"first_name"`
		LastName     string `json:"last_name"`
		CreationTime int64  `json:"creation_time"`
	}
)

const (
	moduleIDModNew = moduleIDModel + ".New"
)

// New creates a new User Model
func New(username, password, email, firstname, lastname string, r rank.Rank) (*Model, *governor.Error) {
	mUID, err := uid.NewU(uidTimeSize, uidRandSize)
	if err != nil {
		err.AddTrace(moduleIDModNew)
		return nil, err
	}

	mHash, err := hash.Hash(password)
	if err != nil {
		err.AddTrace(moduleIDModNew)
		return nil, err
	}

	return &Model{
		ID: ID{
			Userid:   mUID.Bytes(),
			Username: username,
		},
		Auth: Auth{
			Tags: r.Stringify(),
		},
		Passhash: Passhash{
			Hash: mHash,
		},
		Props: Props{
			Email:        email,
			FirstName:    firstname,
			LastName:     lastname,
			CreationTime: time.Now().Unix(),
		},
	}, nil
}

// NewBaseUser creates a new Base User Model
func NewBaseUser(username, password, email, firstname, lastname string) (*Model, *governor.Error) {
	return New(username, password, email, firstname, lastname, rank.BaseUser())
}

// NewAdmin creates a new Admin User Model
func NewAdmin(username, password, email, firstname, lastname string) (*Model, *governor.Error) {
	return New(username, password, email, firstname, lastname, rank.Admin())
}

const (
	moduleIDModB64 = moduleIDModel + ".IDBase64"
)

// IDBase64 returns the userid as a base64 encoded string
func (m *Model) IDBase64() (string, *governor.Error) {
	u, err := uid.FromBytes(uidTimeSize, 0, uidRandSize, m.Userid)
	if err != nil {
		err.AddTrace(moduleIDModB64)
		return "", err
	}
	return u.Base64(), nil
}

const (
	moduleIDModGet64 = moduleIDModel + ".GetByIDB64"
)

var (
	sqlGetByIDB64 = fmt.Sprintf("SELECT userid, username, auth_tags, pass_hash, email, first_name, last_name, creation_time FROM %s WHERE userid=$1;", tableName)
)

// GetByIDB64 returns a user model with the given base64 id
func GetByIDB64(db *sql.DB, idb64 string) (*Model, *governor.Error) {
	u, err := uid.FromBase64(uidTimeSize, 0, uidRandSize, idb64)
	if err != nil {
		err.AddTrace(moduleIDModGet64)
		return nil, err
	}
	mUser := &Model{}
	if err := db.QueryRow(sqlGetByIDB64, u.Bytes()).Scan(&mUser.Userid, &mUser.Username, &mUser.Auth.Tags, &mUser.Passhash.Hash, &mUser.Email, &mUser.FirstName, &mUser.LastName, &mUser.CreationTime); err != nil {
		if err == sql.ErrNoRows {
			return nil, governor.NewError(moduleIDModGet64, "no user found with that id", 0, http.StatusNotFound)
		}
		return nil, governor.NewError(moduleIDModGet64, err.Error(), 0, http.StatusInternalServerError)
	}
	return mUser, nil
}

const (
	moduleIDModGetUN = moduleIDModel + ".GetByUsername"
)

var (
	sqlGetByUsername = fmt.Sprintf("SELECT userid, username, auth_tags, pass_hash, email, first_name, last_name, creation_time FROM %s WHERE username=$1;", tableName)
)

// GetByUsername returns a user model with the given username
func GetByUsername(db *sql.DB, username string) (*Model, *governor.Error) {
	mUser := &Model{}
	if err := db.QueryRow(sqlGetByUsername, username).Scan(&mUser.Userid, &mUser.Username, &mUser.Auth.Tags, &mUser.Passhash.Hash, &mUser.Email, &mUser.FirstName, &mUser.LastName, &mUser.CreationTime); err != nil {
		if err == sql.ErrNoRows {
			return nil, governor.NewErrorUser(moduleIDModGet64, "no user found with that username", 0, http.StatusNotFound)
		}
		return nil, governor.NewError(moduleIDModGet64, err.Error(), 0, http.StatusInternalServerError)
	}
	return mUser, nil
}

const (
	moduleIDModIns = moduleIDModel + ".Insert"
)

var (
	sqlInsert = fmt.Sprintf("INSERT INTO %s (userid, username, auth_tags, pass_hash, email, first_name, last_name, creation_time) VALUES ($1, $2, $3, $4, $5, $6, $7, $8);", tableName)
)

// Insert inserts the model into the db
func (m *Model) Insert(db *sql.DB) *governor.Error {
	_, err := db.Exec(sqlInsert, m.Userid, m.Username, m.Auth.Tags, m.Passhash.Hash, m.Email, m.FirstName, m.LastName, m.CreationTime)
	if err != nil {
		if postgresErr, ok := err.(*pq.Error); ok {
			switch postgresErr.Code {
			case "23505": // unique_violation
				return governor.NewErrorUser(moduleIDModIns, err.Error(), 0, http.StatusBadRequest)
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
	sqlUpdate = fmt.Sprintf("UPDATE %s SET (userid, username, auth_tags, pass_hash, email, first_name, last_name, creation_time) = ($1, $2, $3, $4, $5, $6, $7, $8) WHERE userid = $1;", tableName)
)

// Update updates the model in the db
func (m *Model) Update(db *sql.DB) *governor.Error {
	_, err := db.Exec(sqlUpdate, m.Userid, m.Username, m.Auth.Tags, m.Passhash.Hash, m.Email, m.FirstName, m.LastName, m.CreationTime)
	if err != nil {
		return governor.NewError(moduleIDModUp, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

const (
	moduleIDSetup = moduleID + ".Setup"
)

var (
	sqlSetup = fmt.Sprintf("CREATE TABLE %s (userid BYTEA PRIMARY KEY, username VARCHAR(255) NOT NULL UNIQUE, auth_tags TEXT NOT NULL, pass_hash BYTEA NOT NULL, email VARCHAR(255) NOT NULL UNIQUE, first_name VARCHAR(255) NOT NULL, last_name VARCHAR(255) NOT NULL, creation_time BIGINT NOT NULL);", tableName)
)

// Setup creates a new User table
func Setup(db *sql.DB) *governor.Error {
	_, err := db.Exec(sqlSetup)
	if err != nil {
		return governor.NewError(moduleIDSetup, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}
