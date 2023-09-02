package dbsql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"
	"xorkevin.dev/forge/model/sqldb"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/util/ksync"
	"xorkevin.dev/governor/util/lifecycle"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	// Database is a service wrapper around an sql.DB instance
	Database interface {
		DB(ctx context.Context) (SQLDB, error)
	}

	sqldbClient struct {
		client *sqldbclient
		auth   pgAuth
	}

	Service struct {
		lc        *lifecycle.Lifecycle[sqldbClient]
		connopts  string
		config    governor.SecretReader
		log       *klog.LevelLogger
		hbfailed  int
		hbmaxfail int
		wg        *ksync.WaitGroup
	}

	ctxKeyDatabase struct{}
)

// New creates a new db service
func New() *Service {
	return &Service{
		hbfailed: 0,
		wg:       ksync.NewWaitGroup(),
	}
}

func (s *Service) Register(r governor.ConfigRegistrar) {
	r.SetDefault("auth", "")
	r.SetDefault("dbname", "postgres")
	r.SetDefault("host", "localhost")
	r.SetDefault("port", "5432")
	r.SetDefault("sslmode", "disable")
	r.SetDefault("hbinterval", "5s")
	r.SetDefault("hbmaxfail", 5)
}

var (
	// ErrConn is returned on a db connection error
	ErrConn errConn
	// ErrClient is returned for unknown client errors
	ErrClient errClient
	// ErrNotFound is returned when a row is not found
	ErrNotFound errNotFound
	// ErrUnique is returned when a unique constraint is violated
	ErrUnique errUnique
	// ErrUndefinedTable is returned when a table does not exist yet
	ErrUndefinedTable errUndefinedTable
	// ErrAuthz is returned when not authorized
	ErrAuthz errAuthz
)

type (
	errConn           struct{}
	errClient         struct{}
	errNotFound       struct{}
	errUnique         struct{}
	errUndefinedTable struct{}
	errAuthz          struct{}
)

func (e errConn) Error() string {
	return "DB connection error"
}

func (e errClient) Error() string {
	return "DB client error"
}

func (e errNotFound) Error() string {
	return "Row not found"
}

func (e errUnique) Error() string {
	return "Unique constraint violated"
}

func (e errUndefinedTable) Error() string {
	return "Undefined table"
}

func (e errAuthz) Error() string {
	return "Insufficient privilege"
}

func errWithKind(err error, kind error, msg string) error {
	return kerrors.New(kerrors.OptInner(err), kerrors.OptKind(ErrNotFound), kerrors.OptMsg("Not found"), kerrors.OptSkip(2))
}

func wrapDBErr(err error, fallbackmsg string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return errWithKind(err, ErrNotFound, "Not found")
	}
	var perr *pq.Error
	if errors.As(err, &perr) {
		switch perr.Code {
		case "23505": // unique_violation
			return errWithKind(err, ErrUnique, "Unique constraint violated")
		case "42P01": // undefined_table
			return errWithKind(err, ErrUndefinedTable, "Table not defined")
		case "42501": // insufficient_privilege
			return errWithKind(err, ErrAuthz, "Unauthorized")
		}
	}
	return errWithKind(err, nil, fallbackmsg)
}

func (s *Service) Init(ctx context.Context, r governor.ConfigReader, kit governor.ServiceKit) error {
	s.log = klog.NewLevelLogger(kit.Logger)
	s.config = r

	s.connopts = fmt.Sprintf("dbname=%s host=%s port=%s sslmode=%s", r.GetStr("dbname"), r.GetStr("host"), r.GetStr("port"), r.GetStr("sslmode"))
	hbinterval, err := r.GetDuration("hbinterval")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse hbinterval")
	}
	s.hbmaxfail = r.GetInt("hbmaxfail")

	s.log.Info(ctx, "Loaded config",
		klog.AString("connopts", s.connopts),
		klog.AString("hbinterval", hbinterval.String()),
		klog.AInt("hbmaxfail", s.hbmaxfail),
	)

	ctx = klog.CtxWithAttrs(ctx, klog.AString("gov.phase", "run"))

	s.lc = lifecycle.New(
		ctx,
		s.handleGetClient,
		s.closeClient,
		s.handlePing,
		hbinterval,
	)
	go s.lc.Heartbeat(ctx, s.wg)

	return nil
}

