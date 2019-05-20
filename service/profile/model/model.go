package profilemodel

import (
	"database/sql"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/db"
	"net/http"
)

//go:generate forge model -m Model -t profiles -p profile -o model_gen.go

type (
	Repo interface {
		New(userid, email, bio string) (*Model, error)
		GetByID(userid string) (*Model, error)
		Insert(m *Model) error
		Update(m *Model) error
		Delete(m *Model) error
		Setup() error
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
	l.Info("initialize profile repo", nil)
	return &repo{
		db: db.DB(),
	}
}

// New creates a new profile model
func (r *repo) New(userid, email, bio string) (*Model, error) {
	return &Model{
		Userid: userid,
		Email:  email,
		Bio:    bio,
	}, nil
}

// GetByID returns a profile model with the given base64 id
func (r *repo) GetByID(userid string) (*Model, error) {
	var m *Model
	if mProfile, code, err := profileModelGet(r.db, userid); err != nil {
		if code == 2 {
			return nil, governor.NewError("No profile found with that id", http.StatusNotFound, err)
		}
		return nil, governor.NewError("Failed to get profile", http.StatusInternalServerError, err)
	} else {
		m = mProfile
	}
	return m, nil
}

// Insert inserts the model into the db
func (r *repo) Insert(m *Model) error {
	if code, err := profileModelInsert(r.db, m); err != nil {
		if code == 3 {
			return governor.NewErrorUser("Profile id must be unique", http.StatusBadRequest, err)
		}
		return governor.NewError("Failed to insert profile", http.StatusInternalServerError, err)
	}
	return nil
}

// Update updates the model in the db
func (r *repo) Update(m *Model) error {
	if err := profileModelUpdate(r.db, m); err != nil {
		return governor.NewError("Failed to update profile", http.StatusInternalServerError, err)
	}
	return nil
}

// Delete deletes the model in the db
func (r *repo) Delete(m *Model) error {
	if err := profileModelDelete(r.db, m); err != nil {
		return governor.NewError("Failed to delete profile", http.StatusInternalServerError, err)
	}
	return nil
}

// Setup creates a new Profile table
func (r *repo) Setup() error {
	if err := profileModelSetup(r.db); err != nil {
		return governor.NewError("Failed to setup profile model", http.StatusInternalServerError, err)
	}
	return nil
}
