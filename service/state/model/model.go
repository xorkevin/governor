package model

import (
	"context"
	"errors"
	"time"

	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/state"
	"xorkevin.dev/kerrors"
)

//go:generate forge model -m Model -p state -o model_gen.go Model

const (
	configID = 0
)

type (
	repo struct {
		table *stateModelTable
		db    db.Database
	}

	// Model is the db State model
	Model struct {
		config       int    `model:"config,INT PRIMARY KEY" query:"config;getoneeq,config;updeq,config"`
		Setup        bool   `model:"setup,BOOLEAN NOT NULL" query:"setup"`
		Version      string `model:"version,VARCHAR(255) NOT NULL" query:"version"`
		VHash        string `model:"vhash,VARCHAR(255) NOT NULL" query:"vhash"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL" query:"creation_time"`
	}
)

// New returns a state service backed by a database
func New(database db.Database, table string) state.State {
	return &repo{
		table: &stateModelTable{
			TableName: table,
		},
		db: database,
	}
}

// New creates a new State Model
func (r *repo) New(version string, vhash string) *Model {
	return &Model{
		config:       configID,
		Setup:        false,
		Version:      version,
		VHash:        vhash,
		CreationTime: time.Now().Round(0).Unix(),
	}
}

// GetModel returns the state model
func (r *repo) GetModel(ctx context.Context) (*Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelEqconfig(ctx, d, configID)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get state")
	}
	return m, nil
}

// Insert inserts the model into the db
func (r *repo) Insert(ctx context.Context, m *Model) error {
	m.config = configID
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.Insert(ctx, d, m); err != nil {
		return kerrors.WithMsg(err, "Failed to insert state")
	}
	return nil
}

// Update updates the model in the db
func (r *repo) Update(ctx context.Context, m *Model) error {
	m.config = configID
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.UpdModelEqconfig(ctx, d, m, configID); err != nil {
		return kerrors.WithMsg(err, "Failed to update state")
	}
	return nil
}

// Get retrieves the current server state
func (r *repo) Get(ctx context.Context) (*state.Model, error) {
	m, err := r.GetModel(ctx)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound{}) || errors.Is(err, db.ErrorUndefinedTable{}) {
			return &state.Model{
				Setup: false,
			}, nil
		}
		return nil, err
	}
	return &state.Model{
		Setup:        m.Setup,
		Version:      m.Version,
		VHash:        m.VHash,
		CreationTime: m.CreationTime,
	}, nil
}

// Set updates the server state entry
func (r *repo) Set(ctx context.Context, m *state.Model) error {
	return r.Update(ctx, &Model{
		Setup:        m.Setup,
		Version:      m.Version,
		VHash:        m.VHash,
		CreationTime: m.CreationTime,
	})
}

// Setup creates a new State table if it does not exist and updates the server
// state entry
func (r *repo) Setup(ctx context.Context, req state.ReqSetup) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.Setup(ctx, d); err != nil {
		err = kerrors.WithMsg(err, "Failed to setup state model")
		if !errors.Is(err, db.ErrorAuthz{}) {
			return err
		}
	}
	k := r.New(req.Version, req.VHash)
	k.Setup = true
	if err := r.Insert(ctx, k); err != nil {
		return err
	}
	return nil
}
