package profilemodel

import (
	"database/sql"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/db"
	"github.com/hackform/governor/service/user/model"
	"net/http"
)

//go:generate go run ../../../gen/model.go -- model_gen.go profile profiles Model

const (
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
		Userid []byte `model:"userid,BYTEA PRIMARY KEY"`
		Email  string `model:"contact_email,VARCHAR(4096)"`
		Bio    string `model:"bio,VARCHAR(4096)"`
		Image  string `model:"profile_image_url,VARCHAR(4096)"`
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

// GetByIDB64 returns a profile model with the given base64 id
func (r *repo) GetByIDB64(idb64 string) (*Model, *governor.Error) {
	u, err := usermodel.ParseB64ToUID(idb64)
	if err != nil {
		err.AddTrace(moduleIDModGet64)
		return nil, err
	}
	var m *Model
	if mProfile, code, err := profileModelGet(r.db, u.Bytes()); err != nil {
		if code == 2 {
			return nil, governor.NewError(moduleIDModGet64, "no profile found with that id", 2, http.StatusNotFound)
		}
		return nil, governor.NewError(moduleIDModGet64, err.Error(), 0, http.StatusInternalServerError)
	} else {
		m = mProfile
	}
	return m, nil
}

const (
	moduleIDModIns = moduleIDModel + ".Insert"
)

// Insert inserts the model into the db
func (r *repo) Insert(m *Model) *governor.Error {
	if code, err := profileModelInsert(r.db, m); err != nil {
		if code == 3 {
			return governor.NewErrorUser(moduleIDModIns, err.Error(), 3, http.StatusBadRequest)
		}
		return governor.NewError(moduleIDModIns, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

const (
	moduleIDModUp = moduleIDModel + ".Update"
)

// Update updates the model in the db
func (r *repo) Update(m *Model) *governor.Error {
	if err := profileModelUpdate(r.db, m); err != nil {
		return governor.NewError(moduleIDModUp, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

const (
	moduleIDModDel = moduleIDModel + ".Delete"
)

// Delete deletes the model in the db
func (r *repo) Delete(m *Model) *governor.Error {
	if err := profileModelDelete(r.db, m); err != nil {
		return governor.NewError(moduleIDModDel, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

const (
	moduleIDSetup = moduleID + ".Setup"
)

// Setup creates a new Profile table
func (r *repo) Setup() *governor.Error {
	if err := profileModelSetup(r.db); err != nil {
		return governor.NewError(moduleIDSetup, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}
