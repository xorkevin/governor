package model

import (
	"context"
	"errors"
	"strings"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/hunter2"
	"xorkevin.dev/kerrors"
)

//go:generate forge model -m Model -p session -o model_gen.go Model qID

const (
	uidSize = 8
	keySize = 32
)

const (
	keySeparator = "."
)

type (
	// Repo is a user session repository
	Repo interface {
		New(userid, ipaddr, useragent string) (*Model, string, error)
		ValidateKey(key string, m *Model) (bool, error)
		RehashKey(m *Model) (string, error)
		GetByID(ctx context.Context, sessionid string) (*Model, error)
		GetUserSessions(ctx context.Context, userid string, limit, offset int) ([]Model, error)
		GetUserSessionIDs(ctx context.Context, userid string, limit, offset int) ([]string, error)
		Insert(ctx context.Context, m *Model) error
		Update(ctx context.Context, m *Model) error
		Delete(ctx context.Context, m *Model) error
		DeleteSessions(ctx context.Context, sessionids []string) error
		DeleteUserSessions(ctx context.Context, userid string) error
		Setup(ctx context.Context) error
	}

	repo struct {
		table    *sessionModelTable
		db       db.Database
		hasher   hunter2.Hasher
		verifier *hunter2.Verifier
	}

	// Model is the db User session model
	Model struct {
		SessionID string `model:"sessionid,VARCHAR(63) PRIMARY KEY;index,userid" query:"sessionid;getoneeq,sessionid;updeq,sessionid;deleq,sessionid;deleq,sessionid|arr"`
		Userid    string `model:"userid,VARCHAR(31) NOT NULL" query:"userid;deleq,userid"`
		KeyHash   string `model:"keyhash,VARCHAR(127) NOT NULL" query:"keyhash"`
		Time      int64  `model:"time,BIGINT NOT NULL;index,userid" query:"time;getgroupeq,userid"`
		AuthTime  int64  `model:"auth_time,BIGINT NOT NULL" query:"auth_time"`
		IPAddr    string `model:"ipaddr,VARCHAR(63)" query:"ipaddr"`
		UserAgent string `model:"user_agent,VARCHAR(1023)" query:"user_agent"`
	}

	qID struct {
		SessionID string `query:"sessionid;getgroupeq,userid"`
	}

	ctxKeyRepo struct{}
)

// GetCtxRepo returns a Repo from the context
func GetCtxRepo(inj governor.Injector) Repo {
	v := inj.Get(ctxKeyRepo{})
	if v == nil {
		return nil
	}
	return v.(Repo)
}

// SetCtxRepo sets a Repo in the context
func SetCtxRepo(inj governor.Injector, r Repo) {
	inj.Set(ctxKeyRepo{}, r)
}

// NewInCtx creates a new session repo from a context and sets it in the context
func NewInCtx(inj governor.Injector, table string) {
	SetCtxRepo(inj, NewCtx(inj, table))
}

// NewCtx creates a new session repo from a context
func NewCtx(inj governor.Injector, table string) Repo {
	dbService := db.GetCtxDB(inj)
	return New(dbService, table)
}

// New creates a new user session repository
func New(database db.Database, table string) Repo {
	hasher := hunter2.NewBlake2bHasher()
	verifier := hunter2.NewVerifier()
	verifier.RegisterHash(hasher)

	return &repo{
		table: &sessionModelTable{
			TableName: table,
		},
		db:       database,
		hasher:   hasher,
		verifier: verifier,
	}
}

// New creates a new User session Model
func (r *repo) New(userid, ipaddr, useragent string) (*Model, string, error) {
	sid, err := uid.New(uidSize)
	if err != nil {
		return nil, "", kerrors.WithMsg(err, "Failed to create new session id")
	}
	key, err := uid.New(keySize)
	if err != nil {
		return nil, "", kerrors.WithMsg(err, "Failed to create new session key")
	}
	keystr := key.Base64()
	hash, err := r.hasher.Hash(keystr)
	if err != nil {
		return nil, "", kerrors.WithMsg(err, "Failed to hash session key")
	}
	now := time.Now().Round(0).Unix()
	return &Model{
		SessionID: userid + keySeparator + sid.Base64(),
		Userid:    userid,
		KeyHash:   hash,
		Time:      now,
		AuthTime:  now,
		IPAddr:    ipaddr,
		UserAgent: useragent,
	}, keystr, nil
}

