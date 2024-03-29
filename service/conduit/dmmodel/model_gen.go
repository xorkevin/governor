// Code generated by go generate forge model v0.5.2; DO NOT EDIT.

package dmmodel

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"xorkevin.dev/forge/model/sqldb"
)

type (
	dmModelTable struct {
		TableName string
	}
)

func (t *dmModelTable) Setup(ctx context.Context, d sqldb.Executor) error {
	_, err := d.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS "+t.TableName+" (userid_1 VARCHAR(31), userid_2 VARCHAR(31), chatid VARCHAR(31) NOT NULL UNIQUE, name VARCHAR(255) NOT NULL, theme VARCHAR(4095) NOT NULL, last_updated BIGINT NOT NULL, creation_time BIGINT NOT NULL, PRIMARY KEY (userid_1, userid_2));")
	if err != nil {
		return err
	}
	_, err = d.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS "+t.TableName+"_userid1_last_updated_index ON "+t.TableName+" (userid_1, last_updated);")
	if err != nil {
		return err
	}
	_, err = d.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS "+t.TableName+"_userid2_last_updated_index ON "+t.TableName+" (userid_2, last_updated);")
	if err != nil {
		return err
	}
	return nil
}

func (t *dmModelTable) Insert(ctx context.Context, d sqldb.Executor, m *Model) error {
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (userid_1, userid_2, chatid, name, theme, last_updated, creation_time) VALUES ($1, $2, $3, $4, $5, $6, $7);", m.Userid1, m.Userid2, m.Chatid, m.Name, m.Theme, m.LastUpdated, m.CreationTime)
	if err != nil {
		return err
	}
	return nil
}

func (t *dmModelTable) InsertBulk(ctx context.Context, d sqldb.Executor, models []*Model, allowConflict bool) error {
	conflictSQL := ""
	if allowConflict {
		conflictSQL = " ON CONFLICT DO NOTHING"
	}
	placeholders := make([]string, 0, len(models))
	args := make([]interface{}, 0, len(models)*7)
	for c, m := range models {
		n := c * 7
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d)", n+1, n+2, n+3, n+4, n+5, n+6, n+7))
		args = append(args, m.Userid1, m.Userid2, m.Chatid, m.Name, m.Theme, m.LastUpdated, m.CreationTime)
	}
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (userid_1, userid_2, chatid, name, theme, last_updated, creation_time) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
	if err != nil {
		return err
	}
	return nil
}

func (t *dmModelTable) GetModelByUser1User2(ctx context.Context, d sqldb.Executor, userid1 string, userid2 string) (*Model, error) {
	m := &Model{}
	if err := d.QueryRowContext(ctx, "SELECT userid_1, userid_2, chatid, name, theme, last_updated, creation_time FROM "+t.TableName+" WHERE userid_1 = $1 AND userid_2 = $2;", userid1, userid2).Scan(&m.Userid1, &m.Userid2, &m.Chatid, &m.Name, &m.Theme, &m.LastUpdated, &m.CreationTime); err != nil {
		return nil, err
	}
	return m, nil
}

func (t *dmModelTable) DelByUser1User2(ctx context.Context, d sqldb.Executor, userid1 string, userid2 string) error {
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE userid_1 = $1 AND userid_2 = $2;", userid1, userid2)
	return err
}

func (t *dmModelTable) GetModelByChat(ctx context.Context, d sqldb.Executor, chatid string) (*Model, error) {
	m := &Model{}
	if err := d.QueryRowContext(ctx, "SELECT userid_1, userid_2, chatid, name, theme, last_updated, creation_time FROM "+t.TableName+" WHERE chatid = $1;", chatid).Scan(&m.Userid1, &m.Userid2, &m.Chatid, &m.Name, &m.Theme, &m.LastUpdated, &m.CreationTime); err != nil {
		return nil, err
	}
	return m, nil
}

func (t *dmModelTable) GetModelByChats(ctx context.Context, d sqldb.Executor, chatids []string, limit, offset int) (_ []Model, retErr error) {
	paramCount := 2
	args := make([]interface{}, 0, paramCount+len(chatids))
	args = append(args, limit, offset)
	var placeholderschatids string
	{
		placeholders := make([]string, 0, len(chatids))
		for _, i := range chatids {
			paramCount++
			placeholders = append(placeholders, fmt.Sprintf("($%d)", paramCount))
			args = append(args, i)
		}
		placeholderschatids = strings.Join(placeholders, ", ")
	}
	res := make([]Model, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT userid_1, userid_2, chatid, name, theme, last_updated, creation_time FROM "+t.TableName+" WHERE chatid IN (VALUES "+placeholderschatids+") LIMIT $1 OFFSET $2;", args...)
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
		if err := rows.Scan(&m.Userid1, &m.Userid2, &m.Chatid, &m.Name, &m.Theme, &m.LastUpdated, &m.CreationTime); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (t *dmModelTable) UpddmPropsByUser1User2(ctx context.Context, d sqldb.Executor, m *dmProps, userid1 string, userid2 string) error {
	_, err := d.ExecContext(ctx, "UPDATE "+t.TableName+" SET (name, theme) = ($1, $2) WHERE userid_1 = $3 AND userid_2 = $4;", m.Name, m.Theme, userid1, userid2)
	if err != nil {
		return err
	}
	return nil
}

func (t *dmModelTable) UpddmLastUpdatedByUser1User2(ctx context.Context, d sqldb.Executor, m *dmLastUpdated, userid1 string, userid2 string) error {
	_, err := d.ExecContext(ctx, "UPDATE "+t.TableName+" SET last_updated = $1 WHERE userid_1 = $2 AND userid_2 = $3;", m.LastUpdated, userid1, userid2)
	if err != nil {
		return err
	}
	return nil
}
