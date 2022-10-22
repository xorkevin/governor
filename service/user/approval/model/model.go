package model

import (
	"context"
	"errors"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	usermodel "xorkevin.dev/governor/service/user/model"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/hunter2"
	"xorkevin.dev/kerrors"
)

//go:generate forge model

const (
	keySize = 16
)

type (
	// Repo is an approval repository
	Repo interface {
		New(m *usermodel.Model) *Model
		ToUserModel(m *Model) *usermodel.Model
		ValidateCode(code string, m *Model) (bool, error)
		RehashCode(m *Model) (string, error)
		GetByID(ctx context.Context, userid string) (*Model, error)
		GetGroup(ctx context.Context, limit, offset int) ([]Model, error)
		Insert(ctx context.Context, m *Model) error
		Update(ctx context.Context, m *Model) error
		Delete(ctx context.Context, m *Model) error
		DeleteBefore(ctx context.Context, t int64) error
		Setup(ctx context.Context) error
	}

	repo struct {
		table    *approvalModelTable
		db       db.Database
		hasher   hunter2.Hasher
		verifier *hunter2.Verifier
	}

	// Model is the db Approval model
	//forge:model approval
	//forge:model:query approval
	Model struct {
		Userid       string `model:"userid,VARCHAR(31) PRIMARY KEY" query:"userid;getoneeq,userid;updeq,userid;deleq,userid"`
		Username     string `model:"username,VARCHAR(255) NOT NULL" query:"username"`
		PassHash     string `model:"pass_hash,VARCHAR(255) NOT NULL" query:"pass_hash"`
		Email        string `model:"email,VARCHAR(255) NOT NULL" query:"email"`
		FirstName    string `model:"first_name,VARCHAR(255) NOT NULL" query:"first_name"`
		LastName     string `model:"last_name,VARCHAR(255) NOT NULL" query:"last_name"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL;index" query:"creation_time;getgroup;deleq,creation_time|lt"`
		Approved     bool   `model:"approved,BOOL NOT NULL" query:"approved"`
		CodeHash     string `model:"code_hash,VARCHAR(255) NOT NULL" query:"code_hash"`
		CodeTime     int64  `model:"code_time,BIGINT NOT NULL" query:"code_time"`
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

// NewInCtx creates a new approval repo from a context and sets it in the context
func NewInCtx(inj governor.Injector, table string) {
	SetCtxRepo(inj, NewCtx(inj, table))
}

// NewCtx creates a new approval repo from a context
func NewCtx(inj governor.Injector, table string) Repo {
	dbService := db.GetCtxDB(inj)
	return New(dbService, table)
}

// New creates a new approval repository
func New(database db.Database, table string) Repo {
	hasher := hunter2.NewBlake2bHasher()
	verifier := hunter2.NewVerifier()
	verifier.RegisterHash(hasher)

	return &repo{
		table: &approvalModelTable{
			TableName: table,
		},
		db:       database,
		hasher:   hasher,
		verifier: verifier,
	}
}

func (r *repo) New(m *usermodel.Model) *Model {
	return &Model{
		Userid:       m.Userid,
		Username:     m.Username,
		PassHash:     m.PassHash,
		Email:        m.Email,
		FirstName:    m.FirstName,
		LastName:     m.LastName,
		CreationTime: m.CreationTime,
		Approved:     false,
		CodeHash:     "",
		CodeTime:     0,
	}
}

func (r *repo) ToUserModel(m *Model) *usermodel.Model {
	return &usermodel.Model{
		Userid:       m.Userid,
		Username:     m.Username,
		PassHash:     m.PassHash,
		Email:        m.Email,
		FirstName:    m.FirstName,
		LastName:     m.LastName,
		CreationTime: m.CreationTime,
	}
}

func (r *repo) ValidateCode(code string, m *Model) (bool, error) {
	ok, err := r.verifier.Verify(code, m.CodeHash)
	if err != nil {
		return false, kerrors.WithMsg(err, "Failed to verify code")
	}
	return ok, nil
}

func (r *repo) RehashCode(m *Model) (string, error) {
	code, err := uid.New(keySize)
	if err != nil {
		return "", kerrors.WithMsg(err, "Failed to create new user code")
	}
	codestr := code.Base64()
	codehash, err := r.hasher.Hash(codestr)
	if err != nil {
		return "", kerrors.WithMsg(err, "Failed to hash new user code")
	}
	now := time.Now().Round(0).Unix()
	m.Approved = true
	m.CodeHash = codehash
	m.CodeTime = now
	return codestr, nil
}

func (r *repo) GetByID(ctx context.Context, userid string) (*Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelEqUserid(ctx, d, userid)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get user")
	}
	return m, nil
}

func (r *repo) GetGroup(ctx context.Context, limit, offset int) ([]Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelOrdCreationTime(ctx, d, true, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get user approvals")
	}
	return m, nil
}

func (r *repo) Insert(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.Insert(ctx, d, m); err != nil {
		return kerrors.WithMsg(err, "Failed to insert user")
	}
	return nil
}

func (r *repo) Update(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.UpdModelEqUserid(ctx, d, m, m.Userid); err != nil {
		return kerrors.WithMsg(err, "Failed to update user approval")
	}
	return nil
}

func (r *repo) Delete(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelEqUserid(ctx, d, m.Userid); err != nil {
		return kerrors.WithMsg(err, "Failed to delete user approval")
	}
	return nil
}

func (r *repo) DeleteBefore(ctx context.Context, t int64) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelLtCreationTime(ctx, d, t); err != nil {
		return kerrors.WithMsg(err, "Failed to delete user approvals")
	}
	return nil
}

func (r *repo) Setup(ctx context.Context) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.Setup(ctx, d); err != nil {
		err = kerrors.WithMsg(err, "Failed to setup user approval model")
		if !errors.Is(err, db.ErrorAuthz{}) {
			return err
		}
	}
	return nil
}
