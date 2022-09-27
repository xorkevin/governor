package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/lib/pq"
	"xorkevin.dev/governor"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	// Database is a service wrapper around an sql.DB instance
	Database interface {
		DB(ctx context.Context) (SQLDB, error)
	}

	getClientRes struct {
		client SQLDB
		err    error
	}

	getOp struct {
		ctx context.Context
		res chan<- getClientRes
	}

	Service struct {
		sqldb      *sqldb
		asqldb     *atomic.Pointer[sqldb]
		auth       pgAuth
		connopts   string
		config     governor.SecretReader
		log        *klog.LevelLogger
		ops        chan getOp
		ready      *atomic.Bool
		hbfailed   int
		hbinterval time.Duration
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
func New() *Service {
	return &Service{
		asqldb:   &atomic.Pointer[sqldb]{},
		ops:      make(chan getOp),
		ready:    &atomic.Bool{},
		hbfailed: 0,
	}
}

func (s *Service) Register(name string, inj governor.Injector, r governor.ConfigRegistrar) {
	setCtxDB(inj, s)

	r.SetDefault("auth", "")
	r.SetDefault("dbname", "postgres")
	r.SetDefault("host", "localhost")
	r.SetDefault("port", "5432")
	r.SetDefault("sslmode", "disable")
	r.SetDefault("hbinterval", "5s")
	r.SetDefault("hbmaxfail", 5)
}

type (
	// ErrorConn is returned on a db connection error
	ErrorConn struct{}
	// ErrorClient is returned for unknown client errors
	ErrorClient struct{}
	// ErrorNotFound is returned when a row is not found
	ErrorNotFound struct{}
	// ErrorUnique is returned when a unique constraint is violated
	ErrorUnique struct{}
	// ErrorUndefinedTable is returned when a table does not exist yet
	ErrorUndefinedTable struct{}
	// ErrorAuthz is returned when not authorized
	ErrorAuthz struct{}
)

func (e ErrorConn) Error() string {
	return "DB connection error"
}

func (e ErrorClient) Error() string {
	return "DB client error"
}

func (e ErrorNotFound) Error() string {
	return "Row not found"
}

func (e ErrorUnique) Error() string {
	return "Unique constraint violated"
}

func (e ErrorUndefinedTable) Error() string {
	return "Undefined table"
}

func (e ErrorAuthz) Error() string {
	return "Insufficient privilege"
}

func wrapDBErr(err error, fallbackmsg string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return kerrors.WithKind(err, ErrorNotFound{}, "Not found")
	}
	perr := &pq.Error{}
	if errors.As(err, &perr) {
		switch perr.Code {
		case "23505": // unique_violation
			return kerrors.WithKind(err, ErrorUnique{}, "Unique constraint violated")
		case "42P01": // undefined_table
			return kerrors.WithKind(err, ErrorUndefinedTable{}, "Table not defined")
		case "42501": // insufficient_privilege
			return kerrors.WithKind(err, ErrorAuthz{}, "Unauthorized")
		}
	}
	return kerrors.WithMsg(err, fallbackmsg)
}

func (s *Service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, log klog.Logger, m governor.Router) error {
	s.log = klog.NewLevelLogger(log)
	s.config = r

	s.connopts = fmt.Sprintf("dbname=%s host=%s port=%s sslmode=%s", r.GetStr("dbname"), r.GetStr("host"), r.GetStr("port"), r.GetStr("sslmode"))
	var err error
	s.hbinterval, err = r.GetDuration("hbinterval")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse hbinterval")
	}
	s.hbmaxfail = r.GetInt("hbmaxfail")

	s.log.Info(ctx, "Loaded config", klog.Fields{
		"db.connopts":   s.connopts,
		"db.hbinterval": s.hbinterval.String(),
		"db.hbmaxfail":  s.hbmaxfail,
	})

	ctx = klog.WithFields(ctx, klog.Fields{
		"gov.service.phase": "run",
	})

	done := make(chan struct{})
	go s.execute(ctx, done)
	s.done = done

	return nil
}

func (s *Service) execute(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(s.hbinterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.closeClient(klog.ExtendCtx(context.Background(), ctx, nil))
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

func (s *Service) handlePing(ctx context.Context) {
	var err error
	// Check db auth expiry, and reinit client if about to be expired
	if _, err = s.handleGetClient(ctx); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to create db client"), nil)
	}
	// Regardless of whether we were able to successfully retrieve a db client,
	// if there is a db client then ping the db. This allows vault to be
	// temporarily unavailable without disrupting the DB connections.
	if s.sqldb != nil {
		err = s.sqldb.PingContext(ctx)
		if err == nil {
			s.ready.Store(true)
			s.hbfailed = 0
			return
		}
	}
	// Failed a heartbeat
	s.hbfailed++
	if s.hbfailed < s.hbmaxfail {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to ping db"), klog.Fields{
			"db.connopts": s.connopts,
			"db.username": s.auth.Username,
		})
		return
	}
	s.log.Err(ctx, kerrors.WithMsg(err, "Failed max pings to db"), klog.Fields{
		"db.connopts": s.connopts,
		"db.username": s.auth.Username,
	})
	s.asqldb.Store(nil)
	s.ready.Store(false)
	s.hbfailed = 0
	s.auth = pgAuth{}
	s.config.InvalidateSecret("auth")
}

