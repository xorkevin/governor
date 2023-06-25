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
		NS   string `json:"ns"`
		Key  string `json:"key"`
		Pred string `json:"pred"`
	}

	Relation struct {
		Obj Obj `json:"obj"`
		Sub Obj `json:"sub"`
	}

	ACL interface{}

	ACLManager interface{}

	Service struct {
		repo      aclmodel.Repo
		events    events.Events
		log       *klog.LevelLogger
		streamacl string
	}

	ctxKeyService struct{}
)

// GetCtxACL returns an ACL service from the context
func GetCtxACL(inj governor.Injector) ACL {
	v := inj.Get(ctxKeyService{})
	if v == nil {
		return nil
	}
	return v.(ACL)
}

// GetCtxACLManager returns an ACL service from the context
func GetCtxACLManager(inj governor.Injector) ACLManager {
	v := inj.Get(ctxKeyService{})
	if v == nil {
		return nil
	}
	return v.(ACLManager)
}

// setCtxACL sets an ACLManager service in the context
func setCtxACL(inj governor.Injector, a ACLManager) {
	inj.Set(ctxKeyService{}, a)
}

// NewCtx creates a new ACL service from a context
func NewCtx(inj governor.Injector) *Service {
	return New(
		aclmodel.GetCtxRepo(inj),
		events.GetCtxEvents(inj),
	)
}

// New returns a new RolesManager
func New(repo aclmodel.Repo, ev events.Events) *Service {
	return &Service{
		repo:   repo,
		events: ev,
	}
}

func (s *Service) Register(inj governor.Injector, r governor.ConfigRegistrar) {
	setCtxACL(inj, s)
	s.streamacl = r.Name()
}

func (s *Service) Init(ctx context.Context, r governor.ConfigReader, log klog.Logger, m governor.Router) error {
	s.log = klog.NewLevelLogger(log)
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
