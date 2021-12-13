package model

import (
	"errors"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/hunter2"
)

//go:generate forge model -m Model -p reset -o model_gen.go Model

const (
	keySize = 16
)

type (
	// Repo is a user reset request repository
	Repo interface {
		New(userid, kind string) *Model
		ValidateCode(code string, m *Model) (bool, error)
		RehashCode(m *Model) (string, error)
		GetByID(userid, kind string) (*Model, error)
		Insert(m *Model) error
		Update(m *Model) error
		Delete(userid, kind string) error
		DeleteByUserid(userid string) error
		Setup() error
	}

	repo struct {
		table    string
		db       db.Database
		hasher   hunter2.Hasher
		verifier *hunter2.Verifier
	}

	// Model is the user reset request model
	Model struct {
		Userid   string `model:"userid,VARCHAR(31)" query:"userid;deleq,userid"`
		Kind     string `model:"kind,VARCHAR(255), PRIMARY KEY (userid, kind)" query:"kind;getoneeq,userid,kind;updeq,userid,kind;deleq,userid,kind"`
		CodeHash string `model:"code_hash,VARCHAR(255) NOT NULL" query:"code_hash"`
		CodeTime int64  `model:"code_time,BIGINT NOT NULL" query:"code_time"`
		Params   string `model:"params,VARCHAR(4096)" query:"params"`
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

// NewInCtx creates a new user reset request repo from a context and sets it in the context
func NewInCtx(inj governor.Injector, table string) {
	SetCtxRepo(inj, NewCtx(inj, table))
}

// NewCtx creates a new user reset request repo from a context
func NewCtx(inj governor.Injector, table string) Repo {
	dbService := db.GetCtxDB(inj)
	return New(dbService, table)
}

// New creates a new user reset request repo
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

func (r *repo) New(userid, kind string) *Model {
	return &Model{
		Userid:   userid,
		Kind:     kind,
		CodeHash: "",
		CodeTime: 0,
		Params:   "",
	}
}

func (r *repo) ValidateCode(code string, m *Model) (bool, error) {
	ok, err := r.verifier.Verify(code, m.CodeHash)
	if err != nil {
		return false, governor.ErrWithMsg(err, "Failed to verify code")
	}
	return ok, nil
}

func (r *repo) RehashCode(m *Model) (string, error) {
	code, err := uid.New(keySize)
	if err != nil {
		return "", governor.ErrWithMsg(err, "Failed to create new code")
	}
	codestr := code.Base64()
	codehash, err := r.hasher.Hash(codestr)
	if err != nil {
		return "", governor.ErrWithMsg(err, "Failed to hash new code")
	}
	now := time.Now().Round(0).Unix()
	m.CodeHash = codehash
	m.CodeTime = now
	return codestr, nil
}

func (r *repo) GetByID(userid, kind string) (*Model, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := resetModelGetModelEqUseridEqKind(d, r.table, userid, kind)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get reset code")
	}
	return m, nil
}

func (r *repo) Insert(m *Model) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := resetModelInsert(d, r.table, m); err != nil {
		return db.WrapErr(err, "Failed to insert new reset code")
	}
	return nil
}

func (r *repo) Update(m *Model) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := resetModelUpdModelEqUseridEqKind(d, r.table, m, m.Userid, m.Kind); err != nil {
		return db.WrapErr(err, "Failed to update reset code")
	}
	return nil
}

func (r *repo) Delete(userid, kind string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := resetModelDelEqUseridEqKind(d, r.table, userid, kind); err != nil {
		return db.WrapErr(err, "Failed to delete reset code")
	}
	return nil
}

func (r *repo) DeleteByUserid(userid string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := resetModelDelEqUserid(d, r.table, userid); err != nil {
		return db.WrapErr(err, "Failed to delete reset codes")
	}
	return nil
}

func (r *repo) Setup() error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := resetModelSetup(d, r.table); err != nil {
		err = db.WrapErr(err, "Failed to setup user reset code model")
		if !errors.Is(err, db.ErrAuthz{}) {
			return err
		}
	}
	return nil
}
