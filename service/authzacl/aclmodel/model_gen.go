// Code generated by go generate forge model v0.5.2; DO NOT EDIT.

package aclmodel

import (
	"context"
	"fmt"
	"strings"

	"xorkevin.dev/forge/model/sqldb"
)

type (
	aclModelTable struct {
		TableName string
	}
)

func (t *aclModelTable) Setup(ctx context.Context, d sqldb.Executor) error {
	_, err := d.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS "+t.TableName+" (obj_ns VARCHAR(255), obj_key VARCHAR(255), obj_pred VARCHAR(255), sub_ns VARCHAR(255), sub_key VARCHAR(255), sub_pred VARCHAR(255), PRIMARY KEY (obj_ns, obj_key, obj_pred, sub_ns, sub_key, sub_pred));")
	if err != nil {
		return err
	}
	_, err = d.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS "+t.TableName+"_sub_obj_index ON "+t.TableName+" (sub_ns, sub_key, sub_pred, obj_ns, obj_pred, obj_key);")
	if err != nil {
		return err
	}
	return nil
}

func (t *aclModelTable) Insert(ctx context.Context, d sqldb.Executor, m *Model) error {
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (obj_ns, obj_key, obj_pred, sub_ns, sub_key, sub_pred) VALUES ($1, $2, $3, $4, $5, $6);", m.ObjNS, m.ObjKey, m.ObjPred, m.SubNS, m.SubKey, m.SubPred)
	if err != nil {
		return err
	}
	return nil
}

func (t *aclModelTable) InsertBulk(ctx context.Context, d sqldb.Executor, models []*Model, allowConflict bool) error {
	conflictSQL := ""
	if allowConflict {
		conflictSQL = " ON CONFLICT DO NOTHING"
	}
	placeholders := make([]string, 0, len(models))
	args := make([]interface{}, 0, len(models)*6)
	for c, m := range models {
		n := c * 6
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d)", n+1, n+2, n+3, n+4, n+5, n+6))
		args = append(args, m.ObjNS, m.ObjKey, m.ObjPred, m.SubNS, m.SubKey, m.SubPred)
	}
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (obj_ns, obj_key, obj_pred, sub_ns, sub_key, sub_pred) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
	if err != nil {
		return err
	}
	return nil
}
