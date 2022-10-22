// Code generated by go generate forge model v0.4.0; DO NOT EDIT.

package model

import (
	"context"
	"fmt"
	"strings"

	"xorkevin.dev/governor/service/db"
)

type (
	resetModelTable struct {
		TableName string
	}
)

func (t *resetModelTable) Setup(ctx context.Context, d db.SQLExecutor) error {
	_, err := d.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS "+t.TableName+" (userid VARCHAR(31), kind VARCHAR(255), PRIMARY KEY (userid, kind), code_hash VARCHAR(255) NOT NULL, code_time BIGINT NOT NULL, params VARCHAR(4096));")
	if err != nil {
		return err
	}
	_, err = d.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS "+t.TableName+"_code_time_index ON "+t.TableName+" (code_time);")
	if err != nil {
		return err
	}
	return nil
}

func (t *resetModelTable) Insert(ctx context.Context, d db.SQLExecutor, m *Model) error {
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (userid, kind, code_hash, code_time, params) VALUES ($1, $2, $3, $4, $5);", m.Userid, m.Kind, m.CodeHash, m.CodeTime, m.Params)
	if err != nil {
		return err
	}
	return nil
}

func (t *resetModelTable) InsertBulk(ctx context.Context, d db.SQLExecutor, models []*Model, allowConflict bool) error {
	conflictSQL := ""
	if allowConflict {
		conflictSQL = " ON CONFLICT DO NOTHING"
	}
	placeholders := make([]string, 0, len(models))
	args := make([]interface{}, 0, len(models)*5)
	for c, m := range models {
		n := c * 5
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d)", n+1, n+2, n+3, n+4, n+5))
		args = append(args, m.Userid, m.Kind, m.CodeHash, m.CodeTime, m.Params)
	}
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (userid, kind, code_hash, code_time, params) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
	if err != nil {
		return err
	}
	return nil
}

func (t *resetModelTable) DelEqUserid(ctx context.Context, d db.SQLExecutor, userid string) error {
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE userid = $1;", userid)
	return err
}

func (t *resetModelTable) GetModelEqUseridEqKind(ctx context.Context, d db.SQLExecutor, userid string, kind string) (*Model, error) {
	m := &Model{}
	if err := d.QueryRowContext(ctx, "SELECT userid, kind, code_hash, code_time, params FROM "+t.TableName+" WHERE userid = $1 AND kind = $2;", userid, kind).Scan(&m.Userid, &m.Kind, &m.CodeHash, &m.CodeTime, &m.Params); err != nil {
		return nil, err
	}
	return m, nil
}

func (t *resetModelTable) UpdModelEqUseridEqKind(ctx context.Context, d db.SQLExecutor, m *Model, userid string, kind string) error {
	_, err := d.ExecContext(ctx, "UPDATE "+t.TableName+" SET (userid, kind, code_hash, code_time, params) = ROW($1, $2, $3, $4, $5) WHERE userid = $6 AND kind = $7;", m.Userid, m.Kind, m.CodeHash, m.CodeTime, m.Params, userid, kind)
	if err != nil {
		return err
	}
	return nil
}

func (t *resetModelTable) DelEqUseridEqKind(ctx context.Context, d db.SQLExecutor, userid string, kind string) error {
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE userid = $1 AND kind = $2;", userid, kind)
	return err
}

func (t *resetModelTable) DelLtCodeTime(ctx context.Context, d db.SQLExecutor, codetime int64) error {
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE code_time < $1;", codetime)
	return err
}
