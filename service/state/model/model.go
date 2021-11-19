package model

import (
	"errors"
	"time"

	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/state"
)

//go:generate forge model -m Model -t govstate -p state -o model_gen.go Model

const (
	configID = 0
)

type (
	repo struct {
		db db.Database
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
func New(database db.Database) state.State {
	return &repo{
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
func (r *repo) GetModel() (*Model, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := stateModelGetModelEqconfig(d, configID)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get state")
	}
	return m, nil
}

// Insert inserts the model into the db
func (r *repo) Insert(m *Model) error {
	m.config = configID
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := stateModelInsert(d, m); err != nil {
		return db.WrapErr(err, "Failed to insert state")
	}
	return nil
}

// Update updates the model in the db
func (r *repo) Update(m *Model) error {
	m.config = configID
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := stateModelUpdModelEqconfig(d, m, configID); err != nil {
		return db.WrapErr(err, "Failed to update state")
	}
	return nil
}

// Get retrieves the current server state
func (r *repo) Get() (*state.Model, error) {
	m, err := r.GetModel()
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
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
func (r *repo) Set(m *state.Model) error {
	return r.Update(&Model{
		Setup:        m.Setup,
		Version:      m.Version,
		VHash:        m.VHash,
		CreationTime: m.CreationTime,
	})
}

// Setup creates a new State table if it does not exist and updates the server
// state entry
func (r *repo) Setup(req state.ReqSetup) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := stateModelSetup(d); err != nil {
		err = db.WrapErr(err, "Failed to setup state model")
		if !errors.Is(err, db.ErrAuthz{}) {
			return err
		}
	}
	k := r.New(req.Version, req.VHash)
	k.Setup = true
	if err := r.Insert(k); err != nil {
		return err
	}
	return nil
}
