package model

import (
	"context"
	"errors"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/hunter2"
	"xorkevin.dev/kerrors"
)

//go:generate forge model

const (
	uidSize = 16
	keySize = 32
)

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
		db       db.Database
		hasher   hunter2.Hasher
		verifier *hunter2.Verifier
	}

	// Model is the db OAuth app model
	//forge:model oauthapp
	//forge:model:query oauthapp
	Model struct {
		ClientID     string `model:"clientid,VARCHAR(31) PRIMARY KEY" query:"clientid;getoneeq,clientid;getgroupeq,clientid|in;deleq,clientid"`
		Name         string `model:"name,VARCHAR(255) NOT NULL" query:"name"`
		URL          string `model:"url,VARCHAR(512) NOT NULL" query:"url"`
		RedirectURI  string `model:"redirect_uri,VARCHAR(512) NOT NULL" query:"redirect_uri"`
		Logo         string `model:"logo,VARCHAR(4095)" query:"logo"`
		KeyHash      string `model:"keyhash,VARCHAR(255) NOT NULL" query:"keyhash"`
		Time         int64  `model:"time,BIGINT NOT NULL" query:"time"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL;index;index,creator_id" query:"creation_time;getgroup;getgroupeq,creator_id"`
		CreatorID    string `model:"creator_id,VARCHAR(31) NOT NULL" query:"creator_id;deleq,creator_id"`
	}

	//forge:model:query oauthapp
	oauthKeyHash struct {
		KeyHash string `query:"keyhash;updeq,clientid"`
		Time    int64  `query:"time"`
	}

	//forge:model:query oauthapp
	oauthProps struct {
		Name        string `query:"name;updeq,clientid"`
		URL         string `query:"url"`
		RedirectURI string `query:"redirect_uri"`
		Logo        string `query:"logo"`
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

// NewInCtx creates a new oauth app repo from a context and sets it in the context
func NewInCtx(inj governor.Injector, table string) {
	SetCtxRepo(inj, NewCtx(inj, table))
}

// NewCtx creates a new oauth app repo from a context
func NewCtx(inj governor.Injector, table string) Repo {
	dbService := db.GetCtxDB(inj)
	return New(dbService, table)
}

// New creates a new OAuth app repository
func New(database db.Database, table string) Repo {
	hasher := hunter2.NewBlake2bHasher()
	verifier := hunter2.NewVerifier()
	verifier.RegisterHash(hasher)

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
	mUID, err := uid.New(uidSize)
	if err != nil {
		return nil, "", kerrors.WithMsg(err, "Failed to create new uid")
	}
	clientid := mUID.Base64()

	key, err := uid.New(keySize)
	if err != nil {
		return nil, "", kerrors.WithMsg(err, "Failed to create oauth client secret")
	}
	keystr := key.Base64()
	hash, err := r.hasher.Hash(keystr)
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
	}, keystr, nil
}

func (r *repo) ValidateKey(key string, m *Model) (bool, error) {
	ok, err := r.verifier.Verify(key, m.KeyHash)
	if err != nil {
		return false, kerrors.WithMsg(err, "Failed to verify key")
	}
	return ok, nil
}

func (r *repo) RehashKey(ctx context.Context, m *Model) (string, error) {
	key, err := uid.New(keySize)
	if err != nil {
		return "", kerrors.WithMsg(err, "Failed to create oauth client secret")
	}
	keystr := key.Base64()
	keyhash, err := r.hasher.Hash(keystr)
	if err != nil {
		return "", kerrors.WithMsg(err, "Failed to hash oauth client secret")
	}
	d, err := r.db.DB(ctx)
	if err != nil {
		return "", err
	}
	now := time.Now().Round(0).Unix()
	if err := r.table.UpdoauthKeyHashEqClientID(ctx, d, &oauthKeyHash{
		KeyHash: keyhash,
		Time:    now,
	}, m.ClientID); err != nil {
		return "", kerrors.WithMsg(err, "Failed to update oauth client app")
	}
	m.KeyHash = keyhash
	m.Time = now
	return keystr, nil
}

func (r *repo) GetByID(ctx context.Context, clientid string) (*Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelEqClientID(ctx, d, clientid)
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
		m, err := r.table.GetModelOrdCreationTime(ctx, d, false, limit, offset)
		if err != nil {
			return nil, kerrors.WithMsg(err, "Failed to get oauth apps")
		}
		return m, nil
	}
	m, err := r.table.GetModelEqCreatorIDOrdCreationTime(ctx, d, creatorid, false, limit, offset)
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
	m, err := r.table.GetModelHasClientIDOrdClientID(ctx, d, clientids, true, len(clientids), 0)
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
	if err := r.table.UpdoauthPropsEqClientID(ctx, d, &oauthProps{
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
	if err := r.table.DelEqClientID(ctx, d, m.ClientID); err != nil {
		return kerrors.WithMsg(err, "Failed to delete oauth app")
	}
	return nil
}

func (r *repo) DeleteCreatorApps(ctx context.Context, creatorid string) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelEqCreatorID(ctx, d, creatorid); err != nil {
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
		if !errors.Is(err, db.ErrorAuthz{}) {
			return err
		}
	}
	return nil
}
