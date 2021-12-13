package model

import (
	"errors"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/hunter2"
)

//go:generate forge model -m Model -p oauthapp -o model_gen.go Model

const (
	uidSize = 16
	keySize = 32
)

type (
	// Repo is an OAuth app repository
	Repo interface {
		New(name, url, redirectURI, creatorID string) (*Model, string, error)
		ValidateKey(key string, m *Model) (bool, error)
		RehashKey(m *Model) (string, error)
		GetByID(clientid string) (*Model, error)
		GetApps(limit, offset int, creatorid string) ([]Model, error)
		GetBulk(clientids []string) ([]Model, error)
		Insert(m *Model) error
		Update(m *Model) error
		DeleteCreatorApps(creatorid string) error
		Delete(m *Model) error
		Setup() error
	}

	repo struct {
		table    string
		db       db.Database
		hasher   hunter2.Hasher
		verifier *hunter2.Verifier
	}

	// Model is the db OAuth app model
	Model struct {
		ClientID     string `model:"clientid,VARCHAR(31) PRIMARY KEY" query:"clientid;getoneeq,clientid;getgroupeq,clientid|arr;updeq,clientid;deleq,clientid"`
		Name         string `model:"name,VARCHAR(255) NOT NULL" query:"name"`
		URL          string `model:"url,VARCHAR(512) NOT NULL" query:"url"`
		RedirectURI  string `model:"redirect_uri,VARCHAR(512) NOT NULL" query:"redirect_uri"`
		Logo         string `model:"logo,VARCHAR(4095)" query:"logo"`
		KeyHash      string `model:"keyhash,VARCHAR(255) NOT NULL" query:"keyhash"`
		Time         int64  `model:"time,BIGINT NOT NULL" query:"time"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL;index;index,creator_id" query:"creation_time;getgroup;getgroupeq,creator_id"`
		CreatorID    string `model:"creator_id,VARCHAR(31) NOT NULL" query:"creator_id;deleq,creator_id"`
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
		table:    table,
		db:       database,
		hasher:   hasher,
		verifier: verifier,
	}
}

func (r *repo) New(name, url, redirectURI, creatorID string) (*Model, string, error) {
	mUID, err := uid.New(uidSize)
	if err != nil {
		return nil, "", governor.ErrWithMsg(err, "Failed to create new uid")
	}
	clientid := mUID.Base64()

	key, err := uid.New(keySize)
	if err != nil {
		return nil, "", governor.ErrWithMsg(err, "Failed to create oauth client secret")
	}
	keystr := key.Base64()
	hash, err := r.hasher.Hash(keystr)
	if err != nil {
		return nil, "", governor.ErrWithMsg(err, "Failed to hash oauth client secret")
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
		return false, governor.ErrWithMsg(err, "Failed to verify key")
	}
	return ok, nil
}

func (r *repo) RehashKey(m *Model) (string, error) {
	key, err := uid.New(keySize)
	if err != nil {
		return "", governor.ErrWithMsg(err, "Failed to create oauth client secret")
	}
	keystr := key.Base64()
	keyhash, err := r.hasher.Hash(keystr)
	if err != nil {
		return "", governor.ErrWithMsg(err, "Failed to hash oauth client secret")
	}
	now := time.Now().Round(0).Unix()
	m.KeyHash = keyhash
	m.Time = now
	return keystr, nil
}

func (r *repo) GetByID(clientid string) (*Model, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := oauthappModelGetModelEqClientID(d, r.table, clientid)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get OAuth app")
	}
	return m, nil
}

func (r *repo) GetApps(limit, offset int, creatorid string) ([]Model, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	if creatorid == "" {
		m, err := oauthappModelGetModelOrdCreationTime(d, r.table, false, limit, offset)
		if err != nil {
			return nil, db.WrapErr(err, "Failed to get OAuth apps")
		}
		return m, nil
	}
	m, err := oauthappModelGetModelEqCreatorIDOrdCreationTime(d, r.table, creatorid, false, limit, offset)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get OAuth apps")
	}
	return m, nil
}

func (r *repo) GetBulk(clientids []string) ([]Model, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := oauthappModelGetModelHasClientIDOrdClientID(d, r.table, clientids, true, len(clientids), 0)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get OAuth apps")
	}
	return m, nil
}

func (r *repo) Insert(m *Model) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := oauthappModelInsert(d, r.table, m); err != nil {
		return db.WrapErr(err, "Failed to insert OAuth app config")
	}
	return nil
}

func (r *repo) Update(m *Model) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := oauthappModelUpdModelEqClientID(d, r.table, m, m.ClientID); err != nil {
		return db.WrapErr(err, "Failed to update OAuth app config")
	}
	return nil
}

func (r *repo) Delete(m *Model) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := oauthappModelDelEqClientID(d, r.table, m.ClientID); err != nil {
		return db.WrapErr(err, "Failed to delete OAuth app")
	}
	return nil
}

func (r *repo) DeleteCreatorApps(creatorid string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := oauthappModelDelEqCreatorID(d, r.table, creatorid); err != nil {
		return db.WrapErr(err, "Failed to delete OAuth apps")
	}
	return nil
}

func (r *repo) Setup() error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := oauthappModelSetup(d, r.table); err != nil {
		err = db.WrapErr(err, "Failed to setup OAuth app model")
		if !errors.Is(err, db.ErrAuthz{}) {
			return err
		}
	}
	return nil
}
