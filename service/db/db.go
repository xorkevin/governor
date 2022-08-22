package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/lib/pq"
	"xorkevin.dev/governor"
	"xorkevin.dev/kerrors"
)

type (
	// Database is a service wrapper around an sql.DB instance
	Database interface {
		DB(ctx context.Context) (SQLDB, error)
	}

	// Service is a DB and governor.Service
	Service interface {
		governor.Service
		Database
	}

	getClientRes struct {
		client SQLDB
		err    error
	}

	getOp struct {
		ctx context.Context
		res chan<- getClientRes
	}

	service struct {
		client     *sql.DB
		sqldb      SQLDB
		auth       pgAuth
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

func (s *service) Register(name string, inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
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
	// ErrUndefinedTable is returned when a table does not exist yet
	ErrUndefinedTable struct{}
	// ErrAuthz is returned when not authorized
	ErrAuthz struct{}
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
	return "Unique constraint violated"
}

func (e ErrUndefinedTable) Error() string {
	return "Undefined table"
}

func (e ErrAuthz) Error() string {
	return "Insufficient privilege"
}

func wrapDBErr(err error, fallbackmsg string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return kerrors.WithKind(err, ErrNotFound{}, "Not found")
	}
	perr := &pq.Error{}
	if errors.As(err, &perr) {
		switch perr.Code {
		case "23505": // unique_violation
			return kerrors.WithKind(err, ErrUnique{}, "Unique constraint violated")
		case "42P01": // undefined_table
			return kerrors.WithKind(err, ErrUndefinedTable{}, "Table not defined")
		case "42501": // insufficient_privilege
			return kerrors.WithKind(err, ErrAuthz{}, "Unauthorized")
		}
	}
	return kerrors.WithMsg(err, fallbackmsg)
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

	l.Info("Loaded config", map[string]string{
		"connopts":   s.connopts,
		"hbinterval": strconv.Itoa(s.hbinterval),
		"hbmaxfail":  strconv.Itoa(s.hbmaxfail),
	})

	done := make(chan struct{})
	go s.execute(ctx, done)
	s.done = done

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
			s.handlePing(ctx)
		case op := <-s.ops:
			client, err := s.handleGetClient(ctx)
			select {
			case <-op.ctx.Done():
			case op.res <- getClientRes{
				client: client,
				err:    err,
			}:
				close(op.res)
			}
		}
	}
}

func (s *service) handlePing(ctx context.Context) {
	if s.client != nil {
		err := s.client.Ping()
		if err == nil {
			s.ready = true
			s.hbfailed = 0
			return
		}
		s.hbfailed++
		if s.hbfailed < s.hbmaxfail {
			s.logger.Warn("Failed to ping db", map[string]string{
				"error":      err.Error(),
				"actiontype": "db_ping",
				"connection": s.connopts,
				"username":   s.auth.Username,
			})
			return
		}
		s.logger.Error("Failed max pings to db", map[string]string{
			"error":      err.Error(),
			"actiontype": "db_ping",
			"connection": s.connopts,
			"username":   s.auth.Username,
		})
		s.ready = false
		s.hbfailed = 0
		s.auth = pgAuth{}
		s.config.InvalidateSecret("auth")
	}
	if _, err := s.handleGetClient(ctx); err != nil {
		s.logger.Error("Failed to create db client", map[string]string{
			"error":      err.Error(),
			"actiontype": "db_create_client",
		})
	}
}

type (
	pgAuth struct {
		Username string `mapstructure:"username"`
		Password string `mapstructure:"password"`
	}
)

func (s *service) handleGetClient(ctx context.Context) (SQLDB, error) {
	var auth pgAuth
	if err := s.config.GetSecret(ctx, "auth", 0, &auth); err != nil {
		return nil, kerrors.WithMsg(err, "Invalid secret")
	}
	if auth.Username == "" {
		return nil, kerrors.WithKind(nil, governor.ErrInvalidConfig{}, "Invalid secret")
	}
	if auth == s.auth {
		return s.sqldb, nil
	}

	s.closeClient()

	opts := fmt.Sprintf("user=%s password=%s %s", auth.Username, auth.Password, s.connopts)
	client, err := sql.Open("postgres", opts)
	if err != nil {
		return nil, kerrors.WithKind(err, ErrClient{}, "Failed to init db conn")
	}
	if err := client.PingContext(ctx); err != nil {
		s.config.InvalidateSecret("auth")
		return nil, kerrors.WithKind(err, ErrConn{}, "Failed to ping db")
	}

	s.client = client
	s.sqldb = &sqldb{
		logger: s.logger,
		client: client,
	}
	s.auth = auth
	s.ready = true
	s.hbfailed = 0
	s.logger.Info(fmt.Sprintf("Established connection to %s with user %s", s.connopts, s.auth.Username), nil)
	return s.sqldb, nil
}

