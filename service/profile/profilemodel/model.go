package profilemodel

import (
	"context"
	"errors"

	"xorkevin.dev/governor/service/dbsql"
	"xorkevin.dev/kerrors"
)

//go:generate forge model

type (
	// Repo is a profile repository
	Repo interface {
		New(userid, email, bio string) *Model
		GetByID(ctx context.Context, userid string) (*Model, error)
		GetBulk(ctx context.Context, userids []string) ([]Model, error)
		Insert(ctx context.Context, m *Model) error
		Update(ctx context.Context, m *Model) error
		Delete(ctx context.Context, m *Model) error
		Setup(ctx context.Context) error
	}

	repo struct {
		table *profileModelTable
		db    dbsql.Database
	}

	// Model is the db profile model
	// Image should be mostly be under 1024
	//forge:model profile
	//forge:model:query profile
	Model struct {
		Userid string `model:"userid,VARCHAR(31) PRIMARY KEY"`
		Email  string `model:"contact_email,VARCHAR(255)"`
		Bio    string `model:"bio,VARCHAR(4095)"`
		Image  string `model:"profile_image_url,VARCHAR(4095)"`
	}
)

// New creates a new profile repo
func New(db dbsql.Database, table string) Repo {
	return &repo{
		table: &profileModelTable{
			TableName: table,
		},
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
func (r *repo) GetByID(ctx context.Context, userid string) (*Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelByID(ctx, d, userid)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get profile")
	}
	return m, nil
}

func (r *repo) GetBulk(ctx context.Context, userids []string) ([]Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelByIDs(ctx, d, userids, len(userids), 0)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get profiles of userids")
	}
	return m, nil
}

// Insert inserts the model into the db
func (r *repo) Insert(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.Insert(ctx, d, m); err != nil {
		return kerrors.WithMsg(err, "Failed to insert profile")
	}
	return nil
}

// Update updates the model in the db
func (r *repo) Update(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.UpdModelByID(ctx, d, m, m.Userid); err != nil {
		return kerrors.WithMsg(err, "Failed to update profile")
	}
	return nil
}

// Delete deletes the model in the db
func (r *repo) Delete(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelByID(ctx, d, m.Userid); err != nil {
		return kerrors.WithMsg(err, "Failed to delete profile")
	}
	return nil
}

// Setup creates a new Profile table
func (r *repo) Setup(ctx context.Context) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.Setup(ctx, d); err != nil {
		err = kerrors.WithMsg(err, "Failed to setup profile model")
		if !errors.Is(err, dbsql.ErrAuthz) {
			return err
		}
	}
	return nil
}