type (
	pgAuth struct {
		Username string `mapstructure:"username"`
		Password string `mapstructure:"password"`
	}
)

func (s *Service) handleGetClient(ctx context.Context) (SQLDB, error) {
	var auth pgAuth
	if err := s.config.GetSecret(ctx, "auth", 0, &auth); err != nil {
		return nil, kerrors.WithMsg(err, "Invalid secret")
	}
	if auth.Username == "" {
		return nil, kerrors.WithKind(nil, governor.ErrorInvalidConfig{}, "Empty auth")
	}
	if auth == s.auth {
		return s.sqldb, nil
	}

	s.closeClient(klog.ExtendCtx(context.Background(), ctx, nil))

	opts := fmt.Sprintf("user=%s password=%s %s", auth.Username, auth.Password, s.connopts)
	client, err := sql.Open("postgres", opts)
	if err != nil {
		return nil, kerrors.WithKind(err, ErrorClient{}, "Failed to init db conn")
	}
	if err := client.PingContext(ctx); err != nil {
		s.config.InvalidateSecret("auth")
		return nil, kerrors.WithKind(err, ErrorConn{}, "Failed to ping db")
	}

	s.sqldb = &sqldb{
		log:    s.log,
		client: client,
	}
	s.asqldb.Store(s.sqldb)
	s.auth = auth
	s.ready.Store(true)
	s.hbfailed = 0
	s.log.Info(ctx, "Established connection to db", klog.Fields{
		"db.connopts": s.connopts,
		"db.username": s.auth.Username,
	})
	return s.sqldb, nil
}

func (s *Service) closeClient(ctx context.Context) {
	s.asqldb.Store(nil)
	if s.sqldb != nil {
		if err := s.sqldb.Close(); err != nil {
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to close db client"), klog.Fields{
				"db.connopts": s.connopts,
				"db.username": s.auth.Username,
			})
		} else {
			s.log.Info(ctx, "Closed db client", klog.Fields{
				"db.connopts": s.connopts,
				"db.username": s.auth.Username,
			})
		}
	}
	s.sqldb = nil
	s.auth = pgAuth{}
}

func (s *Service) Start(ctx context.Context) error {
	return nil
}

func (s *Service) Stop(ctx context.Context) {
	select {
	case <-s.done:
		return
	case <-ctx.Done():
		s.log.WarnErr(ctx, kerrors.WithMsg(ctx.Err(), "Failed to stop"), nil)
	}
}

func (s *Service) Setup(ctx context.Context, req governor.ReqSetup) error {
	return nil
}

func (s *Service) Health(ctx context.Context) error {
	if !s.ready.Load() {
		return kerrors.WithKind(nil, ErrorConn{}, "DB service not ready")
	}
	return nil
}

// DB implements [Database] and returns [SQLDB]
func (s *Service) DB(ctx context.Context) (SQLDB, error) {
	if client := s.asqldb.Load(); client != nil {
		return client, nil
	}

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
		PingContext(ctx context.Context) error
	}

	sqldb struct {
		log    *klog.LevelLogger
		client *sql.DB
	}

	sqlrows struct {
		log  *klog.LevelLogger
		ctx  context.Context
		rows *sql.Rows
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
		log:  s.log,
		ctx:  klog.ExtendCtx(context.Background(), ctx, nil),
		rows: rows,
	}, nil
}

// QueryRowContext implements [SQLExecutor]
func (s *sqldb) QueryRowContext(ctx context.Context, query string, args ...interface{}) SQLRow {
	return &sqlrow{
		row: s.client.QueryRowContext(ctx, query, args...),
	}
}

// PingContext implements [SQLDB]
func (s *sqldb) PingContext(ctx context.Context) error {
	if err := s.client.PingContext(ctx); err != nil {
		return wrapDBErr(err, "Failed to ping db")
	}
	return nil
}

// Close closes the db client
func (s *sqldb) Close() error {
	if err := s.client.Close(); err != nil {
		return wrapDBErr(err, "Failed to close db client")
	}
	return nil
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
		r.log.Err(r.ctx, kerrors.WithMsg(err, "Failed closing rows"), nil)
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
