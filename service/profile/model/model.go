package model

import (
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
)

//go:generate forge model -m Model -t profiles -p profile -o model_gen.go Model

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
		db db.Database
	}

	// Model is the db profile model
	// Image should be mostly be under 1024
	Model struct {
		Userid string `model:"userid,VARCHAR(31) PRIMARY KEY" query:"userid,getoneeq,userid;getgroupeq,userid|arr;updeq,userid;deleq,userid"`
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
func NewInCtx(inj governor.Injector) {
	SetCtxRepo(inj, NewCtx(inj))
}

// NewCtx creates a new profile repo from a context
func NewCtx(inj governor.Injector) Repo {
	dbService := db.GetCtxDB(inj)
	return New(dbService)
}

// New creates a new profile repo
func New(db db.Database) Repo {
	return &repo{
		db: db,
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
	m, code, err := profileModelGetModelEqUserid(d, userid)
	if err != nil {
		if code == 2 {
			return nil, governor.ErrWithKind(err, db.ErrNotFound{}, "No profile found with that id")
		}
		return nil, governor.ErrWithKind(err, db.ErrClient{}, "Failed to get profile")
	}
	return m, nil
}

func (r *repo) GetBulk(userids []string) ([]Model, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := profileModelGetModelHasUseridOrdUserid(d, userids, true, len(userids), 0)
	if err != nil {
		return nil, governor.ErrWithKind(err, db.ErrClient{}, "Failed to get profiles of userids")
	}
	return m, nil
}

// Insert inserts the model into the db
func (r *repo) Insert(m *Model) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if code, err := profileModelInsert(d, m); err != nil {
		if code == 3 {
			return governor.ErrWithKind(err, db.ErrUnique{}, "Profile id must be unique")
		}
		return governor.ErrWithKind(err, db.ErrClient{}, "Failed to insert profile")
	}
	return nil
}

// Update updates the model in the db
func (r *repo) Update(m *Model) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if _, err := profileModelUpdModelEqUserid(d, m, m.Userid); err != nil {
		return governor.ErrWithKind(err, db.ErrClient{}, "Failed to update profile")
	}
	return nil
}

// Delete deletes the model in the db
func (r *repo) Delete(m *Model) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := profileModelDelEqUserid(d, m.Userid); err != nil {
		return governor.ErrWithKind(err, db.ErrClient{}, "Failed to delete profile")
	}
	return nil
}

// Setup creates a new Profile table
func (r *repo) Setup() error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := profileModelSetup(d); err != nil {
		return governor.ErrWithKind(err, db.ErrClient{}, "Failed to setup profile model")
	}
	return nil
}
