package oauthmodel

import (
	"net/http"
	"time"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/hunter2"
)

//go:generate forge model -m Model -t oauthapps -p oauthapp -o model_gen.go Model

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
		Insert(m *Model) error
		Update(m *Model) error
		DeleteCreatorApps(creatorid string) error
		Delete(m *Model) error
		Setup() error
	}

	repo struct {
		db       db.Database
		hasher   *hunter2.Blake2bHasher
		verifier *hunter2.Verifier
	}

	// Model is the db OAuth app model
	Model struct {
		ClientID     string `model:"clientid,VARCHAR(31) PRIMARY KEY" query:"clientid,getoneeq,clientid;updeq,clientid;deleq,clientid"`
		Name         string `model:"name,VARCHAR(255) NOT NULL" query:"name"`
		URL          string `model:"url,VARCHAR(255) NOT NULL" query:"url"`
		RedirectURI  string `model:"redirect_uri,VARCHAR(255) NOT NULL" query:"redirect_uri"`
		Logo         string `model:"logo,VARCHAR(4095)" query:"logo"`
		KeyHash      string `model:"keyhash,VARCHAR(255) NOT NULL" query:"keyhash"`
		Time         int64  `model:"time,BIGINT NOT NULL;index" query:"time,getgroup;getgroupeq,creator_id"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL" query:"creation_time"`
		CreatorID    string `model:"creator_id,VARCHAR(31);index" query:"creator_id,deleq,creator_id"`
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

func NewInCtx(inj governor.Injector) {
	SetCtxRepo(inj, NewCtx(inj))
}

func NewCtx(inj governor.Injector) Repo {
	dbService := db.GetCtxDB(inj)
	return New(dbService)
}

// New creates a new OAuth app repository
func New(database db.Database) Repo {
	hasher := hunter2.NewBlake2bHasher()
	verifier := hunter2.NewVerifier()
	verifier.RegisterHash(hasher)

	return &repo{
		db:       database,
		hasher:   hasher,
		verifier: verifier,
	}
}

func (r *repo) New(name, url, redirectURI, creatorID string) (*Model, string, error) {
	mUID, err := uid.New(uidSize)
	if err != nil {
		return nil, "", governor.NewError("Failed to create new uid", http.StatusInternalServerError, err)
	}
	clientid := mUID.Base64()

	key, err := uid.New(keySize)
	if err != nil {
		return nil, "", governor.NewError("Failed to create oauth client secret", http.StatusInternalServerError, err)
	}
	keystr := key.Base64()
	hash, err := r.hasher.Hash(keystr)
	if err != nil {
		return nil, "", governor.NewError("Failed to hash oauth client secret", http.StatusInternalServerError, err)
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
		return false, governor.NewError("Failed to verify key", http.StatusInternalServerError, err)
	}
	return ok, nil
}

func (r *repo) RehashKey(m *Model) (string, error) {
	key, err := uid.New(keySize)
	if err != nil {
		return "", governor.NewError("Failed to create oauth client secret", http.StatusInternalServerError, err)
	}
	keystr := key.Base64()
	keyhash, err := r.hasher.Hash(keystr)
	if err != nil {
		return "", governor.NewError("Failed to hash oauth client secret", http.StatusInternalServerError, err)
	}
	now := time.Now().Round(0).Unix()
	m.KeyHash = keyhash
	m.Time = now
	return keystr, nil
}

func (r *repo) GetByID(clientid string) (*Model, error) {
	db, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, code, err := oauthappModelGetModelEqClientID(db, clientid)
	if err != nil {
		if code == 2 {
			return nil, governor.NewError("No OAuth app found with that id", http.StatusNotFound, err)
		}
		return nil, governor.NewError("Failed to get OAuth app config", http.StatusInternalServerError, err)
	}
	return m, nil
}

func (r *repo) GetApps(limit, offset int, creatorid string) ([]Model, error) {
	db, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	if creatorid == "" {
		m, err := oauthappModelGetModelOrdTime(db, false, limit, offset)
		if err != nil {
			return nil, governor.NewError("Failed to get OAuth app configs", http.StatusInternalServerError, err)
		}
		return m, nil
	}
	m, err := oauthappModelGetModelEqCreatorIDOrdTime(db, creatorid, false, limit, offset)
	if err != nil {
		return nil, governor.NewError("Failed to get OAuth app configs", http.StatusInternalServerError, err)
	}
	return m, nil
}

func (r *repo) Insert(m *Model) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if code, err := oauthappModelInsert(db, m); err != nil {
		if code == 3 {
			return governor.NewError("clientid must be unique", http.StatusBadRequest, err)
		}
		return governor.NewError("Failed to insert OAuth app config", http.StatusInternalServerError, err)
	}
	return nil
}

func (r *repo) Update(m *Model) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if _, err := oauthappModelUpdModelEqClientID(db, m, m.ClientID); err != nil {
		return governor.NewError("Failed to update OAuth app config", http.StatusInternalServerError, err)
	}
	return nil
}

func (r *repo) Delete(m *Model) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := oauthappModelDelEqClientID(db, m.ClientID); err != nil {
		return governor.NewError("Failed to delete OAuth app config", http.StatusInternalServerError, err)
	}
	return nil
}

func (r *repo) DeleteCreatorApps(creatorid string) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := oauthappModelDelEqCreatorID(db, creatorid); err != nil {
		return governor.NewError("Failed to delete OAuth app configs", http.StatusInternalServerError, err)
	}
	return nil
}

func (r *repo) Setup() error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := oauthappModelSetup(db); err != nil {
		return governor.NewError("Failed to setup OAuth app model", http.StatusInternalServerError, err)
	}
	return nil
}
