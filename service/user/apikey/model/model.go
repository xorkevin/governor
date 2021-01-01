package apikeymodel

import (
	"context"
	"net/http"
	"time"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/hunter2"
)

//go:generate forge model -m Model -t userapikeys -p apikey -o model_gen.go Model

const (
	uidSize = 8
	keySize = 32
)

type (
	Repo interface {
		New(userid string, scope string, name, desc string) (*Model, string, error)
		ValidateKey(key string, m *Model) (bool, error)
		RehashKey(m *Model) (string, error)
		GetByID(keyid string) (*Model, error)
		GetUserKeys(userid string, limit, offset int) ([]Model, error)
		Insert(m *Model) error
		Update(m *Model) error
		Delete(m *Model) error
		DeleteUserKeys(userid string) error
		Setup() error
	}

	repo struct {
		db       db.Database
		hasher   *hunter2.Blake2bHasher
		verifier *hunter2.Verifier
	}

	Model struct {
		Keyid   string `model:"keyid,VARCHAR(63) PRIMARY KEY" query:"keyid,getoneeq,keyid;updeq,keyid;deleq,keyid"`
		Userid  string `model:"userid,VARCHAR(31) NOT NULL;index" query:"userid,deleq,userid"`
		Scope   string `model:"scope,VARCHAR(4095) NOT NULL" query:"scope"`
		KeyHash string `model:"keyhash,VARCHAR(127) NOT NULL" query:"keyhash"`
		Name    string `model:"name,VARCHAR(255)" query:"name"`
		Desc    string `model:"description,VARCHAR(255)" query:"description"`
		Time    int64  `model:"time,BIGINT NOT NULL;index" query:"time,getgroupeq,userid"`
	}

	ctxKeyRepo struct{}
)

// GetCtxRepo returns a Repo from the context
func GetCtxRepo(ctx context.Context, r Repo) Repo {
	v := ctx.Value(ctxKeyRepo{})
	if v == nil {
		return nil
	}
	return v.(Repo)
}

// SetCtxRepo sets a Repo in the context
func SetCtxRepo(ctx context.Context, r Repo) context.Context {
	return context.WithValue(ctx, ctxKeyRepo{}, r)
}

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

func (r *repo) New(userid string, scope string, name, desc string) (*Model, string, error) {
	aid, err := uid.New(uidSize)
	if err != nil {
		return nil, "", governor.NewError("Failed to create new api key id", http.StatusInternalServerError, err)
	}
	key, err := uid.New(keySize)
	if err != nil {
		return nil, "", governor.NewError("Failed to create new session key", http.StatusInternalServerError, err)
	}
	keystr := key.Base64()
	hash, err := r.hasher.Hash(keystr)
	if err != nil {
		return nil, "", governor.NewError("Failed to hash session key", http.StatusInternalServerError, err)
	}
	now := time.Now().Round(0).Unix()
	return &Model{
		Keyid:   userid + "|" + aid.Base64(),
		Userid:  userid,
		Scope:   scope,
		KeyHash: hash,
		Name:    name,
		Desc:    desc,
		Time:    now,
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
		return "", governor.NewError("Failed to create new session key", http.StatusInternalServerError, err)
	}
	keystr := key.Base64()
	hash, err := r.hasher.Hash(keystr)
	if err != nil {
		return "", governor.NewError("Failed to hash session key", http.StatusInternalServerError, err)
	}
	now := time.Now().Round(0).Unix()
	m.KeyHash = hash
	m.Time = now
	return keystr, nil
}

func (r *repo) GetByID(keyid string) (*Model, error) {
	db, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, code, err := apikeyModelGetModelEqKeyid(db, keyid)
	if err != nil {
		if code == 2 {
			return nil, governor.NewError("No apikey found with that id", http.StatusNotFound, err)
		}
		return nil, governor.NewError("Failed to get apikey", http.StatusInternalServerError, err)
	}
	return m, nil
}

func (r *repo) GetUserKeys(userid string, limit, offset int) ([]Model, error) {
	db, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := apikeyModelGetModelEqUseridOrdTime(db, userid, false, limit, offset)
	if err != nil {
		return nil, governor.NewError("Failed to get user apikeys", http.StatusInternalServerError, err)
	}
	return m, nil
}

func (r *repo) Insert(m *Model) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if code, err := apikeyModelInsert(db, m); err != nil {
		if code == 3 {
			return governor.NewError("Keyid must be unique", http.StatusBadRequest, err)
		}
		return governor.NewError("Failed to insert apikey", http.StatusInternalServerError, err)
	}
	return nil
}

func (r *repo) Update(m *Model) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if _, err := apikeyModelUpdModelEqKeyid(db, m, m.Keyid); err != nil {
		return governor.NewError("Failed to update apikey", http.StatusInternalServerError, err)
	}
	return nil
}

func (r *repo) Delete(m *Model) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := apikeyModelDelEqKeyid(db, m.Keyid); err != nil {
		return governor.NewError("Failed to delete apikey", http.StatusInternalServerError, err)
	}
	return nil
}

func (r *repo) DeleteUserKeys(userid string) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := apikeyModelDelEqUserid(db, userid); err != nil {
		return governor.NewError("Failed to delete user apikeys", http.StatusInternalServerError, err)
	}
	return nil
}

func (r *repo) Setup() error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := apikeyModelSetup(db); err != nil {
		return governor.NewError("Failed to setup user apikeys model", http.StatusInternalServerError, err)
	}
	return nil
}
