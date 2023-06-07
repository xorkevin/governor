package aclmodel

import (
	"context"
	"errors"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/kerrors"
)

//go:generate forge model

type (
	// Repo is an acl repository
	Repo interface {
		Setup(ctx context.Context) error
	}

	repo struct {
		table *aclModelTable
		db    db.Database
	}

	// Model is the db acl entry model
	//forge:model acl
	Model struct {
		ObjNS   string `model:"obj_ns,VARCHAR(255)"`
		ObjKey  string `model:"obj_key,VARCHAR(255);index,sub_ns,sub_key,sub_pred,obj_pred,obj_ns"`
		ObjPred string `model:"obj_pred,VARCHAR(255)"`
		SubNS   string `model:"sub_ns,VARCHAR(255)"`
		SubKey  string `model:"sub_key,VARCHAR(255)"`
		SubPred string `model:"sub_pred,VARCHAR(255), PRIMARY KEY (obj_ns, obj_key, obj_pred, sub_ns, sub_key, sub_pred)"`
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

// NewInCtx creates a new acl repo from a context and sets it in the context
func NewInCtx(inj governor.Injector, table string) {
	SetCtxRepo(inj, NewCtx(inj, table))
}

// NewCtx creates a new acl repo from a context
func NewCtx(inj governor.Injector, table string) Repo {
	dbService := db.GetCtxDB(inj)
	return New(dbService, table)
}

// New creates a new acl repository
func New(database db.Database, table string) Repo {
	return &repo{
		table: &aclModelTable{
			TableName: table,
		},
		db: database,
	}
}

// Setup creates a new acl table
func (r *repo) Setup(ctx context.Context) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.Setup(ctx, d); err != nil {
		err = kerrors.WithMsg(err, "Failed to setup acl model")
		if !errors.Is(err, db.ErrAuthz) {
			return err
		}
	}
	return nil
}
