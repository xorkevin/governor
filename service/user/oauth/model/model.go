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
	defaultUIDSize = 8
	keySize        = 32
)

type (
	// Repo is an OAuthApp repository
	Repo interface {
		New(appid, name, desc, callbackURL string) (*Model, string, error)
		NewAuto(name, desc, callbackURL string) (*Model, string, error)
		GetByID(appid string) (*Model, error)
		Insert(m *Model) error
		Update(m *Model) error
		Delete(m *Model) error
		Setup() error
	}

	repo struct {
		db       db.Database
		hasher   *hunter2.Blake2bHasher
		verifier *hunter2.Verifier
	}

	// Model is the db OAuthApp model
	Model struct {
		AppID        string `model:"appid,VARCHAR(31) PRIMARY KEY" query:"appid,getoneeq,appid;updeq,appid;deleq,appid"`
		Name         string `model:"name,VARCHAR(255) NOT NULL" query:"name"`
		Desc         string `model:"description,VARCHAR(255) NOT NULL" query:"description"`
		CallbackURL  string `model:"callback_url,VARCHAR(255) NOT NULL" query:"callback_url"`
		KeyHash      string `model:"keyhash,VARCHAR(255) NOT NULL" query:"keyhash"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL" query:"creation_time"`
	}
)

// New creates a new OAuthApp repository
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

func (r *repo) New(appid, name, desc, callbackURL string) (*Model, string, error) {
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
		AppID:        appid,
		Name:         name,
		Desc:         desc,
		CallbackURL:  callbackURL,
		KeyHash:      hash,
		CreationTime: now,
	}, keystr, nil
}

func (r *repo) NewAuto(name, desc, callbackURL string) (*Model, string, error) {
	mUID, err := uid.New(defaultUIDSize)
	if err != nil {
		return nil, "", governor.NewError("Failed to create new uid", http.StatusInternalServerError, err)
	}
	return r.New(mUID.Base64(), name, desc, callbackURL)
}

func (r *repo) GetByID(appid string) (*Model, error) {
	db, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, code, err := oauthappModelGetModelEqAppID(db, appid)
	if err != nil {
		if code == 2 {
			return nil, governor.NewError("No OAuth app found with that id", http.StatusNotFound, err)
		}
		return nil, governor.NewError("Failed to get OAuth app config", http.StatusInternalServerError, err)
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
			return governor.NewError("AppID must be unique", http.StatusBadRequest, err)
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
	if _, err := oauthappModelUpdModelEqAppID(db, m, m.AppID); err != nil {
		return governor.NewError("Failed to update OAuth app config", http.StatusInternalServerError, err)
	}
	return nil
}

func (r *repo) Delete(m *Model) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := oauthappModelDelEqAppID(db, m.AppID); err != nil {
		return governor.NewError("Failed to delete OAuth app config", http.StatusInternalServerError, err)
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
