package confmodel

import (
	"database/sql"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/db"
	"github.com/lib/pq"
	"net/http"
	"time"
)

//go:generate forge model -m Model -t config -p conf -o model_gen.go

const (
	configID = 0
)

type (
	Repo interface {
		New(orgname string) (*Model, error)
		Get() (*Model, error)
		Insert(m *Model) error
		Update(m *Model) error
		Setup() error
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
	l.Info("initialize conf model", nil)
	return &repo{
		db: database.DB(),
	}
}

// New creates a new Conf Model
func (r *repo) New(orgname string) (*Model, error) {
	return &Model{
		config:       configID,
		Orgname:      orgname,
		CreationTime: time.Now().Unix(),
	}, nil
}

// Get returns the conf model
func (r *repo) Get() (*Model, error) {
	var m *Model
	if mConfig, code, err := confModelGet(r.db, configID); err != nil {
		if code == 2 {
			return nil, governor.NewError("No conf found with that id", http.StatusNotFound, err)
		}
		if postgresErr, ok := err.(*pq.Error); ok {
			// undefined_table error
			if postgresErr.Code == "42P01" {
				return nil, governor.NewError("No conf found with that id", http.StatusNotFound, postgresErr)
			}
		}
		return nil, governor.NewError("Failed to get conf", http.StatusInternalServerError, err)
	} else {
		m = mConfig
	}
	return m, nil
}

// Insert inserts the model into the db
func (r *repo) Insert(m *Model) error {
	if _, err := confModelInsert(r.db, m); err != nil {
		return governor.NewError("Failed to insert conf", http.StatusInternalServerError, err)
	}
	return nil
}

// Update updates the model in the db
func (r *repo) Update(m *Model) error {
	if err := confModelUpdate(r.db, m); err != nil {
		return governor.NewError("Failed to update conf", http.StatusInternalServerError, err)
	}
	return nil
}

// Setup creates a new Config table
func (r *repo) Setup() error {
	if err := confModelSetup(r.db); err != nil {
		return governor.NewError("Failed to setup conf model", http.StatusInternalServerError, err)
	}
	return nil
}
