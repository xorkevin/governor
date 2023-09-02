package apikeymodel

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
	// Repo is an apikey repository
	Repo interface {
		New(userid string, scope string, name, desc string) (*Model, string, error)
		ValidateKey(key string, m *Model) (bool, error)
		RehashKey(ctx context.Context, m *Model) (string, error)
		GetByID(ctx context.Context, keyid string) (*Model, error)
		GetUserKeys(ctx context.Context, userid string, limit, offset int) ([]Model, error)
		Insert(ctx context.Context, m *Model) error
		UpdateProps(ctx context.Context, m *Model) error
		Delete(ctx context.Context, m *Model) error
		DeleteKeys(ctx context.Context, keyids []string) error
		Setup(ctx context.Context) error
	}

	repo struct {
		table    *apikeyModelTable
		db       dbsql.Database
		hasher   h2hash.Hasher
		verifier *h2hash.Verifier
	}

	// Model is the db Apikey model
	//forge:model apikey
	//forge:model:query apikey
	Model struct {
		Keyid   string `model:"keyid,VARCHAR(63) PRIMARY KEY"`
		Userid  string `model:"userid,VARCHAR(31) NOT NULL"`
		Scope   string `model:"scope,VARCHAR(4095) NOT NULL"`
		KeyHash string `model:"keyhash,VARCHAR(127) NOT NULL"`
		Name    string `model:"name,VARCHAR(255) NOT NULL"`
		Desc    string `model:"description,VARCHAR(255)"`
		Time    int64  `model:"time,BIGINT NOT NULL"`
	}

	//forge:model:query apikey
	apikeyHash struct {
		KeyHash string `model:"keyhash"`
		Time    int64  `model:"time"`
	}

	//forge:model:query apikey
	apikeyProps struct {
		Scope string `model:"scope"`
		Name  string `model:"name"`
		Desc  string `model:"description"`
	}
)

// New creates a new apikey repository
func New(database dbsql.Database, table string) Repo {
	hasher := blake2b.New(blake2b.Config{})
	verifier := h2hash.NewVerifier()
	verifier.Register(hasher)

	return &repo{
		table: &apikeyModelTable{
			TableName: table,
		},
		db:       database,
		hasher:   hasher,
		verifier: verifier,
	}
}

func (r *repo) New(userid string, scope string, name, desc string) (*Model, string, error) {
	aid, err := uid.New()
	if err != nil {
		return nil, "", kerrors.WithMsg(err, "Failed to create new api key id")
	}
	keybytes, err := uid.NewKey()
	if err != nil {
		return nil, "", kerrors.WithMsg(err, "Failed to create new api key")
	}
	key := keybytes.Base64()
	hash, err := r.hasher.Hash([]byte(key))
	if err != nil {
		return nil, "", kerrors.WithMsg(err, "Failed to hash api key")
	}
	now := time.Now().Round(0).Unix()
	return &Model{
		Keyid:   aid.Base64(),
		Userid:  userid,
		Scope:   scope,
		KeyHash: hash,
		Name:    name,
		Desc:    desc,
		Time:    now,
	}, key, nil
}

func (r *repo) ValidateKey(key string, m *Model) (bool, error) {
	ok, err := r.verifier.Verify([]byte(key), m.KeyHash)
	if err != nil {
		return false, kerrors.WithMsg(err, "Failed to verify key")
	}
	return ok, nil
}

func (r *repo) RehashKey(ctx context.Context, m *Model) (string, error) {
	keybytes, err := uid.NewKey()
	if err != nil {
		return "", kerrors.WithMsg(err, "Failed to create new api key")
	}
	key := keybytes.Base64()
	hash, err := r.hasher.Hash([]byte(key))
	if err != nil {
		return "", kerrors.WithMsg(err, "Failed to hash api key")
	}
	d, err := r.db.DB(ctx)
	if err != nil {
		return "", err
	}
	now := time.Now().Round(0).Unix()
	if err := r.table.UpdapikeyHashByID(ctx, d, &apikeyHash{
		KeyHash: hash,
		Time:    now,
	}, m.Keyid); err != nil {
		return "", kerrors.WithMsg(err, "Failed to update apikey")
	}
	m.KeyHash = hash
	m.Time = now
	return key, nil
}

func (r *repo) GetByID(ctx context.Context, keyid string) (*Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelByID(ctx, d, keyid)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get apikey")
	}
	return m, nil
}

func (r *repo) GetUserKeys(ctx context.Context, userid string, limit, offset int) ([]Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelByUserid(ctx, d, userid, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get user apikeys")
	}
	return m, nil
}

func (r *repo) Insert(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.Insert(ctx, d, m); err != nil {
		return kerrors.WithMsg(err, "Failed to insert apikey")
	}
	return nil
}

func (r *repo) UpdateProps(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.UpdapikeyPropsByID(ctx, d, &apikeyProps{
		Scope: m.Scope,
		Name:  m.Name,
		Desc:  m.Desc,
	}, m.Keyid); err != nil {
		return kerrors.WithMsg(err, "Failed to update apikey")
	}
	return nil
}

func (r *repo) Delete(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelByID(ctx, d, m.Keyid); err != nil {
		return kerrors.WithMsg(err, "Failed to delete apikey")
	}
	return nil
}

func (r *repo) DeleteKeys(ctx context.Context, keyids []string) error {
	if len(keyids) == 0 {
		return nil
	}
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelByIDs(ctx, d, keyids); err != nil {
		return kerrors.WithMsg(err, "Failed to delete apikeys")
	}
	return nil
}

func (r *repo) Setup(ctx context.Context) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.Setup(ctx, d); err != nil {
		return kerrors.WithMsg(err, "Failed to setup user apikeys model")
	}
	return nil
}
