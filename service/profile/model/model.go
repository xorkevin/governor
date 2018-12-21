package profilemodel

import (
	"database/sql"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/db"
	"net/http"
)

//go:generate forge model -m Model -t profiles -p profile -o model_gen.go

const (
	moduleID      = "profilemodel"
	moduleIDModel = moduleID + ".Model"
)

type (
	Repo interface {
		New(userid, email, bio string) (*Model, *governor.Error)
		GetByID(userid string) (*Model, *governor.Error)
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
		Userid string `model:"userid,VARCHAR(31) PRIMARY KEY"`
		Email  string `model:"contact_email,VARCHAR(255)"`
		Bio    string `model:"bio,VARCHAR(4095)"`
		Image  string `model:"profile_image_url,VARCHAR(4095)"`
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
	return &Model{
		Userid: userid,
		Email:  email,
		Bio:    bio,
	}, nil
}

const (
	moduleIDModGet = moduleIDModel + ".GetByID"
)

// GetByID returns a profile model with the given base64 id
func (r *repo) GetByID(userid string) (*Model, *governor.Error) {
	var m *Model
	if mProfile, code, err := profileModelGet(r.db, userid); err != nil {
		if code == 2 {
			return nil, governor.NewError(moduleIDModGet, "no profile found with that id", 2, http.StatusNotFound)
		}
		return nil, governor.NewError(moduleIDModGet, err.Error(), 0, http.StatusInternalServerError)
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
