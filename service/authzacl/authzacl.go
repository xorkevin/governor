package authzacl

import (
	"context"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/authzacl/aclmodel"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/klog"
)

type (
	Obj struct {
		NS  string `json:"ns"`
		Key string `json:"key"`
	}

	ObjRel struct {
		NS   string `json:"ns"`
		Key  string `json:"key"`
		Pred string `json:"pred"`
	}

	Sub struct {
		NS   string `json:"ns"`
		Key  string `json:"key"`
		Pred string `json:"pred"`
	}

	Relation struct {
		Obj ObjRel `json:"obj"`
		Sub Sub    `json:"sub"`
	}

	ACL interface{}

	ACLManager interface{}

	Service struct {
		repo      aclmodel.Repo
		events    events.Events
		log       *klog.LevelLogger
		streamacl string
	}
)

// New returns a new RolesManager
func New(repo aclmodel.Repo, ev events.Events) *Service {
	return &Service{
		repo:   repo,
		events: ev,
	}
}

func (s *Service) Register(r governor.ConfigRegistrar) {
	s.streamacl = r.Name()
}

func (s *Service) Init(ctx context.Context, r governor.ConfigReader, kit governor.ServiceKit) error {
	s.log = klog.NewLevelLogger(kit.Logger)
	return nil
}

func (s *Service) Start(ctx context.Context) error {
	return nil
}

func (s *Service) Stop(ctx context.Context) {
}

func (s *Service) Setup(ctx context.Context, req governor.ReqSetup) error {
	if err := s.repo.Setup(ctx); err != nil {
		return err
	}
	s.log.Info(ctx, "Created acl table")

	return nil
}

func (s *Service) Health(ctx context.Context) error {
	return nil
}
