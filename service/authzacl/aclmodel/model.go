package aclmodel

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"xorkevin.dev/forge/model/sqldb"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/kerrors"
)

//go:generate forge model

type (
	// Repo is an acl repository
	Repo interface {
		Read(ctx context.Context, obj Object, limit int, afterPred string, after Subject) ([]Subject, error)
		ReadBySub(ctx context.Context, sub Subject, limit int, after ObjectRel) ([]ObjectRel, error)
		Insert(ctx context.Context, m []*Model) error
		Delete(ctx context.Context, m []Model) error
		Check(ctx context.Context, obj Object, pred string, sub Subject) (bool, error)
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
		ObjKey  string `model:"obj_key,VARCHAR(255)"`
		ObjPred string `model:"obj_pred,VARCHAR(255)"`
		SubNS   string `model:"sub_ns,VARCHAR(255)"`
		SubKey  string `model:"sub_key,VARCHAR(255)"`
		SubPred string `model:"sub_pred,VARCHAR(255)"`
	}

	Subject struct {
		SubNS   string
		SubKey  string
		SubPred string
	}

	Object struct {
		ObjNS  string
		ObjKey string
	}

	ObjectRel struct {
		ObjNS   string
		ObjKey  string
		ObjPred string
	}
)

// New creates a new acl repository
func New(database db.Database, table string) Repo {
	return &repo{
		table: &aclModelTable{
			TableName: table,
		},
		db: database,
	}
}

func (r *repo) getSubjectsByObjPred(ctx context.Context, d sqldb.Executor, obj Object, limit int, pred string, sub Subject) (_ []Subject, retErr error) {
	res := make([]Subject, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT sub_ns, sub_key, sub_pred FROM "+r.table.TableName+" WHERE obj_ns = $2 AND obj_key = $3 AND (obj_pred > $4 OR (obj_pred = $4 AND sub_ns > $5) OR (obj_pred = $4 AND sub_ns = $5 AND sub_key > $6) OR (obj_pred = $4 AND sub_ns = $5 AND sub_key = $6 AND sub_pred > $7)) ORDER BY obj_pred ASC, sub_ns ASC, sub_key ASC, sub_pred ASC LIMIT $1;", limit, obj.ObjNS, obj.ObjKey, pred, sub.SubNS, sub.SubKey, sub.SubPred)
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

func (r *repo) Read(ctx context.Context, obj Object, limit int, afterPred string, after Subject) ([]Subject, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.getSubjectsByObjPred(ctx, d, obj, limit, afterPred, after)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get acl tuples")
	}
	return m, nil
}

func (r *repo) getObjectsBySubPred(ctx context.Context, d sqldb.Executor, sub Subject, limit int, obj ObjectRel) (_ []ObjectRel, retErr error) {
	res := make([]ObjectRel, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT obj_ns, obj_key, obj_pred FROM "+r.table.TableName+" WHERE sub_ns = $2 AND sub_key = $3 AND sub_pred = $4 AND (obj_ns > $5 OR (obj_ns = $5 AND obj_key > $6) OR (obj_ns = $5 AND obj_key = $6 AND obj_pred > $7)) ORDER BY obj_ns ASC, obj_key ASC, obj_pred ASC LIMIT $1;", limit, sub.SubNS, sub.SubKey, sub.SubPred, obj.ObjNS, obj.ObjKey, obj.ObjPred)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("Failed to close db rows: %w", err))
		}
	}()
	for rows.Next() {
		var m ObjectRel
		if err := rows.Scan(&m.ObjNS, &m.ObjKey, &m.ObjPred); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (r *repo) ReadBySub(ctx context.Context, sub Subject, limit int, after ObjectRel) ([]ObjectRel, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.getObjectsBySubPred(ctx, d, sub, limit, after)
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

func (r *repo) delRelTuples(ctx context.Context, d sqldb.Executor, m []Model) error {
	paramCount := 0
	args := make([]interface{}, 0, paramCount+len(m)*6)
	var placeholdersobjs string
	{
		placeholders := make([]string, 0, len(m))
		for _, i := range m {
			paramCount += 6
			placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d)", paramCount-5, paramCount-4, paramCount-3, paramCount-2, paramCount-1, paramCount))
			args = append(args, i.ObjNS, i.ObjKey, i.ObjPred, i.SubNS, i.SubKey, i.SubPred)
		}
		placeholdersobjs = strings.Join(placeholders, ", ")
	}
	_, err := d.ExecContext(ctx, "DELETE FROM "+r.table.TableName+" WHERE (obj_ns, obj_key, obj_pred, sub_ns, sub_key, sub_pred) IN (VALUES "+placeholdersobjs+");", args...)
	return err
}

func (r *repo) Delete(ctx context.Context, m []Model) error {
	if len(m) == 0 {
		return nil
	}

	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.delRelTuples(ctx, d, m); err != nil {
		return kerrors.WithMsg(err, "Failed deleting acl tuples")
	}
	return nil
}

func (r *repo) checkRelation(ctx context.Context, d sqldb.Executor, obj Object, pred string, sub Subject) (bool, error) {
	var exists bool
	if err := d.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM "+r.table.TableName+" WHERE obj_ns = $1 AND obj_key = $2 AND obj_pred = $3 AND sub_ns = $4 AND sub_key = $5 AND sub_pred = $6);", obj.ObjNS, obj.ObjKey, pred, sub.SubNS, sub.SubKey, sub.SubPred).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (r *repo) Check(ctx context.Context, obj Object, pred string, sub Subject) (bool, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return false, err
	}
	if _, err := r.checkRelation(ctx, d, obj, pred, sub); err != nil {
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
