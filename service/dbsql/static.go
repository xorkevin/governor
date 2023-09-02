package dbsql

import (
	"context"
	"database/sql"
	"fmt"

	"xorkevin.dev/governor"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	Static struct {
		log    *klog.LevelLogger
		client *sqldbclient
	}
)

func NewStatic() *Static {
	return &Static{}
}

func (s *Static) Register(r governor.ConfigRegistrar) {
	r.SetDefault("staticauth.username", "")
	r.SetDefault("staticauth.password", "")
	r.SetDefault("dbname", "postgres")
	r.SetDefault("host", "localhost")
	r.SetDefault("port", "5432")
	r.SetDefault("sslmode", "disable")
}

func (s *Static) Init(ctx context.Context, r governor.ConfigReader, kit governor.ServiceKit) error {
	s.log = klog.NewLevelLogger(kit.Logger)

	connopts := fmt.Sprintf("%s:%s/%s?sslmode=%s", r.GetStr("host"), r.GetStr("port"), r.GetStr("dbname"), r.GetStr("sslmode"))

	s.log.Info(ctx, "Loaded config",
		klog.AString("connopts", connopts),
	)

	dbClient, err := sql.Open("postgres", fmt.Sprintf("postgresql://%s:%s@%s", r.GetStr("staticauth.username"), r.GetStr("staticauth.password"), connopts))
	if err != nil {
		return kerrors.WithKind(err, ErrClient, "Failed to init db conn")
	}
	s.client = &sqldbclient{
		log:    s.log,
		client: dbClient,
	}

	return nil
}

func (s *Static) Start(ctx context.Context) error {
	return nil
}

func (s *Static) Stop(ctx context.Context) error {
	if s.client != nil {
		if err := s.client.Close(); err != nil {
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to close db client"))
		} else {
			s.log.Info(ctx, "Closed db client")
		}
	}
	return nil
}

func (s *Static) Setup(ctx context.Context, req governor.ReqSetup) error {
	return nil
}

func (s *Static) Health(ctx context.Context) error {
	if s.client == nil {
		return kerrors.WithKind(nil, ErrClient, "DB service not ready")
	}
	if err := s.client.PingContext(ctx); err != nil {
		return kerrors.WithKind(nil, ErrConn, "DB service not ready")
	}
	return nil
}

// DB implements [Database] and returns [SQLDB]
func (s *Static) DB(ctx context.Context) (SQLDB, error) {
	if s.client == nil {
		return nil, kerrors.WithKind(nil, ErrClient, "DB service not ready")
	}
	return s.client, nil
}
