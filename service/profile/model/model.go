package profilemodel

import (
	"database/sql"
	"fmt"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/db"
	"github.com/hackform/governor/service/user/model"
	"github.com/lib/pq"
	"net/http"
)

const (
	tableName     = "profiles"
	moduleID      = "profilemodel"
	moduleIDModel = moduleID + ".Model"
)

type (
	Repo interface {
		New(userid, email, bio string) (*Model, *governor.Error)
		GetByIDB64(idb64 string) (*Model, *governor.Error)
		Insert(m *Model) *governor.Error
		Update(m *Model) *governor.Error
		Delete(m *Model) *governor.Error
		Setup() *governor.Error
	}

	repo struct {
		db *sql.DB
	}

	// Model is the db profile model
	Model struct {
		Userid []byte `json:"userid"`
		Email  string `json:"contact_email"`
		Bio    string `json:"bio"`
		Image  string `json:"profile_image_url"`
	}
)

// New creates a new profile repo
func New(conf governor.Config, l governor.Logger, db db.Database) Repo {
	l.Info("initialized profile repo", moduleID, "initialize profile repo", 0, nil)
	return &repo{
		db: db.DB(),
	}
}

// New creates a new profile model
func (r *repo) New(userid, email, bio string) (*Model, *governor.Error) {
	m := Model{
		Email: email,
		Bio:   bio,
	}
	if err := m.SetIDB64(userid); err != nil {
		err.SetErrorUser()
		return nil, err
	}
	return &m, nil
}

const (
	moduleIDModSetIDB64 = moduleIDModel + ".SetIDB64"
)

// SetIDB64 sets the userid of the model from a base64 value
func (m *Model) SetIDB64(idb64 string) *governor.Error {
	u, err := usermodel.ParseB64ToUID(idb64)
	if err != nil {
		err.AddTrace(moduleIDModSetIDB64)
		return err
	}
	m.Userid = u.Bytes()
	return nil
}

const (
	moduleIDModB64 = moduleIDModel + ".IDBase64"
)

// IDBase64 returns the userid as a base64 encoded string
func (m *Model) IDBase64() (string, *governor.Error) {
	u, err := usermodel.ParseUIDToB64(m.Userid)
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
	sqlGetByIDB64 = fmt.Sprintf("SELECT userid, contact_email, bio, profile_image_url FROM %s WHERE userid=$1;", tableName)
)

// GetByIDB64 returns a profile model with the given base64 id
func (r *repo) GetByIDB64(idb64 string) (*Model, *governor.Error) {
	u, err := usermodel.ParseB64ToUID(idb64)
	if err != nil {
		err.AddTrace(moduleIDModGet64)
		return nil, err
	}
	mUser := &Model{}
	if err := r.db.QueryRow(sqlGetByIDB64, u.Bytes()).Scan(&mUser.Userid, &mUser.Email, &mUser.Bio, &mUser.Image); err != nil {
		if err == sql.ErrNoRows {
			return nil, governor.NewError(moduleIDModGet64, "no profile found with that id", 2, http.StatusNotFound)
		}
		return nil, governor.NewError(moduleIDModGet64, err.Error(), 0, http.StatusInternalServerError)
	}
	return mUser, nil
}

const (
	moduleIDModIns = moduleIDModel + ".Insert"
)

var (
	sqlInsert = fmt.Sprintf("INSERT INTO %s (userid, contact_email, bio, profile_image_url) VALUES ($1, $2, $3, $4);", tableName)
)

// Insert inserts the model into the db
func (r *repo) Insert(m *Model) *governor.Error {
	_, err := r.db.Exec(sqlInsert, m.Userid, m.Email, m.Bio, m.Image)
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
	sqlUpdate = fmt.Sprintf("UPDATE %s SET (userid, contact_email, bio, profile_image_url) = ($1, $2, $3, $4) WHERE userid = $1;", tableName)
)

// Update updates the model in the db
func (r *repo) Update(m *Model) *governor.Error {
	_, err := r.db.Exec(sqlUpdate, m.Userid, m.Email, m.Bio, m.Image)
	if err != nil {
		return governor.NewError(moduleIDModUp, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

const (
	moduleIDModDel = moduleIDModel + ".Delete"
)

var (
	sqlDelete = fmt.Sprintf("DELETE FROM %s WHERE userid = $1;", tableName)
)

// Delete deletes the model in the db
func (r *repo) Delete(m *Model) *governor.Error {
	_, err := r.db.Exec(sqlDelete, m.Userid)
	if err != nil {
		return governor.NewError(moduleIDModDel, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

const (
	moduleIDSetup = moduleID + ".Setup"
)

var (
	sqlSetup = fmt.Sprintf("CREATE TABLE %s (userid BYTEA PRIMARY KEY, contact_email VARCHAR(4096), bio VARCHAR(4096), profile_image_url VARCHAR(4096));", tableName)
)

// Setup creates a new Profile table
func (r *repo) Setup() *governor.Error {
	_, err := r.db.Exec(sqlSetup)
	if err != nil {
		return governor.NewError(moduleIDSetup, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}
