package db

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	_ "github.com/lib/pq" // depends upon postgres
	"xorkevin.dev/governor"
)

type (
	// Database is a service wrapper around an sql.DB instance
	//
	// DB returns the wrapped sql database instance
	Database interface {
		DB() (*sql.DB, error)
	}

	// Service is a DB and governor.Service
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

	ctxKeyDatabase struct{}
)

// GetCtxDB returns a Database from the context
func GetCtxDB(inj governor.Injector) Database {
	v := inj.Get(ctxKeyDatabase{})
	if v == nil {
		return nil
	}
	return v.(Database)
}

// setCtxDB sets a Database in the context
func setCtxDB(inj governor.Injector, d Database) {
	inj.Set(ctxKeyDatabase{}, d)
}

// New creates a new db service
func New() Service {
	return &service{
		ops:      make(chan getOp),
		ready:    false,
		hbfailed: 0,
	}
}

func (s *service) Register(inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	setCtxDB(inj, s)

	r.SetDefault("auth", "")
	r.SetDefault("dbname", "postgres")
	r.SetDefault("host", "localhost")
	r.SetDefault("port", "5432")
	r.SetDefault("sslmode", "disable")
	r.SetDefault("hbinterval", 5)
	r.SetDefault("hbmaxfail", 5)
}

type (
	// ErrConn is returned on a db connection error
	ErrConn struct{}
	// ErrClient is returned for unknown client errors
	ErrClient struct{}
	// ErrNotFound is returned when a row is not found
	ErrNotFound struct{}
	// ErrUnique is returned when a unique constraint is violated
	ErrUnique struct{}
)

func (e ErrConn) Error() string {
	return "DB connection error"
}

func (e ErrClient) Error() string {
	return "DB client error"
}

func (e ErrNotFound) Error() string {
	return "Row not found"
}

func (e ErrUnique) Error() string {
	return "Uniqueness constraint violated"
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, m governor.Router) error {
	s.logger = l
	l = s.logger.WithData(map[string]string{
		"phase": "init",
	})

	s.config = r

	s.connopts = fmt.Sprintf("dbname=%s host=%s port=%s sslmode=%s", r.GetStr("dbname"), r.GetStr("host"), r.GetStr("port"), r.GetStr("sslmode"))
	s.hbinterval = r.GetInt("hbinterval")
	s.hbmaxfail = r.GetInt("hbmaxfail")

	l.Info("loaded config", map[string]string{
		"connopts":   s.connopts,
		"hbinterval": strconv.Itoa(s.hbinterval),
		"hbmaxfail":  strconv.Itoa(s.hbmaxfail),
	})

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
		return nil, governor.ErrWithKind(nil, governor.ErrInvalidConfig{}, "Invalid secret")
	}
	password, ok := authsecret["password"].(string)
	if !ok {
		return nil, governor.ErrWithKind(nil, governor.ErrInvalidConfig{}, "Invalid secret")
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
		return nil, governor.ErrWithKind(err, ErrClient{}, "Failed to init db conn")
	}
	if err := client.Ping(); err != nil {
		s.config.InvalidateSecret("auth")
		return nil, governor.ErrWithKind(err, ErrConn{}, "Failed to ping db")
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
		return governor.ErrWithKind(nil, ErrConn{}, "DB service not ready")
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
		return nil, governor.ErrWithKind(nil, ErrConn{}, "DB service shutdown")
	case s.ops <- op:
		v := <-res
		return v.client, v.err
	}
}
