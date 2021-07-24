package model

import (
	"strings"
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

const (
	keySeparator = "."
)

type (
	// Repo is an apikey repository
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
		hasher   hunter2.Hasher
		verifier *hunter2.Verifier
	}

	// Model is the db Apikey model
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

// NewInCtx creates a new apikey repo from a context and sets it in the context
func NewInCtx(inj governor.Injector) {
	SetCtxRepo(inj, NewCtx(inj))
}

// NewCtx creates a new apikey repo from a context
func NewCtx(inj governor.Injector) Repo {
	dbService := db.GetCtxDB(inj)
	return New(dbService)
}

// New creates a new apikey repository
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
		return nil, "", governor.ErrWithMsg(err, "Failed to create new api key id")
	}
	key, err := uid.New(keySize)
	if err != nil {
		return nil, "", governor.ErrWithMsg(err, "Failed to create new session key")
	}
	keystr := key.Base64()
	hash, err := r.hasher.Hash(keystr)
	if err != nil {
		return nil, "", governor.ErrWithMsg(err, "Failed to hash session key")
	}
	now := time.Now().Round(0).Unix()
	return &Model{
		Keyid:   userid + keySeparator + aid.Base64(),
		Userid:  userid,
		Scope:   scope,
		KeyHash: hash,
		Name:    name,
		Desc:    desc,
		Time:    now,
	}, keystr, nil
}

// ParseIDUserid gets the userid from a keyid
func ParseIDUserid(keyid string) (string, error) {
	k := strings.SplitN(keyid, keySeparator, 2)
	if len(k) != 2 {
		return "", governor.ErrWithMsg(nil, "Invalid apikey format")
	}
	return k[0], nil
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
		return "", governor.ErrWithMsg(err, "Failed to create new session key")
	}
	keystr := key.Base64()
	hash, err := r.hasher.Hash(keystr)
	if err != nil {
		return "", governor.ErrWithMsg(err, "Failed to hash session key")
	}
	now := time.Now().Round(0).Unix()
	m.KeyHash = hash
	m.Time = now
	return keystr, nil
}

func (r *repo) GetByID(keyid string) (*Model, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, code, err := apikeyModelGetModelEqKeyid(d, keyid)
	if err != nil {
		if code == 2 {
			return nil, governor.ErrWithKind(err, db.ErrNotFound{}, "No apikey found with that id")
		}
		return nil, governor.ErrWithMsg(err, "Failed to get apikey")
	}
	return m, nil
}

func (r *repo) GetUserKeys(userid string, limit, offset int) ([]Model, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := apikeyModelGetModelEqUseridOrdTime(d, userid, false, limit, offset)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get user apikeys")
	}
	return m, nil
}

func (r *repo) Insert(m *Model) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if code, err := apikeyModelInsert(d, m); err != nil {
		if code == 3 {
			return governor.ErrWithKind(err, db.ErrUnique{}, "Keyid must be unique")
		}
		return governor.ErrWithMsg(err, "Failed to insert apikey")
	}
	return nil
}

func (r *repo) Update(m *Model) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if _, err := apikeyModelUpdModelEqKeyid(d, m, m.Keyid); err != nil {
		return governor.ErrWithMsg(err, "Failed to update apikey")
	}
	return nil
}

func (r *repo) Delete(m *Model) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := apikeyModelDelEqKeyid(d, m.Keyid); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete apikey")
	}
	return nil
}

func (r *repo) DeleteUserKeys(userid string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := apikeyModelDelEqUserid(d, userid); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete user apikeys")
	}
	return nil
}

func (r *repo) Setup() error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if code, err := apikeyModelSetup(d); err != nil {
		if code != 5 {
			return governor.ErrWithMsg(err, "Failed to setup user apikeys model")
		}
	}
	return nil
}
