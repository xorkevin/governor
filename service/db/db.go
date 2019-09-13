package db

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/labstack/echo/v4"
	_ "github.com/lib/pq" // depends upon postgres
	"net/http"
	"strings"
	"xorkevin.dev/governor"
)

type (
	// Database is a service wrapper around an sql.DB instance
	//
	// DB returns the wrapped sql database instance
	Database interface {
		DB() *sql.DB
	}

	Service interface {
		governor.Service
		Database
	}

	service struct {
		db     *sql.DB
		logger governor.Logger
		done   <-chan struct{}
	}
)

// New creates a new db service
func New() Service {
	return &service{}
}

func (s *service) Register(r governor.ConfigRegistrar) {
	r.SetDefault("user", "postgres")
	r.SetDefault("password", "admin")
	r.SetDefault("dbname", "governor")
	r.SetDefault("host", "localhost")
	r.SetDefault("port", "5432")
	r.SetDefault("sslmode", "disable")
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, g *echo.Group) error {
	s.logger = l
	l = s.logger.WithData(map[string]string{
		"phase": "init",
	})
	conf := r.GetStrMap("")
	pgarr := make([]string, 0, len(conf))
	for k, v := range conf {
		pgarr = append(pgarr, k+"="+v)
	}
	postgresURL := strings.Join(pgarr, " ")
	db, err := sql.Open("postgres", postgresURL)
	if err != nil {
		return governor.NewError("Failed to init postgres conn", http.StatusInternalServerError, err)
	}
	s.db = db

	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		l := s.logger.WithData(map[string]string{
			"phase": "stop",
		})
		if err := s.db.Close(); err != nil {
			l.Error("failed to close db connection", map[string]string{
				"error": err.Error(),
			})
		} else {
			l.Info("closed connection", nil)
		}
		done <- struct{}{}
	}()
	s.done = done

	l.Info("opened database connection", nil)

	if err := db.Ping(); err != nil {
		return governor.NewError("Failed to ping db", http.StatusInternalServerError, err)
	}
	l.Info("ping database success", nil)

	l.Info(fmt.Sprintf("established connection to %s:%s with user %s", conf["host"], conf["port"], conf["user"]), nil)
	return nil
}

func (s *service) Setup(req governor.ReqSetup) error {
	return nil
}

func (s *service) Start(ctx context.Context) error {
	return nil
}

func (s *service) Stop(ctx context.Context) {
	l := s.logger.WithData(map[string]string{
		"phase": "stop",
	})
	select {
	case <-s.done:
		return
	case <-ctx.Done():
		l.Warn("failed to stop", nil)
	}
}

func (s *service) Health() error {
	if _, err := s.db.Exec("SELECT 1;"); err != nil {
		return governor.NewError("Failed to connect to db", http.StatusInternalServerError, err)
	}
	return nil
}

// DB implements Database.DB by returning its wrapped sql.DB
func (s *service) DB() *sql.DB {
	return s.db
}
