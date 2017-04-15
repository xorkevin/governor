package confmodel

import (
	"database/sql"
	"time"
)

const (
	// Latest holds the value of the latest version
	Latest = 1
)

type (
	config struct {
		version int
	}
)

var (
	v001 = &config{
		version: 1,
	}

	latestConfig = v001
)

func newConfig(version int) *config {
	switch version {
	case v001.version:
		return v001
	default:
		return latestConfig
	}
}

func (c *config) Version() int {
	return c.version
}

type (
	// Model is the db User model
	Model struct {
		Version int `json:"version"`
		Props
	}

	// Props stores user info
	Props struct {
		Orgname      string `json:"orgname"`
		CreationDate int64  `json:"creation_date"`
	}
)

// New creates a new Conf Model
func New(orgname string, version int) (*Model, error) {
	c := newConfig(version)
	return &Model{
		Version: c.version,
		Props: Props{
			Orgname:      orgname,
			CreationDate: time.Now().Unix(),
		},
	}, nil
}

// Insert inserts the model into the db
func (m *Model) Insert(db *sql.DB) error {
	return nil
}

// Update updates the model in the db
func (m *Model) Update(db *sql.DB) error {
	return nil
}

// Setup creates a new User table
func Setup(db *sql.DB) error {
	return nil
}
