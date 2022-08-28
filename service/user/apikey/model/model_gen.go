// Code generated by go generate forge model v0.3; DO NOT EDIT.

package model

import (
	"context"
	"fmt"
	"strings"

	"xorkevin.dev/governor/service/db"
)

type (
	apikeyModelTable struct {
		TableName string
	}
)

func (t *apikeyModelTable) Setup(ctx context.Context, d db.SQLExecutor) error {
	_, err := d.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS "+t.TableName+" (keyid VARCHAR(63) PRIMARY KEY, userid VARCHAR(31) NOT NULL, scope VARCHAR(4095) NOT NULL, keyhash VARCHAR(127) NOT NULL, name VARCHAR(255) NOT NULL, description VARCHAR(255), time BIGINT NOT NULL);")
	if err != nil {
		return err
	}
	_, err = d.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS "+t.TableName+"_userid_index ON "+t.TableName+" (userid);")
	if err != nil {
		return err
	}
	_, err = d.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS "+t.TableName+"_userid__time_index ON "+t.TableName+" (userid, time);")
	if err != nil {
		return err
	}
	return nil
}

func (t *apikeyModelTable) Insert(ctx context.Context, d db.SQLExecutor, m *Model) error {
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (keyid, userid, scope, keyhash, name, description, time) VALUES ($1, $2, $3, $4, $5, $6, $7);", m.Keyid, m.Userid, m.Scope, m.KeyHash, m.Name, m.Desc, m.Time)
	if err != nil {
		return err
	}
	return nil
}

func (t *apikeyModelTable) InsertBulk(ctx context.Context, d db.SQLExecutor, models []*Model, allowConflict bool) error {
	conflictSQL := ""
	if allowConflict {
		conflictSQL = " ON CONFLICT DO NOTHING"
	}
	placeholders := make([]string, 0, len(models))
	args := make([]interface{}, 0, len(models)*7)
	for c, m := range models {
		n := c * 7
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d)", n+1, n+2, n+3, n+4, n+5, n+6, n+7))
		args = append(args, m.Keyid, m.Userid, m.Scope, m.KeyHash, m.Name, m.Desc, m.Time)
	}
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (keyid, userid, scope, keyhash, name, description, time) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
	if err != nil {
		return err
	}
	return nil
}

func (t *apikeyModelTable) GetModelEqKeyid(ctx context.Context, d db.SQLExecutor, keyid string) (*Model, error) {
	m := &Model{}
	if err := d.QueryRowContext(ctx, "SELECT keyid, userid, scope, keyhash, name, description, time FROM "+t.TableName+" WHERE keyid = $1;", keyid).Scan(&m.Keyid, &m.Userid, &m.Scope, &m.KeyHash, &m.Name, &m.Desc, &m.Time); err != nil {
		return nil, err
	}
	return m, nil
}

func (t *apikeyModelTable) UpdModelEqKeyid(ctx context.Context, d db.SQLExecutor, m *Model, keyid string) error {
	_, err := d.ExecContext(ctx, "UPDATE "+t.TableName+" SET (keyid, userid, scope, keyhash, name, description, time) = ROW($1, $2, $3, $4, $5, $6, $7) WHERE keyid = $8;", m.Keyid, m.Userid, m.Scope, m.KeyHash, m.Name, m.Desc, m.Time, keyid)
	if err != nil {
		return err
	}
	return nil
}

func (t *apikeyModelTable) DelEqKeyid(ctx context.Context, d db.SQLExecutor, keyid string) error {
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE keyid = $1;", keyid)
	return err
}

func (t *apikeyModelTable) DelHasKeyid(ctx context.Context, d db.SQLExecutor, keyid []string) error {
	paramCount := 0
	args := make([]interface{}, 0, paramCount+len(keyid))
	var placeholderskeyid string
	{
		placeholders := make([]string, 0, len(keyid))
		for _, i := range keyid {
			paramCount++
			placeholders = append(placeholders, fmt.Sprintf("($%d)", paramCount))
			args = append(args, i)
		}
		placeholderskeyid = strings.Join(placeholders, ", ")
	}
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE keyid IN (VALUES "+placeholderskeyid+");", args...)
	return err
}

func (t *apikeyModelTable) GetModelEqUseridOrdTime(ctx context.Context, d db.SQLExecutor, userid string, orderasc bool, limit, offset int) ([]Model, error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]Model, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT keyid, userid, scope, keyhash, name, description, time FROM "+t.TableName+" WHERE userid = $3 ORDER BY time "+order+" LIMIT $1 OFFSET $2;", limit, offset, userid)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		m := Model{}
		if err := rows.Scan(&m.Keyid, &m.Userid, &m.Scope, &m.KeyHash, &m.Name, &m.Desc, &m.Time); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (t *apikeyModelTable) UpdapikeyHashEqKeyid(ctx context.Context, d db.SQLExecutor, m *apikeyHash, keyid string) error {
	_, err := d.ExecContext(ctx, "UPDATE "+t.TableName+" SET (keyhash, time) = ROW($1, $2) WHERE keyid = $3;", m.KeyHash, m.Time, keyid)
	if err != nil {
		return err
	}
	return nil
}

func (t *apikeyModelTable) UpdapikeyPropsEqKeyid(ctx context.Context, d db.SQLExecutor, m *apikeyProps, keyid string) error {
	_, err := d.ExecContext(ctx, "UPDATE "+t.TableName+" SET (scope, name, description) = ROW($1, $2, $3) WHERE keyid = $4;", m.Scope, m.Name, m.Desc, keyid)
	if err != nil {
		return err
	}
	return nil
}
