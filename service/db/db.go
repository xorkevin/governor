package db

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/labstack/echo/v4"
	_ "github.com/lib/pq" // depends upon postgres
	"net/http"
	"xorkevin.dev/governor"
)

type (
	// Database is a service wrapper around an sql.DB instance
	//
	// DB returns the wrapped sql database instance
	Database interface {
		DB() (*sql.DB, error)
	}

	Service interface {
		governor.Service
		Database
	}

	pgauth struct {
		username string
		password string
	}

	getClientRes struct {
		client *sql.DB
		err    error
	}

	getOp struct {
		res chan<- getClientRes
	}

	pingOp struct {
		res chan<- error
	}

	service struct {
		db       *sql.DB
		auth     pgauth
		connopts string
		config   governor.SecretReader
		logger   governor.Logger
		ops      chan getOp
		pings    chan pingOp
		done     <-chan struct{}
	}
)

// New creates a new db service
func New() Service {
	return &service{
		ops:   make(chan getOp),
		pings: make(chan pingOp),
	}
}

func (s *service) Register(r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	r.SetDefault("auth", "")
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
	s.connopts = fmt.Sprintf("dbname=%s host=%s port=%s sslmode=%s", conf["dbname"], conf["host"], conf["port"], conf["sslmode"])

	done := make(chan struct{})
	go s.execute(ctx, done)
	s.done = done

	if _, err := s.DB(); err != nil {
		return err
	}
	return nil
}

func (s *service) execute(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	for {
		select {
		case <-ctx.Done():
			s.closeClient()
			return
		case op := <-s.pings:
			op.res <- s.handlePing()
			close(op.res)
		case op := <-s.ops:
			client, err := s.handleGetClient()
			op.res <- getClientRes{
				client: client,
				err:    err,
			}
			close(op.res)
		}
	}
}

func (s *service) handlePing() error {
	if s.db == nil {
		return governor.NewError("No db connection", http.StatusInternalServerError, nil)
	}
	if err := s.db.Ping(); err != nil {
		s.config.InvalidateSecret("auth")
		return governor.NewError("Failed to ping db", http.StatusInternalServerError, err)
	}
	return nil
}

func (s *service) handleGetClient() (*sql.DB, error) {
	authsecret, err := s.config.GetSecret("auth")
	if err != nil {
		return nil, err
	}
	auth := pgauth{
		username: authsecret["username"].(string),
		password: authsecret["password"].(string),
	}
	if auth == s.auth {
		return s.db, nil
	}

	s.closeClient()

	connection := fmt.Sprintf("user=%s password=%s %s", auth.username, auth.password, s.connopts)
	db, err := sql.Open("postgres", connection)
	if err != nil {
		return nil, governor.NewError("Failed to init postgres conn", http.StatusInternalServerError, err)
	}
	if err := db.Ping(); err != nil {
		s.config.InvalidateSecret("auth")
		return nil, governor.NewError("Failed to ping db", http.StatusInternalServerError, err)
	}

	s.db = db
	s.auth = auth
	s.logger.Info(fmt.Sprintf("established connection to %s with user %s", s.connopts, s.auth.username), nil)
	return s.db, nil
}

func (s *service) closeClient() {
	if s.db == nil {
		return
	}
	if err := s.db.Close(); err != nil {
		s.logger.Error("failed to close db connection", map[string]string{
			"error":      err.Error(),
			"actiontype": "closedberr",
			"connection": s.connopts,
			"username":   s.auth.username,
		})
	} else {
		s.logger.Info("closed db connection", map[string]string{
			"actiontype": "closedbok",
			"connection": s.connopts,
			"username":   s.auth.username,
		})
	}
	s.db = nil
	s.auth = pgauth{}
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
	res := make(chan error)
	op := pingOp{
		res: res,
	}
	select {
	case <-s.done:
		return governor.NewError("DB service shutdown", http.StatusInternalServerError, nil)
	case s.pings <- op:
		return <-res
	}
}

// DB implements Database.DB by returning its wrapped sql.DB
func (s *service) DB() (*sql.DB, error) {
	res := make(chan getClientRes)
	op := getOp{
		res: res,
	}
	select {
	case <-s.done:
		return nil, governor.NewError("DB service shutdown", http.StatusInternalServerError, nil)
	case s.ops <- op:
		v := <-res
		return v.client, v.err
	}
}
