package oauthappmodel

import (
	"context"
	"errors"
	"time"

	"xorkevin.dev/governor/service/dbsql"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/hunter2/h2hash"
	"xorkevin.dev/hunter2/h2hash/blake2b"
	"xorkevin.dev/kerrors"
)

//go:generate forge model

type (
	// Repo is an OAuth app repository
	Repo interface {
		New(name, url, redirectURI, creatorID string) (*Model, string, error)
		ValidateKey(key string, m *Model) (bool, error)
		RehashKey(ctx context.Context, m *Model) (string, error)
		GetByID(ctx context.Context, clientid string) (*Model, error)
		GetApps(ctx context.Context, limit, offset int, creatorid string) ([]Model, error)
		GetBulk(ctx context.Context, clientids []string) ([]Model, error)
		Insert(ctx context.Context, m *Model) error
		UpdateProps(ctx context.Context, m *Model) error
		DeleteCreatorApps(ctx context.Context, creatorid string) error
		Delete(ctx context.Context, m *Model) error
		Setup(ctx context.Context) error
	}

	repo struct {
		table    *oauthappModelTable
		db       dbsql.Database
		hasher   h2hash.Hasher
		verifier *h2hash.Verifier
	}

	// Model is the db OAuth app model
	//forge:model oauthapp
	//forge:model:query oauthapp
	Model struct {
		ClientID     string `model:"clientid,VARCHAR(31) PRIMARY KEY"`
		Name         string `model:"name,VARCHAR(255) NOT NULL"`
		URL          string `model:"url,VARCHAR(512) NOT NULL"`
		RedirectURI  string `model:"redirect_uri,VARCHAR(512) NOT NULL"`
		Logo         string `model:"logo,VARCHAR(4095)"`
		KeyHash      string `model:"keyhash,VARCHAR(255) NOT NULL"`
		Time         int64  `model:"time,BIGINT NOT NULL"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL"`
		CreatorID    string `model:"creator_id,VARCHAR(31) NOT NULL"`
	}

	//forge:model:query oauthapp
	oauthKeyHash struct {
		KeyHash string `model:"keyhash"`
		Time    int64  `model:"time"`
	}

	//forge:model:query oauthapp
	oauthProps struct {
		Name        string `model:"name"`
		URL         string `model:"url"`
		RedirectURI string `model:"redirect_uri"`
		Logo        string `model:"logo"`
	}
)

// New creates a new OAuth app repository
func New(database dbsql.Database, table string) Repo {
	hasher := blake2b.New(blake2b.Config{})
	verifier := h2hash.NewVerifier()
	verifier.Register(hasher)

	return &repo{
		table: &oauthappModelTable{
			TableName: table,
		},
		db:       database,
		hasher:   hasher,
		verifier: verifier,
	}
}

func (r *repo) New(name, url, redirectURI, creatorID string) (*Model, string, error) {
	mUID, err := uid.New()
	if err != nil {
		return nil, "", kerrors.WithMsg(err, "Failed to create new uid")
	}
	clientid := mUID.Base64()

	keybytes, err := uid.NewKey()
	if err != nil {
		return nil, "", kerrors.WithMsg(err, "Failed to create oauth client secret")
	}
	key := keybytes.Base64()
	hash, err := r.hasher.Hash([]byte(key))
	if err != nil {
		return nil, "", kerrors.WithMsg(err, "Failed to hash oauth client secret")
	}

	now := time.Now().Round(0).Unix()
	return &Model{
		ClientID:     clientid,
		Name:         name,
		URL:          url,
		RedirectURI:  redirectURI,
		KeyHash:      hash,
		Time:         now,
		CreationTime: now,
		CreatorID:    creatorID,
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
		return "", kerrors.WithMsg(err, "Failed to create oauth client secret")
	}
	key := keybytes.Base64()
	keyhash, err := r.hasher.Hash([]byte(key))
	if err != nil {
		return "", kerrors.WithMsg(err, "Failed to hash oauth client secret")
	}
	d, err := r.db.DB(ctx)
	if err != nil {
		return "", err
	}
	now := time.Now().Round(0).Unix()
	if err := r.table.UpdoauthKeyHashByID(ctx, d, &oauthKeyHash{
		KeyHash: keyhash,
		Time:    now,
	}, m.ClientID); err != nil {
		return "", kerrors.WithMsg(err, "Failed to update oauth client app")
	}
	m.KeyHash = keyhash
	m.Time = now
	return key, nil
}

func (r *repo) GetByID(ctx context.Context, clientid string) (*Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelByID(ctx, d, clientid)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get oauth app")
	}
	return m, nil
}

func (r *repo) GetApps(ctx context.Context, limit, offset int, creatorid string) ([]Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	if creatorid == "" {
		m, err := r.table.GetModelAll(ctx, d, limit, offset)
		if err != nil {
			return nil, kerrors.WithMsg(err, "Failed to get oauth apps")
		}
		return m, nil
	}
	m, err := r.table.GetModelByCreator(ctx, d, creatorid, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get oauth apps")
	}
	return m, nil
}

func (r *repo) GetBulk(ctx context.Context, clientids []string) ([]Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelByIDs(ctx, d, clientids, len(clientids), 0)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get oauth apps")
	}
	return m, nil
}

func (r *repo) Insert(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.Insert(ctx, d, m); err != nil {
		return kerrors.WithMsg(err, "Failed to insert oauth app")
	}
	return nil
}

func (r *repo) UpdateProps(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.UpdoauthPropsByID(ctx, d, &oauthProps{
		Name:        m.Name,
		URL:         m.URL,
		RedirectURI: m.RedirectURI,
		Logo:        m.Logo,
	}, m.ClientID); err != nil {
		return kerrors.WithMsg(err, "Failed to update oauth app")
	}
	return nil
}

func (r *repo) Delete(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelByID(ctx, d, m.ClientID); err != nil {
		return kerrors.WithMsg(err, "Failed to delete oauth app")
	}
	return nil
}

func (r *repo) DeleteCreatorApps(ctx context.Context, creatorid string) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelByCreator(ctx, d, creatorid); err != nil {
		return kerrors.WithMsg(err, "Failed to delete oauth apps")
	}
	return nil
}

func (r *repo) Setup(ctx context.Context) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.Setup(ctx, d); err != nil {
		err = kerrors.WithMsg(err, "Failed to setup oauth app model")
		if !errors.Is(err, dbsql.ErrAuthz) {
			return err
		}
	}
	return nil
}
