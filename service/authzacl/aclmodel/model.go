package aclmodel

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"xorkevin.dev/forge/model/sqldb"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/kerrors"
)

//go:generate forge model

type (
	// Repo is an acl repository
	Repo interface {
		Read(ctx context.Context, ns, key string, pred string, limit, offset int) ([]Subject, error)
		Insert(ctx context.Context, m []*Model) error
		DeleteForSub(ctx context.Context, sub Subject, objs []Object) error
		DeleteForObj(ctx context.Context, obj Object, sub []Subject) error
		Check(ctx context.Context, objns, objkey string, pred string, subns, subkey string) (bool, error)
		Setup(ctx context.Context) error
	}

	repo struct {
		table *aclModelTable
		db    db.Database
	}

	// Model is the db acl entry model
	//forge:model acl
	//forge:model:query acl
	Model struct {
		ObjNS   string `model:"obj_ns,VARCHAR(255)" query:"obj_ns;deleq,obj_ns,obj_key;deleq,sub_ns,sub_key,sub_pred"`
		ObjKey  string `model:"obj_key,VARCHAR(255);index,sub_ns,sub_key,sub_pred,obj_pred,obj_ns" query:"obj_key"`
		ObjPred string `model:"obj_pred,VARCHAR(255)" query:"obj_pred"`
		SubNS   string `model:"sub_ns,VARCHAR(255)" query:"sub_ns"`
		SubKey  string `model:"sub_key,VARCHAR(255)" query:"sub_key"`
		SubPred string `model:"sub_pred,VARCHAR(255), PRIMARY KEY (obj_ns, obj_key, obj_pred, sub_ns, sub_key, sub_pred)" query:"sub_pred"`
	}

	//forge:model:query acl
	Subject struct {
		SubNS   string `query:"sub_ns;getoneeq,obj_ns,obj_key,obj_pred,sub_ns,sub_key,sub_pred"`
		SubKey  string `query:"sub_key"`
		SubPred string `query:"sub_pred"`
	}

	Object struct {
		ObjNS   string `query:"obj_ns"`
		ObjKey  string `query:"obj_key"`
		ObjPred string `query:"obj_pred"`
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

func (r *repo) getSubjectsByObjPred(ctx context.Context, d sqldb.Executor, objns string, objkey string, objpred string, limit, offset int) (_ []Subject, retErr error) {
	res := make([]Subject, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT sub_ns, sub_key, sub_pred FROM "+r.table.TableName+" WHERE obj_ns = $3 AND obj_key = $4 AND obj_pred = $5 ORDER BY sub_ns ASC, sub_key ASC, sub_pred ASC LIMIT $1 OFFSET $2;", limit, offset, objns, objkey, objpred)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("Failed to close db rows: %w", err))
		}
	}()
	for rows.Next() {
		var m Subject
		if err := rows.Scan(&m.SubNS, &m.SubKey, &m.SubPred); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (r *repo) Read(ctx context.Context, ns, key string, pred string, limit, offset int) ([]Subject, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.getSubjectsByObjPred(ctx, d, ns, key, pred, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get acl tuples")
	}
	return m, nil
}

func (r *repo) Insert(ctx context.Context, m []*Model) error {
	if len(m) == 0 {
		return nil
	}

	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.InsertBulk(ctx, d, m, true); err != nil {
		return kerrors.WithMsg(err, "Failed to insert acl tuples")
	}
	return nil
}

func (r *repo) delSubTuples(ctx context.Context, d sqldb.Executor, sub Subject, objs []Object) error {
	paramCount := 3
	args := make([]interface{}, 0, paramCount+len(objs)*3)
	args = append(args, sub.SubNS, sub.SubKey, sub.SubPred)
	var placeholdersobjs string
	{
		placeholders := make([]string, 0, len(objs))
		for _, i := range objs {
			paramCount += 3
			placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d)", paramCount-2, paramCount-1, paramCount))
			args = append(args, i.ObjNS, i.ObjKey, i.ObjPred)
		}
		placeholdersobjs = strings.Join(placeholders, ", ")
	}
	_, err := d.ExecContext(ctx, "DELETE FROM "+r.table.TableName+" WHERE sub_ns = $1 AND sub_key = $2 AND sub_pred = $3 AND (obj_ns, obj_key, obj_pred) IN (VALUES "+placeholdersobjs+");", args...)
	return err
}

func (r *repo) DeleteForSub(ctx context.Context, sub Subject, objs []Object) error {
	if len(objs) == 0 {
		return nil
	}

	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.delSubTuples(ctx, d, sub, objs); err != nil {
		return kerrors.WithMsg(err, "Failed deleting acl tuples for subject")
	}
	return nil
}

func (r *repo) delObjTuples(ctx context.Context, d sqldb.Executor, obj Object, subs []Subject) error {
	paramCount := 3
	args := make([]interface{}, 0, paramCount+len(subs)*3)
	args = append(args, obj.ObjNS, obj.ObjKey, obj.ObjPred)
	var placeholderssubs string
	{
		placeholders := make([]string, 0, len(subs))
		for _, i := range subs {
			paramCount += 3
			placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d)", paramCount-2, paramCount-1, paramCount))
			args = append(args, i.SubNS, i.SubKey, i.SubPred)
		}
		placeholderssubs = strings.Join(placeholders, ", ")
	}
	_, err := d.ExecContext(ctx, "DELETE FROM "+r.table.TableName+" WHERE obj_ns = $1 AND obj_key = $2 AND obj_pred = $3 AND (sub_ns, sub_key, sub_pred) IN (VALUES "+placeholderssubs+");", args...)
	return err
}

func (r *repo) DeleteForObj(ctx context.Context, obj Object, subs []Subject) error {
	if len(subs) == 0 {
		return nil
	}

	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.delObjTuples(ctx, d, obj, subs); err != nil {
		return kerrors.WithMsg(err, "Failed deleting acl tuples for object")
	}
	return nil
}

func (r *repo) Check(ctx context.Context, objns, objkey string, pred string, subns, subkey string) (bool, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return false, err
	}
	if _, err := r.table.GetSubjectEqObjNSEqObjKeyEqObjPredEqSubNSEqSubKeyEqSubPred(ctx, d, objns, objkey, pred, subns, subkey, ""); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return false, nil
		}
		return false, kerrors.WithMsg(err, "Failed to check acl tuple")
	}
	return true, nil
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
