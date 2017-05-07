package governor

import (
	"database/sql"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"net/http"
)

type (
	database struct {
		db *sql.DB
	}
)

const (
	moduleID = "database"
)

func newDB(c *Config) (*database, error) {
	db, err := sql.Open("postgres", c.PostgresURL)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return &database{
		db: db,
	}, nil
}

// Mount is a place to mount routes to satisfy the Service interface
func (db *database) Mount(conf Config, r *echo.Group, sdb *sql.DB, l *logrus.Logger) error {
	return nil
}

const (
	moduleIDHealth = moduleID + ".Health"
)

// Health is a health check for the service
func (db *database) Health() *Error {
	if err := db.db.Ping(); err != nil {
		return NewError(moduleIDHealth, err.Error(), 0, http.StatusServiceUnavailable)
	}
	return nil
}
