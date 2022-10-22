package model

import (
	"context"
	"errors"
	"strings"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/hunter2"
	"xorkevin.dev/kerrors"
)

//go:generate forge model

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
		db       db.Database
		hasher   hunter2.Hasher
		verifier *hunter2.Verifier
	}

	// Model is the db Apikey model
	//forge:model apikey
	//forge:model:query apikey
	Model struct {
		Keyid   string `model:"keyid,VARCHAR(63) PRIMARY KEY" query:"keyid;getoneeq,keyid;deleq,keyid;deleq,keyid|in"`
		Userid  string `model:"userid,VARCHAR(31) NOT NULL;index" query:"userid"`
		Scope   string `model:"scope,VARCHAR(4095) NOT NULL" query:"scope"`
		KeyHash string `model:"keyhash,VARCHAR(127) NOT NULL" query:"keyhash"`
		Name    string `model:"name,VARCHAR(255) NOT NULL" query:"name"`
		Desc    string `model:"description,VARCHAR(255)" query:"description"`
		Time    int64  `model:"time,BIGINT NOT NULL;index,userid" query:"time;getgroupeq,userid"`
	}

	//forge:model:query apikey
	apikeyHash struct {
		KeyHash string `query:"keyhash;updeq,keyid"`
		Time    int64  `query:"time"`
	}

	//forge:model:query apikey
	apikeyProps struct {
		Scope string `query:"scope;updeq,keyid"`
		Name  string `query:"name"`
		Desc  string `query:"description"`
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
func NewInCtx(inj governor.Injector, table string) {
	SetCtxRepo(inj, NewCtx(inj, table))
}

// NewCtx creates a new apikey repo from a context
func NewCtx(inj governor.Injector, table string) Repo {
	dbService := db.GetCtxDB(inj)
	return New(dbService, table)
}

// New creates a new apikey repository
func New(database db.Database, table string) Repo {
	hasher := hunter2.NewBlake2bHasher()
	verifier := hunter2.NewVerifier()
	verifier.RegisterHash(hasher)

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
	aid, err := uid.New(uidSize)
	if err != nil {
		return nil, "", kerrors.WithMsg(err, "Failed to create new api key id")
	}
	key, err := uid.New(keySize)
	if err != nil {
		return nil, "", kerrors.WithMsg(err, "Failed to create new api key")
	}
	keystr := key.Base64()
	hash, err := r.hasher.Hash(keystr)
	if err != nil {
		return nil, "", kerrors.WithMsg(err, "Failed to hash api key")
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
	userid, _, ok := strings.Cut(keyid, keySeparator)
	if !ok {
		return "", kerrors.WithMsg(nil, "Invalid apikey format")
	}
	return userid, nil
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
		return "", kerrors.WithMsg(err, "Failed to create new api key")
	}
	keystr := key.Base64()
	hash, err := r.hasher.Hash(keystr)
	if err != nil {
		return "", kerrors.WithMsg(err, "Failed to hash api key")
	}
	d, err := r.db.DB(ctx)
	if err != nil {
		return "", err
	}
	now := time.Now().Round(0).Unix()
	if err := r.table.UpdapikeyHashEqKeyid(ctx, d, &apikeyHash{
		KeyHash: hash,
		Time:    now,
	}, m.Keyid); err != nil {
		return "", kerrors.WithMsg(err, "Failed to update apikey")
	}
	m.KeyHash = hash
	m.Time = now
	return keystr, nil
}

func (r *repo) GetByID(ctx context.Context, keyid string) (*Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelEqKeyid(ctx, d, keyid)
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
	m, err := r.table.GetModelEqUseridOrdTime(ctx, d, userid, false, limit, offset)
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
	if err := r.table.UpdapikeyPropsEqKeyid(ctx, d, &apikeyProps{
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
	if err := r.table.DelEqKeyid(ctx, d, m.Keyid); err != nil {
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
	if err := r.table.DelHasKeyid(ctx, d, keyids); err != nil {
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
		err = kerrors.WithMsg(err, "Failed to setup user apikeys model")
		if !errors.Is(err, db.ErrorAuthz{}) {
			return err
		}
	}
	return nil
}
