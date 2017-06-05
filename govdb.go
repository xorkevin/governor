package governor

import (
	"database/sql"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"net/http"
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

// NewDB creates a new db service
func NewDB(c *Config) (*Database, error) {
	db, err := sql.Open("postgres", c.PostgresURL)
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
func (db *Database) Mount(conf Config, r *echo.Group, l *logrus.Logger) error {
	return nil
}

const (
	moduleIDHealth = moduleID + ".Health"
)

// Health is a health check for the service
func (db *Database) Health() *Error {
	if _, err := db.db.Exec("SELECT 1;"); err != nil {
		return NewError(moduleIDHealth, err.Error(), 0, http.StatusServiceUnavailable)
	}
	return nil
}

// DB returns the sql database instance
func (db *Database) DB() *sql.DB {
	return db.db
}
