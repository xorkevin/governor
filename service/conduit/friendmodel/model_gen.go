// Code generated by go generate forge model v0.4.3; DO NOT EDIT.

package friendmodel

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"xorkevin.dev/forge/model/sqldb"
)

type (
	friendModelTable struct {
		TableName string
	}
)

func (t *friendModelTable) Setup(ctx context.Context, d sqldb.Executor) error {
	_, err := d.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS "+t.TableName+" (userid_1 VARCHAR(31), userid_2 VARCHAR(31), PRIMARY KEY (userid_1, userid_2), username VARCHAR(255) NOT NULL);")
	if err != nil {
		return err
	}
	_, err = d.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS "+t.TableName+"_userid_2_index ON "+t.TableName+" (userid_2);")
	if err != nil {
		return err
	}
	_, err = d.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS "+t.TableName+"_userid_1__username_index ON "+t.TableName+" (userid_1, username);")
	if err != nil {
		return err
	}
	return nil
}

func (t *friendModelTable) Insert(ctx context.Context, d sqldb.Executor, m *Model) error {
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (userid_1, userid_2, username) VALUES ($1, $2, $3);", m.Userid1, m.Userid2, m.Username)
	if err != nil {
		return err
	}
	return nil
}

func (t *friendModelTable) InsertBulk(ctx context.Context, d sqldb.Executor, models []*Model, allowConflict bool) error {
	conflictSQL := ""
	if allowConflict {
		conflictSQL = " ON CONFLICT DO NOTHING"
	}
	placeholders := make([]string, 0, len(models))
	args := make([]interface{}, 0, len(models)*3)
	for c, m := range models {
		n := c * 3
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d)", n+1, n+2, n+3))
		args = append(args, m.Userid1, m.Userid2, m.Username)
	}
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (userid_1, userid_2, username) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
	if err != nil {
		return err
	}
	return nil
}

func (t *friendModelTable) GetModelEqUserid1EqUserid2(ctx context.Context, d sqldb.Executor, userid1 string, userid2 string) (*Model, error) {
	m := &Model{}
	if err := d.QueryRowContext(ctx, "SELECT userid_1, userid_2, username FROM "+t.TableName+" WHERE userid_1 = $1 AND userid_2 = $2;", userid1, userid2).Scan(&m.Userid1, &m.Userid2, &m.Username); err != nil {
		return nil, err
	}
	return m, nil
}

func (t *friendModelTable) GetModelEqUserid1HasUserid2OrdUserid2(ctx context.Context, d sqldb.Executor, userid1 string, userid2s []string, orderasc bool, limit, offset int) (_ []Model, retErr error) {
	paramCount := 3
	args := make([]interface{}, 0, paramCount+len(userid2s))
	args = append(args, limit, offset, userid1)
	var placeholdersuserid2s string
	{
		placeholders := make([]string, 0, len(userid2s))
		for _, i := range userid2s {
			paramCount++
			placeholders = append(placeholders, fmt.Sprintf("($%d)", paramCount))
			args = append(args, i)
		}
		placeholdersuserid2s = strings.Join(placeholders, ", ")
	}
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]Model, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT userid_1, userid_2, username FROM "+t.TableName+" WHERE userid_1 = $3 AND userid_2 IN (VALUES "+placeholdersuserid2s+") ORDER BY userid_2 "+order+" LIMIT $1 OFFSET $2;", args...)
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
		if err := rows.Scan(&m.Userid1, &m.Userid2, &m.Username); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (t *friendModelTable) GetModelEqUserid1OrdUsername(ctx context.Context, d sqldb.Executor, userid1 string, orderasc bool, limit, offset int) (_ []Model, retErr error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]Model, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT userid_1, userid_2, username FROM "+t.TableName+" WHERE userid_1 = $3 ORDER BY username "+order+" LIMIT $1 OFFSET $2;", limit, offset, userid1)
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
		if err := rows.Scan(&m.Userid1, &m.Userid2, &m.Username); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (t *friendModelTable) GetModelEqUserid1LikeUsernameOrdUsername(ctx context.Context, d sqldb.Executor, userid1 string, usernamePrefix string, orderasc bool, limit, offset int) (_ []Model, retErr error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]Model, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT userid_1, userid_2, username FROM "+t.TableName+" WHERE userid_1 = $3 AND username LIKE $4 ORDER BY username "+order+" LIMIT $1 OFFSET $2;", limit, offset, userid1, usernamePrefix)
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
		if err := rows.Scan(&m.Userid1, &m.Userid2, &m.Username); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (t *friendModelTable) UpdfriendUsernameEqUserid2(ctx context.Context, d sqldb.Executor, m *friendUsername, userid2 string) error {
	_, err := d.ExecContext(ctx, "UPDATE "+t.TableName+" SET (username) = ROW($1) WHERE userid_2 = $2;", m.Username, userid2)
	if err != nil {
		return err
	}
	return nil
}
