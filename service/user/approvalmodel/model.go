package approvalmodel

import (
	"context"
	"time"

	"xorkevin.dev/governor/service/dbsql"
	"xorkevin.dev/governor/service/user/usermodel"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/hunter2/h2hash"
	"xorkevin.dev/hunter2/h2hash/blake2b"
	"xorkevin.dev/kerrors"
)

//go:generate forge model

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
		db       dbsql.Database
		hasher   h2hash.Hasher
		verifier *h2hash.Verifier
	}

	// Model is the db Approval model
	//forge:model approval
	//forge:model:query approval
	Model struct {
		Userid       string `model:"userid,VARCHAR(31) PRIMARY KEY"`
		Username     string `model:"username,VARCHAR(255) NOT NULL UNIQUE"`
		PassHash     string `model:"pass_hash,VARCHAR(255) NOT NULL"`
		Email        string `model:"email,VARCHAR(255) NOT NULL UNIQUE"`
		FirstName    string `model:"first_name,VARCHAR(255) NOT NULL"`
		LastName     string `model:"last_name,VARCHAR(255) NOT NULL"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL"`
		Approved     bool   `model:"approved,BOOL NOT NULL"`
		CodeHash     string `model:"code_hash,VARCHAR(255) NOT NULL"`
		CodeTime     int64  `model:"code_time,BIGINT NOT NULL"`
	}
)

// New creates a new approval repository
func New(database dbsql.Database, table string) Repo {
	hasher := blake2b.New(blake2b.Config{})
	verifier := h2hash.NewVerifier()
	verifier.Register(hasher)

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
	ok, err := r.verifier.Verify([]byte(code), m.CodeHash)
	if err != nil {
		return false, kerrors.WithMsg(err, "Failed to verify code")
	}
	return ok, nil
}

func (r *repo) RehashCode(m *Model) (string, error) {
	codebytes, err := uid.NewKey()
	if err != nil {
		return "", kerrors.WithMsg(err, "Failed to create new user code")
	}
	code := codebytes.Base64()
	codehash, err := r.hasher.Hash([]byte(code))
	if err != nil {
		return "", kerrors.WithMsg(err, "Failed to hash new user code")
	}
	now := time.Now().Round(0).Unix()
	m.Approved = true
	m.CodeHash = codehash
	m.CodeTime = now
	return code, nil
}

func (r *repo) GetByID(ctx context.Context, userid string) (*Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelByID(ctx, d, userid)
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
	m, err := r.table.GetModelAll(ctx, d, limit, offset)
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
	if err := r.table.UpdModelByID(ctx, d, m, m.Userid); err != nil {
		return kerrors.WithMsg(err, "Failed to update user approval")
	}
	return nil
}

func (r *repo) Delete(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelByID(ctx, d, m.Userid); err != nil {
		return kerrors.WithMsg(err, "Failed to delete user approval")
	}
	return nil
}

func (r *repo) DeleteBefore(ctx context.Context, t int64) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelBeforeCreationTime(ctx, d, t); err != nil {
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
		return kerrors.WithMsg(err, "Failed to setup user approval model")
	}
	return nil
}
