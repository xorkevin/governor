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
		l.Error("db: fail create db", map[string]string{
			"err": err.Error(),
		})
		return nil, err
	}
	if err := db.Ping(); err != nil {
		l.Error("db: fail ping db", map[string]string{
			"err": err.Error(),
		})
		return nil, err
	}

	l.Info(fmt.Sprintf("db: establish connection to %s:%s with user %s", pgconf["host"], pgconf["port"], pgconf["user"]), nil)
	l.Info("initialize database service", nil)

	return &postgresDB{
		db: db,
	}, nil
}

// Mount is a place to mount routes to satisfy the Service interface
func (db *postgresDB) Mount(conf governor.Config, l governor.Logger, r *echo.Group) error {
	l.Info("mount database service", nil)
	return nil
}

// Health is a health check for the service
func (db *postgresDB) Health() error {
	if _, err := db.db.Exec("SELECT 1;"); err != nil {
		return governor.NewError("Failed to connect to db", http.StatusServiceUnavailable, err)
	}
	return nil
}

// Setup is run on service setup
func (db *postgresDB) Setup(conf governor.Config, l governor.Logger, rsetup governor.ReqSetupPost) error {
	return nil
}

// DB returns the sql database instance
func (db *postgresDB) DB() *sql.DB {
	return db.db
}
