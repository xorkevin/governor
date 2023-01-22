package oauthconnmodel

import (
	"context"
	"errors"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/hunter2/h2hash"
	"xorkevin.dev/hunter2/h2hash/blake2b"
	"xorkevin.dev/kerrors"
)

//go:generate forge model

const (
	keySize = 32
)

type (
	// Repo is the OAuth account connection repository
	Repo interface {
		New(userid, clientid, scope, nonce, challenge, method string, authTime int64) (*Model, string, error)
		ValidateCode(code string, m *Model) (bool, error)
		RehashCode(m *Model) (string, error)
		ValidateKey(key string, m *Model) (bool, error)
		RehashKey(m *Model) (string, error)
		GetByID(ctx context.Context, userid, clientid string) (*Model, error)
		GetUserConnections(ctx context.Context, userid string, limit, offset int) ([]Model, error)
		Insert(ctx context.Context, m *Model) error
		Update(ctx context.Context, m *Model) error
		Delete(ctx context.Context, userid string, clientids []string) error
		DeleteUserConnections(ctx context.Context, userid string) error
		Setup(ctx context.Context) error
	}

	repo struct {
		table    *connectionModelTable
		db       db.Database
		hasher   h2hash.Hasher
		verifier *h2hash.Verifier
	}

	// Model is an connected OAuth app to a user account
	//forge:model connection
	//forge:model:query connection
	Model struct {
		Userid          string `model:"userid,VARCHAR(31)" query:"userid;deleq,userid"`
		ClientID        string `model:"clientid,VARCHAR(31), PRIMARY KEY (userid, clientid);index" query:"clientid;getoneeq,userid,clientid;updeq,userid,clientid;deleq,userid,clientid|in"`
		Scope           string `model:"scope,VARCHAR(4095) NOT NULL" query:"scope"`
		Nonce           string `model:"nonce,VARCHAR(255)" query:"nonce"`
		Challenge       string `model:"challenge,VARCHAR(128)" query:"challenge"`
		ChallengeMethod string `model:"challenge_method,VARCHAR(31)" query:"challenge_method"`
		CodeHash        string `model:"codehash,VARCHAR(255) NOT NULL" query:"codehash"`
		AuthTime        int64  `model:"auth_time,BIGINT NOT NULL" query:"auth_time"`
		CodeTime        int64  `model:"code_time,BIGINT NOT NULL" query:"code_time"`
		AccessTime      int64  `model:"access_time,BIGINT NOT NULL;index,userid" query:"access_time;getgroupeq,userid"`
		CreationTime    int64  `model:"creation_time,BIGINT NOT NULL" query:"creation_time"`
		KeyHash         string `model:"keyhash,VARCHAR(255) NOT NULL" query:"keyhash"`
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

// NewInCtx creates a new oauth connection repo from a context and sets it in the context
func NewInCtx(inj governor.Injector, table string) {
	SetCtxRepo(inj, NewCtx(inj, table))
}

// NewCtx creates a new oauth connection repo from a context
func NewCtx(inj governor.Injector, table string) Repo {
	dbService := db.GetCtxDB(inj)
	return New(dbService, table)
}

// New creates a new OAuth connection repository
func New(database db.Database, table string) Repo {
	hasher := blake2b.New(blake2b.Config{})
	verifier := h2hash.NewVerifier()
	verifier.Register(hasher)

	return &repo{
		table: &connectionModelTable{
			TableName: table,
		},
		db:       database,
		hasher:   hasher,
		verifier: verifier,
	}
}

func (r *repo) New(userid, clientid, scope, nonce, challenge, challengeMethod string, authTime int64) (*Model, string, error) {
	codebytes, err := uid.New(keySize)
	if err != nil {
		return nil, "", kerrors.WithMsg(err, "Failed to create oauth authorization code")
	}
	code := codebytes.Base64()
	codehash, err := r.hasher.Hash([]byte(code))
	if err != nil {
		return nil, "", kerrors.WithMsg(err, "Failed to hash oauth authorization code")
	}

	now := time.Now().Round(0).Unix()
	return &Model{
		Userid:          userid,
		ClientID:        clientid,
		Scope:           scope,
		Nonce:           nonce,
		Challenge:       challenge,
		ChallengeMethod: challengeMethod,
		CodeHash:        codehash,
		AuthTime:        authTime,
		CodeTime:        now,
		AccessTime:      now,
		CreationTime:    now,
	}, code, nil
}

func (r *repo) ValidateCode(code string, m *Model) (bool, error) {
	if m.CodeHash == "" {
		return false, nil
	}
	ok, err := r.verifier.Verify([]byte(code), m.CodeHash)
	if err != nil {
		return false, kerrors.WithMsg(err, "Failed to verify code")
	}
	return ok, nil
}

func (r *repo) RehashCode(m *Model) (string, error) {
	codebytes, err := uid.New(keySize)
	if err != nil {
		return "", kerrors.WithMsg(err, "Failed to create oauth authorization code")
	}
	code := codebytes.Base64()
	codehash, err := r.hasher.Hash([]byte(code))
	if err != nil {
		return "", kerrors.WithMsg(err, "Failed to hash oauth authorization code")
	}
	m.CodeHash = codehash
	return code, nil
}

func (r *repo) ValidateKey(key string, m *Model) (bool, error) {
	if m.KeyHash == "" {
		return false, nil
	}
	ok, err := r.verifier.Verify([]byte(key), m.KeyHash)
	if err != nil {
		return false, kerrors.WithMsg(err, "Failed to verify key")
	}
	return ok, nil
}

func (r *repo) RehashKey(m *Model) (string, error) {
	keybytes, err := uid.New(keySize)
	if err != nil {
		return "", kerrors.WithMsg(err, "Failed to create oauth session key")
	}
	key := keybytes.Base64()
	keyhash, err := r.hasher.Hash([]byte(key))
	if err != nil {
		return "", kerrors.WithMsg(err, "Failed to hash oauth session key")
	}
	m.KeyHash = keyhash
	return key, nil
}

func (r *repo) GetByID(ctx context.Context, userid, clientid string) (*Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelEqUseridEqClientID(ctx, d, userid, clientid)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get connected oauth app")
	}
	return m, nil
}

func (r *repo) GetUserConnections(ctx context.Context, userid string, limit, offset int) ([]Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelEqUseridOrdAccessTime(ctx, d, userid, false, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get connected oauth apps")
	}
	return m, nil
}

func (r *repo) Insert(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.Insert(ctx, d, m); err != nil {
		return kerrors.WithMsg(err, "Failed to add connected oauth app")
	}
	return nil
}

func (r *repo) Update(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.UpdModelEqUseridEqClientID(ctx, d, m, m.Userid, m.ClientID); err != nil {
		return kerrors.WithMsg(err, "Failed to update connected oauth app")
	}
	return nil
}

func (r *repo) Delete(ctx context.Context, userid string, clientids []string) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelEqUseridHasClientID(ctx, d, userid, clientids); err != nil {
		return kerrors.WithMsg(err, "Failed to delete connected oauth app")
	}
	return nil
}

func (r *repo) DeleteUserConnections(ctx context.Context, userid string) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelEqUserid(ctx, d, userid); err != nil {
		return kerrors.WithMsg(err, "Failed to delete connected oauth apps")
	}
	return nil
}

func (r *repo) Setup(ctx context.Context) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.Setup(ctx, d); err != nil {
		err = kerrors.WithMsg(err, "Failed to setup oauth connection model")
		if !errors.Is(err, db.ErrorAuthz) {
			return err
		}
	}
	return nil
}
