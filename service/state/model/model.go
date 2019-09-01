package statemodel

import (
	"net/http"
	"time"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/state"
)

//go:generate forge model -m Model -t govstate -p state -o model_gen.go

const (
	configID = 0
)

type (
	repo struct {
		db db.Database
	}

	// Model is the db State model
	Model struct {
		config       int    `model:"config,INT PRIMARY KEY"`
		Orgname      string `model:"orgname,VARCHAR(255) NOT NULL"`
		Setup        bool   `model:"setup,BOOLEAN NOT NULL"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL"`
	}
)

func New(database db.Database) state.State {
	return &repo{
		db: database,
	}
}

// New creates a new State Model
func (r *repo) New(orgname string) *Model {
	return &Model{
		config:       configID,
		Orgname:      orgname,
		Setup:        false,
		CreationTime: time.Now().Unix(),
	}
}

// GetModel returns the state model
func (r *repo) GetModel() (*Model, error) {
	m, code, err := stateModelGet(r.db.DB(), configID)
	if err != nil {
		switch code {
		case 2:
			return nil, governor.NewError("No state found with that id", http.StatusNotFound, err)
		case 4:
			return nil, governor.NewError("No state found with that id", http.StatusNotFound, err)
		default:
			return nil, governor.NewError("Failed to get state", http.StatusInternalServerError, err)
		}
	}
	return m, nil
}

// Insert inserts the model into the db
func (r *repo) Insert(m *Model) error {
	if _, err := stateModelInsert(r.db.DB(), m); err != nil {
		return governor.NewError("Failed to insert state", http.StatusInternalServerError, err)
	}
	return nil
}

// Update updates the model in the db
func (r *repo) Update(m *Model) error {
	if err := stateModelUpdate(r.db.DB(), m); err != nil {
		return governor.NewError("Failed to update state", http.StatusInternalServerError, err)
	}
	return nil
}

func (r *repo) Get() (*state.Model, error) {
	m, err := r.GetModel()
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return &state.Model{
				Setup: false,
			}, nil
		}
		return nil, err
	}
	return &state.Model{
		Orgname:      m.Orgname,
		Setup:        m.Setup,
		CreationTime: m.CreationTime,
	}, nil
}

func (r *repo) Set(m *state.Model) error {
	return r.Update(&Model{
		config:       configID,
		Orgname:      m.Orgname,
		Setup:        m.Setup,
		CreationTime: m.CreationTime,
	})
}

// Setup creates a new State table
func (r *repo) Setup(req state.ReqSetup) error {
	m, err := r.GetModel()
	if err != nil && governor.ErrorStatus(err) != http.StatusNotFound {
		return err
	}
	if err == nil && m.Setup {
		return governor.NewError("Setup already run", http.StatusForbidden, nil)
	}

	if err := stateModelSetup(r.db.DB()); err != nil {
		return governor.NewError("Failed to setup state model", http.StatusInternalServerError, err)
	}
	if err == nil {
		m.Setup = true
		if err := r.Update(m); err != nil {
			return err
		}
	} else {
		k := r.New(req.Orgname)
		k.Setup = true
		if err := r.Insert(k); err != nil {
			return err
		}
	}
	return nil
}
