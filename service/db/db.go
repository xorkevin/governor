package db

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/labstack/echo/v4"
	_ "github.com/lib/pq" // depends upon postgres
	"net/http"
	"time"
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

	service struct {
		client     *sql.DB
		auth       pgauth
		connopts   string
		config     governor.SecretReader
		logger     governor.Logger
		ops        chan getOp
		ready      bool
		hbfailed   int
		hbinterval int
		hbmaxfail  int
		done       <-chan struct{}
	}
)

// New creates a new db service
func New() Service {
	return &service{
		ops:      make(chan getOp),
		ready:    false,
		hbfailed: 0,
	}
}

func (s *service) Register(r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	r.SetDefault("auth", "")
	r.SetDefault("dbname", "postgres")
	r.SetDefault("host", "localhost")
	r.SetDefault("port", "5432")
	r.SetDefault("sslmode", "disable")
	r.SetDefault("hbinterval", 5)
	r.SetDefault("hbmaxfail", 5)
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, g *echo.Group) error {
	s.logger = l
	l = s.logger.WithData(map[string]string{
		"phase": "init",
	})

	s.config = r

	conf := r.GetStrMap("")
	s.connopts = fmt.Sprintf("dbname=%s host=%s port=%s sslmode=%s", conf["dbname"], conf["host"], conf["port"], conf["sslmode"])
	s.hbinterval = r.GetInt("hbinterval")
	s.hbmaxfail = r.GetInt("hbmaxfail")

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
	ticker := time.NewTicker(time.Duration(s.hbinterval) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.closeClient()
			return
		case <-ticker.C:
			s.handlePing()
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

func (s *service) handlePing() {
	if s.client != nil {
		err := s.client.Ping()
		if err == nil {
			s.ready = true
			s.hbfailed = 0
			return
		}
		s.hbfailed++
		if s.hbfailed < s.hbmaxfail {
			s.logger.Warn("failed to ping db", map[string]string{
				"error":      err.Error(),
				"actiontype": "pingdb",
				"connection": s.connopts,
				"username":   s.auth.username,
			})
			return
		}
		s.logger.Error("failed max pings to db", map[string]string{
			"error":      err.Error(),
			"actiontype": "pingdbmax",
			"connection": s.connopts,
			"username":   s.auth.username,
		})
		s.ready = false
		s.hbfailed = 0
		s.config.InvalidateSecret("auth")
	}
	if _, err := s.handleGetClient(); err != nil {
		s.logger.Error("failed to create db client", map[string]string{
			"error":      err.Error(),
			"actiontype": "createdbclient",
		})
	}
}

func (s *service) handleGetClient() (*sql.DB, error) {
	authsecret, err := s.config.GetSecret("auth")
	if err != nil {
		return nil, err
	}
	username, ok := authsecret["username"].(string)
	if !ok {
		return nil, governor.NewError("Invalid secret", http.StatusInternalServerError, nil)
	}
	password, ok := authsecret["password"].(string)
	if !ok {
		return nil, governor.NewError("Invalid secret", http.StatusInternalServerError, nil)
	}
	auth := pgauth{
		username: username,
		password: password,
	}
	if auth == s.auth {
		return s.client, nil
	}

	s.closeClient()

	opts := fmt.Sprintf("user=%s password=%s %s", auth.username, auth.password, s.connopts)
	client, err := sql.Open("postgres", opts)
	if err != nil {
		return nil, governor.NewError("Failed to init db conn", http.StatusInternalServerError, err)
	}
	if err := client.Ping(); err != nil {
		s.config.InvalidateSecret("auth")
		return nil, governor.NewError("Failed to ping db", http.StatusInternalServerError, err)
	}

	s.client = client
	s.auth = auth
	s.ready = true
	s.hbfailed = 0
	s.logger.Info(fmt.Sprintf("established connection to %s with user %s", s.connopts, s.auth.username), nil)
	return s.client, nil
}

func (s *service) closeClient() {
	if s.client == nil {
		return
	}
	if err := s.client.Close(); err != nil {
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
	s.client = nil
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
	if !s.ready {
		return governor.NewError("DB service not ready", http.StatusInternalServerError, nil)
	}
	return nil
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
