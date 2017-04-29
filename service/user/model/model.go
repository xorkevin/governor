package usermodel

import (
	"database/sql"
	"fmt"
	"github.com/hackform/governor"
	"github.com/hackform/governor/util/hash"
	"github.com/hackform/governor/util/rank"
	"github.com/hackform/governor/util/uid"
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
		Hash    []byte `json:"pass_hash"`
		Salt    []byte `json:"pass_salt"`
		Version int    `json:"pass_version"`
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

	mHash, mSalt, mVersion, err := hash.Hash(password, hash.Latest)
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
			Hash:    mHash,
			Salt:    mSalt,
			Version: mVersion,
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
	moduleIDModIns = moduleIDModel + ".Insert"
)

// Insert inserts the model into the db
func (m *Model) Insert(db *sql.DB) *governor.Error {
	_, err := db.Exec(fmt.Sprintf("INSERT INTO %s (userid, username, auth_tags, pass_hash, pass_salt, pass_version, email, first_name, last_name, creation_time) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);", tableName), m.Userid, m.Username, m.Auth.Tags, m.Passhash.Hash, m.Passhash.Salt, m.Passhash.Version, m.Email, m.FirstName, m.LastName, m.CreationTime)
	if err != nil {
		return governor.NewError(moduleIDModIns, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

const (
	moduleIDModUp = moduleIDModel + ".Update"
)

// Update updates the model in the db
func (m *Model) Update(db *sql.DB) *governor.Error {
	_, err := db.Exec(fmt.Sprintf("UPDATE %s SET (userid, username, auth_tags, pass_hash, pass_salt, pass_version, email, first_name, last_name, creation_time) = ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) WHERE userid = $1;", tableName), m.Userid, m.Username, m.Auth.Tags, m.Passhash.Hash, m.Passhash.Salt, m.Passhash.Version, m.Email, m.FirstName, m.LastName, m.CreationTime)
	if err != nil {
		return governor.NewError(moduleIDModUp, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

const (
	moduleIDSetup = moduleID + ".Setup"
)

// Setup creates a new User table
func Setup(db *sql.DB) *governor.Error {
	_, err := db.Exec(fmt.Sprintf("CREATE TABLE %s (userid BYTEA PRIMARY KEY, username VARCHAR(255) NOT NULL, auth_tags TEXT NOT NULL, pass_hash BYTEA NOT NULL, pass_salt BYTEA NOT NULL, pass_version INT NOT NULL, email VARCHAR(255) NOT NULL, first_name VARCHAR(255) NOT NULL, last_name VARCHAR(255) NOT NULL, creation_time BIGINT NOT NULL);", tableName))
	if err != nil {
		return governor.NewError(moduleIDSetup, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}
