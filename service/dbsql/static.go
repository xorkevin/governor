package dbsql

import (
	"context"
	"database/sql"

	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	Static struct {
		client *sqldbclient
	}
)

func NewStatic(dsn string, log klog.Logger) (*Static, error) {
	dbClient, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, kerrors.WithKind(err, ErrClient, "Failed to init db conn")
	}
	return &Static{
		client: &sqldbclient{
			log:    klog.NewLevelLogger(log),
			client: dbClient,
		},
	}, nil
}

func (s *Static) Close() error {
	if s.client != nil {
		if err := s.client.Close(); err != nil {
			return kerrors.WithMsg(err, "Failed to close db client")
		}
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
