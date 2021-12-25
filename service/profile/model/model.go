package model

import (
	"errors"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
)

//go:generate forge model -m Model -p profile -o model_gen.go Model

type (
	// Repo is a profile repository
	Repo interface {
		New(userid, email, bio string) *Model
		GetByID(userid string) (*Model, error)
		GetBulk(userids []string) ([]Model, error)
		Insert(m *Model) error
		Update(m *Model) error
		Delete(m *Model) error
		Setup() error
	}

	repo struct {
		table string
		db    db.Database
	}

	// Model is the db profile model
	// Image should be mostly be under 1024
	Model struct {
		Userid string `model:"userid,VARCHAR(31) PRIMARY KEY" query:"userid;getoneeq,userid;getgroupeq,userid|arr;updeq,userid;deleq,userid"`
		Email  string `model:"contact_email,VARCHAR(255)" query:"contact_email"`
		Bio    string `model:"bio,VARCHAR(4095)" query:"bio"`
		Image  string `model:"profile_image_url,VARCHAR(4095)" query:"profile_image_url"`
	}

	ctxKeyRepo struct{}
)

// GetCtxRepo returns a Repo from the context
func GetCtxRepo(inj governor.Injector) Repo {
	v := inj.Get(ctxKeyRepo{})
	if v == nil {
		return nil
	}
	return v.(Repo)
}

// SetCtxRepo sets a Repo in the context
func SetCtxRepo(inj governor.Injector, r Repo) {
	inj.Set(ctxKeyRepo{}, r)
}

// NewInCtx creates a new profile repo from a context and sets it in the context
func NewInCtx(inj governor.Injector, table string) {
	SetCtxRepo(inj, NewCtx(inj, table))
}

// NewCtx creates a new profile repo from a context
func NewCtx(inj governor.Injector, table string) Repo {
	dbService := db.GetCtxDB(inj)
	return New(dbService, table)
}

// New creates a new profile repo
func New(db db.Database, table string) Repo {
	return &repo{
		table: table,
		db:    db,
	}
}

// New creates a new profile model
func (r *repo) New(userid, email, bio string) *Model {
	return &Model{
		Userid: userid,
		Email:  email,
		Bio:    bio,
	}
}

// GetByID returns a profile model with the given base64 id
func (r *repo) GetByID(userid string) (*Model, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := profileModelGetModelEqUserid(d, r.table, userid)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get profile")
	}
	return m, nil
}

func (r *repo) GetBulk(userids []string) ([]Model, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := profileModelGetModelHasUseridOrdUserid(d, r.table, userids, true, len(userids), 0)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get profiles of userids")
	}
	return m, nil
}

// Insert inserts the model into the db
func (r *repo) Insert(m *Model) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := profileModelInsert(d, r.table, m); err != nil {
		return db.WrapErr(err, "Failed to insert profile")
	}
	return nil
}

// Update updates the model in the db
func (r *repo) Update(m *Model) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := profileModelUpdModelEqUserid(d, r.table, m, m.Userid); err != nil {
		return db.WrapErr(err, "Failed to update profile")
	}
	return nil
}

// Delete deletes the model in the db
func (r *repo) Delete(m *Model) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := profileModelDelEqUserid(d, r.table, m.Userid); err != nil {
		return db.WrapErr(err, "Failed to delete profile")
	}
	return nil
}

// Setup creates a new Profile table
func (r *repo) Setup() error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := profileModelSetup(d, r.table); err != nil {
		err = db.WrapErr(err, "Failed to setup profile model")
		if !errors.Is(err, db.ErrAuthz{}) {
			return err
		}
	}
	return nil
}