// ParseIDUserid gets the userid from a keyid
func ParseIDUserid(sessionID string) (string, error) {
	userid, _, ok := strings.Cut(sessionID, keySeparator)
	if !ok {
		return "", kerrors.WithMsg(nil, "Invalid session id")
	}
	return userid, nil
}

// ValidateKey validates the key against a hash
func (r *repo) ValidateKey(key string, m *Model) (bool, error) {
	ok, err := r.verifier.Verify(key, m.KeyHash)
	if err != nil {
		return false, kerrors.WithMsg(err, "Failed to verify key")
	}
	return ok, nil
}

// RehashKey generates a new key and saves its hash
func (r *repo) RehashKey(m *Model) (string, error) {
	key, err := uid.New(keySize)
	if err != nil {
		return "", kerrors.WithMsg(err, "Failed to create new session key")
	}
	keystr := key.Base64()
	hash, err := r.hasher.Hash(keystr)
	if err != nil {
		return "", kerrors.WithMsg(err, "Failed to hash session key")
	}
	now := time.Now().Round(0).Unix()
	m.KeyHash = hash
	m.Time = now
	return keystr, nil
}

// GetByID returns a user session model with the given id
func (r *repo) GetByID(ctx context.Context, sessionID string) (*Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelEqSessionID(ctx, d, sessionID)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get session")
	}
	return m, nil
}

// GetUserSessions returns all the sessions of a user
func (r *repo) GetUserSessions(ctx context.Context, userid string, limit, offset int) ([]Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelEqUseridOrdTime(ctx, d, userid, false, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get user sessions")
	}
	return m, nil
}

// GetUserSessionIDs returns all the session ids of a user
func (r *repo) GetUserSessionIDs(ctx context.Context, userid string, limit, offset int) ([]string, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetqIDEqUseridOrdSessionID(ctx, d, userid, true, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get user session ids")
	}
	res := make([]string, 0, len(m))
	for _, i := range m {
		res = append(res, i.SessionID)
	}
	return res, nil
}

// Insert inserts the model into the db
func (r *repo) Insert(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.Insert(ctx, d, m); err != nil {
		return kerrors.WithMsg(err, "Failed to insert session")
	}
	return nil
}

// Update updates the model in the db
func (r *repo) Update(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.UpdModelEqSessionID(ctx, d, m, m.SessionID); err != nil {
		return kerrors.WithMsg(err, "Failed to update session")
	}
	return nil
}

// Delete deletes the model in the db
func (r *repo) Delete(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelEqSessionID(ctx, d, m.SessionID); err != nil {
		return kerrors.WithMsg(err, "Failed to delete session")
	}
	return nil
}

// DeleteSessions deletes the sessions in the set of session ids
func (r *repo) DeleteSessions(ctx context.Context, sessionids []string) error {
	if len(sessionids) == 0 {
		return nil
	}
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelHasSessionID(ctx, d, sessionids); err != nil {
		return kerrors.WithMsg(err, "Failed to delete sessions")
	}
	return nil
}

// DeleteUserSessions deletes all the sessions of a user
func (r *repo) DeleteUserSessions(ctx context.Context, userid string) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelEqUserid(ctx, d, userid); err != nil {
		return kerrors.WithMsg(err, "Failed to delete sessions")
	}
	return nil
}

// Setup creates a new User session table
func (r *repo) Setup(ctx context.Context) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.Setup(ctx, d); err != nil {
		err = kerrors.WithMsg(err, "Failed to setup user session model")
		if !errors.Is(err, db.ErrAuthz{}) {
			return err
		}
	}
	return nil
}
