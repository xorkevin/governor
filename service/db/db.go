package db

import (
	"database/sql"
	"fmt"
	"github.com/hackform/governor"
	"github.com/labstack/echo"
	_ "github.com/lib/pq" // depends upon postgres
	"net/http"
	"strings"
)

type (
	// Database is a service wrapper around an sql.DB instance
	Database interface {
		governor.Service
		DB() *sql.DB
	}

	postgresDB struct {
		db *sql.DB
	}
)

const (
	moduleID = "database"
)

// New creates a new db service
func New(c governor.Config, l governor.Logger) (Database, error) {
	v := c.Conf()
	pgconf := v.GetStringMapString("postgres")
	pgarr := make([]string, 0, len(pgconf))
	for k, v := range pgconf {
		pgarr = append(pgarr, k+"="+v)
	}
	postgresURL := strings.Join(pgarr, " ")
	db, err := sql.Open("postgres", postgresURL)
	if err != nil {
		l.Error(err.Error(), moduleID, "fail create db", 0, nil)
		return nil, err
	}
	if err := db.Ping(); err != nil {
		l.Error(err.Error(), moduleID, "fail ping db", 0, nil)
		return nil, err
	}

	l.Info(fmt.Sprintf("db: connected to %s:%s with user %s", pgconf["host"], pgconf["port"], pgconf["user"]), moduleID, "establish db connection", 0, nil)
	l.Info("initialized database", moduleID, "initialize database service", 0, nil)

	return &postgresDB{
		db: db,
	}, nil
}

// Mount is a place to mount routes to satisfy the Service interface
func (db *postgresDB) Mount(conf governor.Config, l governor.Logger, r *echo.Group) error {
	l.Info("mounted database", moduleID, "mount database service", 0, nil)
	return nil
}

const (
	moduleIDHealth = moduleID + ".health"
)

// Health is a health check for the service
func (db *postgresDB) Health() *governor.Error {
	if _, err := db.db.Exec("SELECT 1;"); err != nil {
		return governor.NewError(moduleIDHealth, err.Error(), 0, http.StatusServiceUnavailable)
	}
	return nil
}

// Setup is run on service setup
func (db *postgresDB) Setup(conf governor.Config, l governor.Logger, rsetup governor.ReqSetupPost) *governor.Error {
	return nil
}

// DB returns the sql database instance
func (db *postgresDB) DB() *sql.DB {
	return db.db
}
