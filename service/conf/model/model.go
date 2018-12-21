package confmodel

import (
	"database/sql"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/db"
	"net/http"
	"time"
)

//go:generate forge -- model_gen.go conf config Model

const (
	configID      = 0
	moduleID      = "confmodel"
	moduleIDModel = moduleID + ".Model"
)

type (
	Repo interface {
		New(orgname string) (*Model, *governor.Error)
		Get() (*Model, *governor.Error)
		Insert(m *Model) *governor.Error
		Update(m *Model) *governor.Error
		Setup() *governor.Error
	}

	repo struct {
		db *sql.DB
	}

	// Model is the db Config model
	Model struct {
		config       int    `model:"config,INT PRIMARY KEY"`
		Orgname      string `model:"orgname,VARCHAR(255) NOT NULL"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL"`
	}
)

func New(conf governor.Config, l governor.Logger, database db.Database) Repo {
	l.Info("initialized user role model", moduleID, "initialize conf model", 0, nil)
	return &repo{
		db: database.DB(),
	}
}

// New creates a new Conf Model
func (r *repo) New(orgname string) (*Model, *governor.Error) {
	return &Model{
		config:       configID,
		Orgname:      orgname,
		CreationTime: time.Now().Unix(),
	}, nil
}

const (
	moduleIDModGet = moduleIDModel + ".Get"
)

// Get returns the conf model
func (r *repo) Get() (*Model, *governor.Error) {
	var m *Model
	if mConfig, code, err := confModelGet(r.db, configID); err != nil {
		if code == 2 {
			return nil, governor.NewError(moduleIDModGet, "no conf found with that id", 2, http.StatusNotFound)
		}
		return nil, governor.NewError(moduleIDModGet, err.Error(), 0, http.StatusInternalServerError)
	} else {
		m = mConfig
	}
	return m, nil
}

const (
	moduleIDModIns = moduleIDModel + ".Insert"
)

// Insert inserts the model into the db
func (r *repo) Insert(m *Model) *governor.Error {
	if _, err := confModelInsert(r.db, m); err != nil {
		return governor.NewError(moduleIDModIns, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

const (
	moduleIDModUp = moduleIDModel + ".Update"
)

// Update updates the model in the db
func (r *repo) Update(m *Model) *governor.Error {
	if err := confModelUpdate(r.db, m); err != nil {
		return governor.NewError(moduleIDModUp, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

const (
	moduleIDSetup = moduleID + ".Setup"
)

// Setup creates a new Config table
func (r *repo) Setup() *governor.Error {
	if err := confModelSetup(r.db); err != nil {
		return governor.NewError(moduleIDSetup, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}
