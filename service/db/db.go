package db

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/labstack/echo/v4"
	_ "github.com/lib/pq" // depends upon postgres
	"net/http"
	"sync"
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

	pgauth struct {
		username string
		password string
	}

	service struct {
		db      *sql.DB
		auth    pgauth
		dbname  string
		host    string
		port    string
		sslmode string
		mu      *sync.RWMutex
		config  governor.SecretReader
		logger  governor.Logger
		done    <-chan struct{}
	}
)

// New creates a new db service
func New() Service {
	return &service{
		mu: &sync.RWMutex{},
	}
}

func (s *service) Register(r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	r.SetDefault("user", "postgres")
	r.SetDefault("password", "admin")
	r.SetDefault("dbname", "postgres")
	r.SetDefault("host", "localhost")
	r.SetDefault("port", "5432")
	r.SetDefault("sslmode", "disable")
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, g *echo.Group) error {
	s.logger = l
	l = s.logger.WithData(map[string]string{
		"phase": "init",
	})

	s.config = r

	conf := r.GetStrMap("")
	s.dbname = conf["dbname"]
	s.host = conf["host"]
	s.port = conf["port"]
	s.sslmode = conf["sslmode"]

	if err := s.authPostgres(); err != nil {
		return err
	}

	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		s.mu.Lock()
		defer s.mu.Unlock()

		s.closeDBLocked()
		close(done)
	}()
	s.done = done
	return nil
}

func (s *service) closeDBLocked() {
	if s.db == nil {
		return
	}
	if err := s.db.Close(); err != nil {
		s.logger.Error("failed to close db connection", map[string]string{
			"error":      err.Error(),
			"actiontype": "closedb",
			"username":   s.auth.username,
		})
	} else {
		s.logger.Info("closed connection", map[string]string{
			"actiontype": "closedb",
			"username":   s.auth.username,
		})
	}
	s.db = nil
}

func (s *service) ensureValidAuth() error {
	if ok, err := s.authPostgresValid(); err != nil {
		return err
	} else if ok {
		return nil
	}
	return s.authPostgres()
}

func (s *service) getPostgresAuth() (pgauth, error) {
	secret, err := s.config.GetSecret("auth")
	if err != nil {
		return pgauth{}, err
	}
	return pgauth{
		username: secret["username"].(string),
		password: secret["password"].(string),
	}, nil
}

func (s *service) authPostgresValidLocked() (bool, error) {
	auth, err := s.getPostgresAuth()
	if err != nil {
		return false, err
	}
	return auth == s.auth, nil
}

func (s *service) authPostgresValid() (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.authPostgresValidLocked()
}

func (s *service) authPostgres() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if ok, err := s.authPostgresValidLocked(); err != nil {
		return err
	} else if ok {
		return nil
	}

	auth, err := s.getPostgresAuth()
	if err != nil {
		return err
	}

	connection := fmt.Sprintf("user=%s password=%s dbname=%s host=%s port=%s sslmode=%s", auth.username, auth.password, s.dbname, s.host, s.port, s.sslmode)
	db, err := sql.Open("postgres", connection)
	if err != nil {
		return governor.NewError("Failed to init postgres conn", http.StatusInternalServerError, err)
	}

	if err := db.Ping(); err != nil {
		return governor.NewError("Failed to ping db", http.StatusInternalServerError, err)
	}

	s.closeDBLocked()

	s.db = db
	s.auth = auth

	s.logger.Info(fmt.Sprintf("established connection to %s:%s with user %s", s.host, s.port, s.auth.username), nil)
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
