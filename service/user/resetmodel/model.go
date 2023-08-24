package resetmodel

import (
	"context"
	"errors"
	"time"

	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/hunter2/h2hash"
	"xorkevin.dev/hunter2/h2hash/blake2b"
	"xorkevin.dev/kerrors"
)

//go:generate forge model

type (
	// Repo is a user reset request repository
	Repo interface {
		New(userid, kind string) *Model
		ValidateCode(code string, m *Model) (bool, error)
		RehashCode(m *Model) (string, error)
		GetByID(ctx context.Context, userid, kind string) (*Model, error)
		Insert(ctx context.Context, m *Model) error
		Update(ctx context.Context, m *Model) error
		Delete(ctx context.Context, userid, kind string) error
		DeleteByUserid(ctx context.Context, userid string) error
		DeleteBefore(ctx context.Context, t int64) error
		Setup(ctx context.Context) error
	}

	repo struct {
		table    *resetModelTable
		db       db.Database
		hasher   h2hash.Hasher
		verifier *h2hash.Verifier
	}

	// Model is the user reset request model
	//forge:model reset
	//forge:model:query reset
	Model struct {
		Userid   string `model:"userid,VARCHAR(31)"`
		Kind     string `model:"kind,VARCHAR(255)"`
		CodeHash string `model:"code_hash,VARCHAR(255) NOT NULL"`
		CodeTime int64  `model:"code_time,BIGINT NOT NULL"`
		Params   string `model:"params,VARCHAR(4096)"`
	}
)

// New creates a new user reset request repo
func New(database db.Database, table string) Repo {
	hasher := blake2b.New(blake2b.Config{})
	verifier := h2hash.NewVerifier()
	verifier.Register(hasher)

	return &repo{
		table: &resetModelTable{
			TableName: table,
		},
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
	ok, err := r.verifier.Verify([]byte(code), m.CodeHash)
	if err != nil {
		return false, kerrors.WithMsg(err, "Failed to verify code")
	}
	return ok, nil
}

func (r *repo) RehashCode(m *Model) (string, error) {
	codebytes, err := uid.NewKey()
	if err != nil {
		return "", kerrors.WithMsg(err, "Failed to create new code")
	}
	code := codebytes.Base64()
	codehash, err := r.hasher.Hash([]byte(code))
	if err != nil {
		return "", kerrors.WithMsg(err, "Failed to hash new code")
	}
	now := time.Now().Round(0).Unix()
	m.CodeHash = codehash
	m.CodeTime = now
	return code, nil
}

func (r *repo) GetByID(ctx context.Context, userid, kind string) (*Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelByUserKind(ctx, d, userid, kind)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get reset code")
	}
	return m, nil
}

func (r *repo) Insert(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.Insert(ctx, d, m); err != nil {
		return kerrors.WithMsg(err, "Failed to insert new reset code")
	}
	return nil
}

func (r *repo) Update(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.UpdModelByUserKind(ctx, d, m, m.Userid, m.Kind); err != nil {
		return kerrors.WithMsg(err, "Failed to update reset code")
	}
	return nil
}

func (r *repo) Delete(ctx context.Context, userid, kind string) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelByUserKind(ctx, d, userid, kind); err != nil {
		return kerrors.WithMsg(err, "Failed to delete reset code")
	}
	return nil
}

func (r *repo) DeleteByUserid(ctx context.Context, userid string) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelByUserid(ctx, d, userid); err != nil {
		return kerrors.WithMsg(err, "Failed to delete reset codes")
	}
	return nil
}

func (r *repo) DeleteBefore(ctx context.Context, t int64) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelBeforeCodeTime(ctx, d, t); err != nil {
		return kerrors.WithMsg(err, "Failed to delete reset codes")
	}
	return nil
}

func (r *repo) Setup(ctx context.Context) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.Setup(ctx, d); err != nil {
		err = kerrors.WithMsg(err, "Failed to setup user reset code model")
		if !errors.Is(err, db.ErrAuthz) {
			return err
		}
	}
	return nil
}