func (s *Service) handlePing(ctx context.Context, m *lifecycle.Manager[sqldbClient]) {
	// Check db auth expiry, and reinit client if about to be expired
	client, err := m.Construct(ctx)
	if err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to create db client"))
	}
	// Regardless of whether we were able to successfully retrieve a db client,
	// if there is a db client then ping the db. This allows vault to be
	// temporarily unavailable without disrupting the DB connections.
	var username string
	if client != nil {
		err = client.client.PingContext(ctx)
		if err == nil {
			s.hbfailed = 0
			return
		}
		username = client.auth.Username
	}
	// Failed a heartbeat
	s.hbfailed++
	if s.hbfailed < s.hbmaxfail {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to ping db"),
			klog.AString("connopts", s.connopts),
			klog.AString("username", username),
		)
		return
	}
	s.log.Err(ctx, kerrors.WithMsg(err, "Failed max pings to db"),
		klog.AString("connopts", s.connopts),
		klog.AString("username", username),
	)

	s.hbfailed = 0
	// first invalidate cached secret in order to ensure that construct client
	// will use refreshed auth
	s.config.InvalidateSecret("auth")
	// must stop the client in order to invalidate cached client, and force wait
	// on newly constructed client
	m.Stop(ctx)
}

type (
	pgAuth struct {
		Username string `mapstructure:"username"`
		Password string `mapstructure:"password"`
	}
)

func (s *Service) handleGetClient(ctx context.Context, m *lifecycle.State[sqldbClient]) (*sqldbClient, error) {
	var auth pgAuth
	{
		client := m.Load(ctx)
		if err := s.config.GetSecret(ctx, "auth", 0, &auth); err != nil {
			return client, kerrors.WithMsg(err, "Invalid secret")
		}
		if auth.Username == "" {
			return client, kerrors.WithKind(nil, governor.ErrInvalidConfig, "Empty auth")
		}
		if client != nil && auth == client.auth {
			return client, nil
		}
	}

	dbClient, err := sql.Open("postgres", fmt.Sprintf("user=%s password=%s %s", auth.Username, auth.Password, s.connopts))
	if err != nil {
		return nil, kerrors.WithKind(err, ErrClient, "Failed to init db conn")
	}
	if err := dbClient.PingContext(ctx); err != nil {
		if err := dbClient.Close(); err != nil {
			s.log.Err(ctx, kerrors.WithKind(err, ErrConn, "Failed to close db after failed initial ping"),
				klog.AString("connopts", s.connopts),
				klog.AString("username", auth.Username),
			)
		}
		s.config.InvalidateSecret("auth")
		return nil, kerrors.WithKind(err, ErrConn, "Failed to ping db")
	}

	m.Stop(ctx)

	s.log.Info(ctx, "Established connection to db",
		klog.AString("connopts", s.connopts),
		klog.AString("username", auth.Username),
	)

	client := &sqldbClient{
		client: &sqldbclient{
			log:    s.log,
			client: dbClient,
		},
		auth: auth,
	}
	m.Store(client)

	return client, nil
}

func (s *Service) closeClient(ctx context.Context, client *sqldbClient) {
	if client != nil {
		if err := client.client.Close(); err != nil {
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to close db client"),
				klog.AString("connopts", s.connopts),
				klog.AString("username", client.auth.Username),
			)
		} else {
			s.log.Info(ctx, "Closed db client",
				klog.AString("connopts", s.connopts),
				klog.AString("username", client.auth.Username),
			)
		}
	}
}

