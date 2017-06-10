package db

import (
	"database/sql"
	"github.com/hackform/governor"
	"github.com/labstack/echo"
	_ "github.com/lib/pq" // depends upon postgres
	"github.com/sirupsen/logrus"
	"net/http"
	"strings"
)

type (
	// Database is a service wrapper around an sql.DB instance
	Database struct {
		db *sql.DB
	}
)

const (
	moduleID = "database"
)

// New creates a new db service
func New(c governor.Config) (*Database, error) {
	v := c.Conf()
	pgconf := v.GetStringMapString("postgres")
	pgarr := []string{}
	for k, v := range pgconf {
		pgarr = append(pgarr, k+"="+v)
	}
	postgresURL := strings.Join(pgarr, " ")
	db, err := sql.Open("postgres", postgresURL)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return &Database{
		db: db,
	}, nil
}

// Mount is a place to mount routes to satisfy the Service interface
func (db *Database) Mount(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
	return nil
}

const (
	moduleIDHealth = moduleID + ".Health"
)

// Health is a health check for the service
func (db *Database) Health() *governor.Error {
	if _, err := db.db.Exec("SELECT 1;"); err != nil {
		return governor.NewError(moduleIDHealth, err.Error(), 0, http.StatusServiceUnavailable)
	}
	return nil
}

// DB returns the sql database instance
func (db *Database) DB() *sql.DB {
	return db.db
}
