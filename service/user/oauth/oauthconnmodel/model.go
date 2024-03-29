package oauthconnmodel

import (
	"context"
	"time"

	"xorkevin.dev/governor/service/dbsql"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/hunter2/h2hash"
	"xorkevin.dev/hunter2/h2hash/blake2b"
	"xorkevin.dev/kerrors"
)

//go:generate forge model

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
		table    *connModelTable
		db       dbsql.Database
		hasher   h2hash.Hasher
		verifier *h2hash.Verifier
	}

	// Model is an connected OAuth app to a user account
	//forge:model conn
	//forge:model:query conn
	Model struct {
		Userid          string `model:"userid,VARCHAR(31)"`
		ClientID        string `model:"clientid,VARCHAR(31)"`
		Scope           string `model:"scope,VARCHAR(4095) NOT NULL"`
		Nonce           string `model:"nonce,VARCHAR(255)"`
		Challenge       string `model:"challenge,VARCHAR(128)"`
		ChallengeMethod string `model:"challenge_method,VARCHAR(31)"`
		CodeHash        string `model:"codehash,VARCHAR(255) NOT NULL"`
		AuthTime        int64  `model:"auth_time,BIGINT NOT NULL"`
		CodeTime        int64  `model:"code_time,BIGINT NOT NULL"`
		AccessTime      int64  `model:"access_time,BIGINT NOT NULL"`
		CreationTime    int64  `model:"creation_time,BIGINT NOT NULL"`
		KeyHash         string `model:"keyhash,VARCHAR(255) NOT NULL"`
	}
)

// New creates a new OAuth connection repository
func New(database dbsql.Database, table string) Repo {
	hasher := blake2b.New(blake2b.Config{})
	verifier := h2hash.NewVerifier()
	verifier.Register(hasher)

	return &repo{
		table: &connModelTable{
			TableName: table,
		},
		db:       database,
		hasher:   hasher,
		verifier: verifier,
	}
}

func (r *repo) New(userid, clientid, scope, nonce, challenge, challengeMethod string, authTime int64) (*Model, string, error) {
	codebytes, err := uid.NewKey()
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
	codebytes, err := uid.NewKey()
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
	keybytes, err := uid.NewKey()
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
	m, err := r.table.GetModelByUserClient(ctx, d, userid, clientid)
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
	m, err := r.table.GetModelByUserid(ctx, d, userid, limit, offset)
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
	if err := r.table.UpdModelByUserClient(ctx, d, m, m.Userid, m.ClientID); err != nil {
		return kerrors.WithMsg(err, "Failed to update connected oauth app")
	}
	return nil
}

func (r *repo) Delete(ctx context.Context, userid string, clientids []string) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelByUserClients(ctx, d, userid, clientids); err != nil {
		return kerrors.WithMsg(err, "Failed to delete connected oauth app")
	}
	return nil
}

func (r *repo) DeleteUserConnections(ctx context.Context, userid string) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelByUserid(ctx, d, userid); err != nil {
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
		return kerrors.WithMsg(err, "Failed to setup oauth connection model")
	}
	return nil
}
