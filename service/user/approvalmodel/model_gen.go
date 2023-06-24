// Code generated by go generate forge model v0.4.4; DO NOT EDIT.

package approvalmodel

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"xorkevin.dev/forge/model/sqldb"
)

type (
	approvalModelTable struct {
		TableName string
	}
)

func (t *approvalModelTable) Setup(ctx context.Context, d sqldb.Executor) error {
	_, err := d.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS "+t.TableName+" (userid VARCHAR(31) PRIMARY KEY, username VARCHAR(255) NOT NULL, pass_hash VARCHAR(255) NOT NULL, email VARCHAR(255) NOT NULL, first_name VARCHAR(255) NOT NULL, last_name VARCHAR(255) NOT NULL, creation_time BIGINT NOT NULL, approved BOOL NOT NULL, code_hash VARCHAR(255) NOT NULL, code_time BIGINT NOT NULL);")
	if err != nil {
		return err
	}
	_, err = d.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS "+t.TableName+"_creation_time_index ON "+t.TableName+" (creation_time);")
	if err != nil {
		return err
	}
	return nil
}

func (t *approvalModelTable) Insert(ctx context.Context, d sqldb.Executor, m *Model) error {
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (userid, username, pass_hash, email, first_name, last_name, creation_time, approved, code_hash, code_time) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);", m.Userid, m.Username, m.PassHash, m.Email, m.FirstName, m.LastName, m.CreationTime, m.Approved, m.CodeHash, m.CodeTime)
	if err != nil {
		return err
	}
	return nil
}

func (t *approvalModelTable) InsertBulk(ctx context.Context, d sqldb.Executor, models []*Model, allowConflict bool) error {
	conflictSQL := ""
	if allowConflict {
		conflictSQL = " ON CONFLICT DO NOTHING"
	}
	placeholders := make([]string, 0, len(models))
	args := make([]interface{}, 0, len(models)*10)
	for c, m := range models {
		n := c * 10
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)", n+1, n+2, n+3, n+4, n+5, n+6, n+7, n+8, n+9, n+10))
		args = append(args, m.Userid, m.Username, m.PassHash, m.Email, m.FirstName, m.LastName, m.CreationTime, m.Approved, m.CodeHash, m.CodeTime)
	}
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (userid, username, pass_hash, email, first_name, last_name, creation_time, approved, code_hash, code_time) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
	if err != nil {
		return err
	}
	return nil
}

func (t *approvalModelTable) GetModelEqUserid(ctx context.Context, d sqldb.Executor, userid string) (*Model, error) {
	m := &Model{}
	if err := d.QueryRowContext(ctx, "SELECT userid, username, pass_hash, email, first_name, last_name, creation_time, approved, code_hash, code_time FROM "+t.TableName+" WHERE userid = $1;", userid).Scan(&m.Userid, &m.Username, &m.PassHash, &m.Email, &m.FirstName, &m.LastName, &m.CreationTime, &m.Approved, &m.CodeHash, &m.CodeTime); err != nil {
		return nil, err
	}
	return m, nil
}

func (t *approvalModelTable) UpdModelEqUserid(ctx context.Context, d sqldb.Executor, m *Model, userid string) error {
	_, err := d.ExecContext(ctx, "UPDATE "+t.TableName+" SET (userid, username, pass_hash, email, first_name, last_name, creation_time, approved, code_hash, code_time) = ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) WHERE userid = $11;", m.Userid, m.Username, m.PassHash, m.Email, m.FirstName, m.LastName, m.CreationTime, m.Approved, m.CodeHash, m.CodeTime, userid)
	if err != nil {
		return err
	}
	return nil
}

func (t *approvalModelTable) DelEqUserid(ctx context.Context, d sqldb.Executor, userid string) error {
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE userid = $1;", userid)
	return err
}

func (t *approvalModelTable) GetModelOrdCreationTime(ctx context.Context, d sqldb.Executor, orderasc bool, limit, offset int) (_ []Model, retErr error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]Model, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT userid, username, pass_hash, email, first_name, last_name, creation_time, approved, code_hash, code_time FROM "+t.TableName+" ORDER BY creation_time "+order+" LIMIT $1 OFFSET $2;", limit, offset)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("Failed to close db rows: %w", err))
		}
	}()
	for rows.Next() {
		var m Model
		if err := rows.Scan(&m.Userid, &m.Username, &m.PassHash, &m.Email, &m.FirstName, &m.LastName, &m.CreationTime, &m.Approved, &m.CodeHash, &m.CodeTime); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (t *approvalModelTable) DelLtCreationTime(ctx context.Context, d sqldb.Executor, creationtime int64) error {
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE creation_time < $1;", creationtime)
	return err
}