func (s *service) closeClient() {
	if s.client == nil {
		return
	}
	if err := s.client.Close(); err != nil {
		s.logger.Error("Failed to close db connection", map[string]string{
			"error":      err.Error(),
			"actiontype": "db_close",
			"connection": s.connopts,
			"username":   s.auth.Username,
		})
	} else {
		s.logger.Info("Closed db connection", map[string]string{
			"actiontype": "db_close_ok",
			"connection": s.connopts,
			"username":   s.auth.Username,
		})
	}
	s.client = nil
	s.sqldb = nil
	s.auth = pgAuth{}
}

func (s *service) Setup(req governor.ReqSetup) error {
	return nil
}

func (s *service) PostSetup(req governor.ReqSetup) error {
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
		l.Warn("Failed to stop", map[string]string{
			"error":      ctx.Err().Error(),
			"actiontype": "db_stop",
		})
	}
}

func (s *service) Health() error {
	if !s.ready {
		return kerrors.WithKind(nil, ErrConn{}, "DB service not ready")
	}
	return nil
}

// DB implements [Database] and returns [SQLDB]
func (s *service) DB(ctx context.Context) (SQLDB, error) {
	res := make(chan getClientRes)
	op := getOp{
		ctx: ctx,
		res: res,
	}
	select {
	case <-s.done:
		return nil, kerrors.WithMsg(nil, "DB service shutdown")
	case <-ctx.Done():
		return nil, kerrors.WithMsg(ctx.Err(), "Context cancelled")
	case s.ops <- op:
		select {
		case <-ctx.Done():
			return nil, kerrors.WithMsg(ctx.Err(), "Context cancelled")
		case v := <-res:
			return v.client, v.err
		}
	}
}

type (
	// SQLExecutor is the interface of the subset of methods shared by [database/sql.DB] and [database/sql.Tx]
	SQLExecutor interface {
		ExecContext(ctx context.Context, query string, args ...interface{}) (SQLResult, error)
		QueryContext(ctx context.Context, query string, args ...interface{}) (SQLRows, error)
		QueryRowContext(ctx context.Context, query string, args ...interface{}) SQLRow
	}

	// SQLResult is [sql.Result]
	SQLResult = sql.Result

	// SQLRows is the interface boundary of [database/sql.Rows]
	SQLRows interface {
		Next() bool
		Scan(dest ...interface{}) error
		Err() error
		Close() error
	}

	// SQLRow is the interface boundary of [database/sql.Row]
	SQLRow interface {
		Scan(dest ...interface{}) error
		Err() error
	}

	// SQLDB is the interface boundary of a [database/sql.DB]
	SQLDB interface {
		SQLExecutor
	}

	sqldb struct {
		logger governor.Logger
		client *sql.DB
	}

	sqlrows struct {
		logger governor.Logger
		rows   *sql.Rows
	}

	sqlrow struct {
		row *sql.Row
	}
)

// ExecContext implements [SQLExecutor]
func (s *sqldb) ExecContext(ctx context.Context, query string, args ...interface{}) (SQLResult, error) {
	r, err := s.client.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, wrapDBErr(err, "Failed executing command")
	}
	return r, nil
}

// QueryContext implements [SQLExecutor]
func (s *sqldb) QueryContext(ctx context.Context, query string, args ...interface{}) (SQLRows, error) {
	rows, err := s.client.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, wrapDBErr(err, "Failed executing query")
	}
	return &sqlrows{
		logger: s.logger,
		rows:   rows,
	}, nil
}

// QueryRowContext implements [SQLExecutor]
func (s *sqldb) QueryRowContext(ctx context.Context, query string, args ...interface{}) SQLRow {
	return &sqlrow{
		row: s.client.QueryRowContext(ctx, query, args...),
	}
}

// Next implements [SQLRows]
func (r *sqlrows) Next() bool {
	return r.rows.Next()
}

// Scan implements [SQLRows]
func (r *sqlrows) Scan(dest ...interface{}) error {
	if err := r.rows.Scan(dest...); err != nil {
		return wrapDBErr(err, "Failed scanning row")
	}
	return nil
}

// Err implements [SQLRows]
func (r *sqlrows) Err() error {
	if err := r.rows.Err(); err != nil {
		return wrapDBErr(err, "Failed iterating rows")
	}
	return nil
}

// Close implements [SQLRows]
func (r *sqlrows) Close() error {
	if err := r.rows.Close(); err != nil {
		err := wrapDBErr(err, "Failed closing rows")
		r.logger.Error("Failed closing rows", map[string]string{
			"error":      err.Error(),
			"actiontype": "db_close_rows",
		})
		return err
	}
	return nil
}

// Scan implements [SQLRow]
func (r *sqlrow) Scan(dest ...interface{}) error {
	if err := r.row.Scan(dest...); err != nil {
		return wrapDBErr(err, "Failed scanning row")
	}
	return nil
}

// Err implements [SQLRow]
func (r *sqlrow) Err() error {
	if err := r.row.Err(); err != nil {
		return wrapDBErr(err, "Failed executing query")
	}
	return nil
}
