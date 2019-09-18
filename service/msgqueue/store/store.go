package msgqueuestore

import (
	"context"
	"github.com/labstack/echo/v4"
	"net/http"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
)

const (
	uidSize = 16
)

type (
	Service interface {
		governor.Service
	}

	service struct {
		db     db.Database
		logger governor.Logger
	}
)

// New creates a new msgqueuestore service
func New(database db.Database) Service {
	return &service{
		db: database,
	}
}

func (s *service) Register(r governor.ConfigRegistrar, jr governor.JobRegistrar) {
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, g *echo.Group) error {
	s.logger = l
	return nil
}

func (s *service) Setup(req governor.ReqSetup) error {
	l := s.logger.WithData(map[string]string{
		"phase": "setup",
	})
	if _, err := s.db.DB().Exec(setupSQL); err != nil {
		return governor.NewError("Failed to setup message queue store", http.StatusInternalServerError, err)
	}
	l.Info("created msgqueue store tables", nil)
	return nil
}

func (s *service) Start(ctx context.Context) error {
	return nil
}

func (s *service) Stop(ctx context.Context) {
}

func (s *service) Health() error {
	return nil
}
