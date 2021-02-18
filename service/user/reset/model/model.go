package model

import (
	"net/http"
	"time"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/hunter2"
)

//go:generate forge model -m Model -t userresets -p reset -o model_gen.go Model

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
		db       db.Database
		hasher   *hunter2.Blake2bHasher
		verifier *hunter2.Verifier
	}

	// Model is the user reset request model
	Model struct {
		Userid   string `model:"userid,VARCHAR(31)" query:"userid,deleq,userid"`
		Kind     string `model:"kind,VARCHAR(255), PRIMARY KEY (userid, kind)" query:"kind,getoneeq,userid,kind;updeq,userid,kind;deleq,userid,kind"`
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
func NewInCtx(inj governor.Injector) {
	SetCtxRepo(inj, NewCtx(inj))
}

// NewCtx creates a new user reset request repo from a context
func NewCtx(inj governor.Injector) Repo {
	dbService := db.GetCtxDB(inj)
	return New(dbService)
}

// New creates a new user reset request repo
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
		return false, governor.NewError("Failed to verify code", http.StatusInternalServerError, err)
	}
	return ok, nil
}

func (r *repo) RehashCode(m *Model) (string, error) {
	code, err := uid.New(keySize)
	if err != nil {
		return "", governor.NewError("Failed to create new code", http.StatusInternalServerError, err)
	}
	codestr := code.Base64()
	codehash, err := r.hasher.Hash(codestr)
	if err != nil {
		return "", governor.NewError("Failed to hash new code", http.StatusInternalServerError, err)
	}
	now := time.Now().Round(0).Unix()
	m.CodeHash = codehash
	m.CodeTime = now
	return codestr, nil
}

func (r *repo) GetByID(userid, kind string) (*Model, error) {
	db, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, code, err := resetModelGetModelEqUseridEqKind(db, userid, kind)
	if err != nil {
		if code == 2 {
			return nil, governor.NewError("Code does not exist", http.StatusNotFound, err)
		}
		return nil, governor.NewError("Failed to get reset code", http.StatusInternalServerError, err)
	}
	return m, nil
}

func (r *repo) Insert(m *Model) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if code, err := resetModelInsert(db, m); err != nil {
		if code == 3 {
			return governor.NewError("Reset code already exists", http.StatusBadRequest, err)
		}
		return governor.NewError("Failed to insert new reset code", http.StatusInternalServerError, err)
	}
	return nil
}

func (r *repo) Update(m *Model) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if _, err := resetModelUpdModelEqUseridEqKind(db, m, m.Userid, m.Kind); err != nil {
		return governor.NewError("Failed to update reset code", http.StatusInternalServerError, err)
	}
	return nil
}

func (r *repo) Delete(userid, kind string) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := resetModelDelEqUseridEqKind(db, userid, kind); err != nil {
		return governor.NewError("Failed to delete reset code", http.StatusInternalServerError, err)
	}
	return nil
}

func (r *repo) DeleteByUserid(userid string) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := resetModelDelEqUserid(db, userid); err != nil {
		return governor.NewError("Failed to delete reset codes", http.StatusInternalServerError, err)
	}
	return nil
}

func (r *repo) Setup() error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := resetModelSetup(db); err != nil {
		return governor.NewError("Failed to setup user reset code model", http.StatusInternalServerError, err)
	}
	return nil
}
