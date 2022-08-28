package role

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/user/role/model"
	"xorkevin.dev/governor/util/bytefmt"
	"xorkevin.dev/governor/util/rank"
	"xorkevin.dev/kerrors"
)

const (
	time24h int64 = int64(24 * time.Hour / time.Second)
)

type (
	// Roles manages user roles
	Roles interface {
		IntersectRoles(ctx context.Context, userid string, roles rank.Rank) (rank.Rank, error)
		InsertRoles(ctx context.Context, userid string, roles rank.Rank) error
		DeleteRoles(ctx context.Context, userid string, roles rank.Rank) error
		DeleteByRole(ctx context.Context, roleName string, userids []string) error
		GetRoles(ctx context.Context, userid string, prefix string, amount, offset int) (rank.Rank, error)
		GetByRole(ctx context.Context, roleName string, amount, offset int) ([]string, error)
	}

	// Service is a Roles and governor.Service
	Service interface {
		governor.Service
		Roles
	}

	RolesProps struct {
		Userid string
		Roles  []string
	}

	service struct {
		roles         model.Repo
		kvroleset     kvstore.KVStore
		events        events.Events
		logger        governor.Logger
		streamns      string
		opts          Opts
		streamsize    int64
		eventsize     int32
		roleCacheTime int64
	}

	ctxKeyRoles struct{}

	Opts struct {
		StreamName    string
		CreateChannel string
		DeleteChannel string
	}

	ctxKeyOpts struct{}
)

// GetCtxRoles returns a Roles service from the context
func GetCtxRoles(inj governor.Injector) Roles {
	v := inj.Get(ctxKeyRoles{})
	if v == nil {
		return nil
	}
	return v.(Roles)
}

// setCtxRoles sets a Roles service in the context
func setCtxRoles(inj governor.Injector, r Roles) {
	inj.Set(ctxKeyRoles{}, r)
}

func GetCtxOpts(inj governor.Injector) Opts {
	v := inj.Get(ctxKeyOpts{})
	if v == nil {
		return Opts{}
	}
	return v.(Opts)
}

func SetCtxOpts(inj governor.Injector, o Opts) {
	inj.Set(ctxKeyOpts{}, o)
}

// NewCtx creates a new Roles service from a context
func NewCtx(inj governor.Injector) Service {
	roles := model.GetCtxRepo(inj)
	kv := kvstore.GetCtxKVStore(inj)
	ev := events.GetCtxEvents(inj)
	return New(roles, kv, ev)
}

// New returns a new Roles
func New(roles model.Repo, kv kvstore.KVStore, ev events.Events) Service {
	return &service{
		roles:         roles,
		kvroleset:     kv.Subtree("roleset"),
		events:        ev,
		roleCacheTime: time24h,
	}
}

func (s *service) Register(name string, inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	setCtxRoles(inj, s)
	streamname := strings.ToUpper(name)
	s.streamns = streamname
	s.opts = Opts{
		StreamName:    streamname,
		CreateChannel: streamname + ".create",
		DeleteChannel: streamname + ".delete",
	}
	SetCtxOpts(inj, s.opts)

	r.SetDefault("streamsize", "200M")
	r.SetDefault("eventsize", "2K")
	r.SetDefault("rolecache", "24h")
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, m governor.Router) error {
	s.logger = l
	l = s.logger.WithData(map[string]string{
		"phase": "init",
	})

	var err error
	s.streamsize, err = bytefmt.ToBytes(r.GetStr("streamsize"))
	if err != nil {
		return kerrors.WithMsg(err, "Invalid stream size")
	}
	eventsize, err := bytefmt.ToBytes(r.GetStr("eventsize"))
	if err != nil {
		return kerrors.WithMsg(err, "Invalid msg size")
	}
	s.eventsize = int32(eventsize)
	if t, err := time.ParseDuration(r.GetStr("rolecache")); err != nil {
		return kerrors.WithMsg(err, "Failed to parse role cache time")
	} else {
		s.roleCacheTime = int64(t / time.Second)
	}

	l.Info("Loaded config", map[string]string{
		"stream size (bytes)": r.GetStr("streamsize"),
		"event size (bytes)":  r.GetStr("eventsize"),
		"rolecache (s)":       strconv.FormatInt(s.roleCacheTime, 10),
	})

	return nil
}

func (s *service) Setup(req governor.ReqSetup) error {
	l := s.logger.WithData(map[string]string{
		"phase": "setup",
	})

	if err := s.events.InitStream(context.Background(), s.opts.StreamName, []string{s.opts.StreamName + ".>"}, events.StreamOpts{
		Replicas:   1,
		MaxAge:     30 * 24 * time.Hour,
		MaxBytes:   s.streamsize,
		MaxMsgSize: s.eventsize,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to init roles stream")
	}
	l.Info("Created roles stream", nil)

	if err := s.roles.Setup(context.Background()); err != nil {
		return err
	}
	l.Info("Created userrole table", nil)

	return nil
}

func (s *service) PostSetup(req governor.ReqSetup) error {
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

// DecodeRolesProps unmarshals json encoded roles props into a struct
func DecodeRolesProps(msgdata []byte) (*RolesProps, error) {
	m := &RolesProps{}
	if err := json.Unmarshal(msgdata, m); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to decode roles props")
	}
	return m, nil
}