func (s *Service) Start(ctx context.Context) error {
	return nil
}

func (s *Service) Stop(ctx context.Context) {
	if err := s.wg.Wait(ctx); err != nil {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to stop"))
	}
}

func (s *Service) Setup(ctx context.Context, req governor.ReqSetup) error {
	return nil
}

func (s *Service) Health(ctx context.Context) error {
	if s.lc.Load(ctx) == nil {
		return kerrors.WithKind(nil, ErrConn, "DB service not ready")
	}
	return nil
}

// DB implements [Database] and returns [SQLDB]
func (s *Service) DB(ctx context.Context) (SQLDB, error) {
	if client := s.lc.Load(ctx); client != nil {
		return client.client, nil
	}

	client, err := s.lc.Construct(ctx)
	if err != nil {
		// explicitly return nil in order to prevent usage of any cached client
		return nil, err
	}
	return client.client, nil
}

type (
	// SQLDB is the interface boundary of a [database/sql.DB]
	SQLDB interface {
		sqldb.Executor
		PingContext(ctx context.Context) error
	}

	sqldbclient struct {
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

// ExecContext implements [sqldb.Executor]
func (s *sqldbclient) ExecContext(ctx context.Context, query string, args ...interface{}) (sqldb.Result, error) {
	r, err := s.client.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, wrapDBErr(err, "Failed executing command")
	}
	return r, nil
}

// QueryContext implements [sqldb.Executor]
func (s *sqldbclient) QueryContext(ctx context.Context, query string, args ...interface{}) (sqldb.Rows, error) {
	rows, err := s.client.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, wrapDBErr(err, "Failed executing query")
	}
	return &sqlrows{
		log:  s.log,
		ctx:  klog.ExtendCtx(context.Background(), ctx),
		rows: rows,
	}, nil
}

// QueryRowContext implements [sqldb.Executor]
func (s *sqldbclient) QueryRowContext(ctx context.Context, query string, args ...interface{}) sqldb.Row {
	return &sqlrow{
		row: s.client.QueryRowContext(ctx, query, args...),
	}
}

// PingContext implements [SQLDB]
func (s *sqldbclient) PingContext(ctx context.Context) error {
	if err := s.client.PingContext(ctx); err != nil {
		return wrapDBErr(err, "Failed to ping db")
	}
	return nil
}

// Close closes the db client
func (s *sqldbclient) Close() error {
	if err := s.client.Close(); err != nil {
		return wrapDBErr(err, "Failed to close db client")
	}
	return nil
}

// Next implements [sqldb.Rows]
func (r *sqlrows) Next() bool {
	return r.rows.Next()
}

// Scan implements [sqldb.Rows]
func (r *sqlrows) Scan(dest ...interface{}) error {
	if err := r.rows.Scan(dest...); err != nil {
		return wrapDBErr(err, "Failed scanning row")
	}
	return nil
}

// Err implements [sqldb.Rows]
func (r *sqlrows) Err() error {
	if err := r.rows.Err(); err != nil {
		return wrapDBErr(err, "Failed iterating rows")
	}
	return nil
}

// Close implements [sqldb.Rows]
func (r *sqlrows) Close() error {
	if err := r.rows.Close(); err != nil {
		err := wrapDBErr(err, "Failed closing rows")
		r.log.Err(r.ctx, kerrors.WithMsg(err, "Failed closing rows"))
		return err
	}
	return nil
}

// Scan implements [sqldb.Row]
func (r *sqlrow) Scan(dest ...interface{}) error {
	if err := r.row.Scan(dest...); err != nil {
		return wrapDBErr(err, "Failed scanning row")
	}
	return nil
}

// Err implements [sqldb.Row]
func (r *sqlrow) Err() error {
	if err := r.row.Err(); err != nil {
		return wrapDBErr(err, "Failed executing query")
	}
	return nil
}
