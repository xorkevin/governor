// Code generated by go generate forge model v0.5.2; DO NOT EDIT.

package profilemodel

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"xorkevin.dev/forge/model/sqldb"
)

type (
	profileModelTable struct {
		TableName string
	}
)

func (t *profileModelTable) Setup(ctx context.Context, d sqldb.Executor) error {
	_, err := d.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS "+t.TableName+" (userid VARCHAR(31) PRIMARY KEY, contact_email VARCHAR(255), bio VARCHAR(4095), profile_image_url VARCHAR(4095));")
	if err != nil {
		return err
	}
	return nil
}

func (t *profileModelTable) Insert(ctx context.Context, d sqldb.Executor, m *Model) error {
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (userid, contact_email, bio, profile_image_url) VALUES ($1, $2, $3, $4);", m.Userid, m.Email, m.Bio, m.Image)
	if err != nil {
		return err
	}
	return nil
}

func (t *profileModelTable) InsertBulk(ctx context.Context, d sqldb.Executor, models []*Model, allowConflict bool) error {
	conflictSQL := ""
	if allowConflict {
		conflictSQL = " ON CONFLICT DO NOTHING"
	}
	placeholders := make([]string, 0, len(models))
	args := make([]interface{}, 0, len(models)*4)
	for c, m := range models {
		n := c * 4
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d)", n+1, n+2, n+3, n+4))
		args = append(args, m.Userid, m.Email, m.Bio, m.Image)
	}
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (userid, contact_email, bio, profile_image_url) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
	if err != nil {
		return err
	}
	return nil
}

func (t *profileModelTable) GetModelByID(ctx context.Context, d sqldb.Executor, userid string) (*Model, error) {
	m := &Model{}
	if err := d.QueryRowContext(ctx, "SELECT userid, contact_email, bio, profile_image_url FROM "+t.TableName+" WHERE userid = $1;", userid).Scan(&m.Userid, &m.Email, &m.Bio, &m.Image); err != nil {
		return nil, err
	}
	return m, nil
}

func (t *profileModelTable) GetModelByIDs(ctx context.Context, d sqldb.Executor, userids []string, limit, offset int) (_ []Model, retErr error) {
	paramCount := 2
	args := make([]interface{}, 0, paramCount+len(userids))
	args = append(args, limit, offset)
	var placeholdersuserids string
	{
		placeholders := make([]string, 0, len(userids))
		for _, i := range userids {
			paramCount++
			placeholders = append(placeholders, fmt.Sprintf("($%d)", paramCount))
			args = append(args, i)
		}
		placeholdersuserids = strings.Join(placeholders, ", ")
	}
	res := make([]Model, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT userid, contact_email, bio, profile_image_url FROM "+t.TableName+" WHERE userid IN (VALUES "+placeholdersuserids+") LIMIT $1 OFFSET $2;", args...)
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
		if err := rows.Scan(&m.Userid, &m.Email, &m.Bio, &m.Image); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (t *profileModelTable) UpdModelByID(ctx context.Context, d sqldb.Executor, m *Model, userid string) error {
	_, err := d.ExecContext(ctx, "UPDATE "+t.TableName+" SET (userid, contact_email, bio, profile_image_url) = ($1, $2, $3, $4) WHERE userid = $5;", m.Userid, m.Email, m.Bio, m.Image, userid)
	if err != nil {
		return err
	}
	return nil
}

func (t *profileModelTable) DelByID(ctx context.Context, d sqldb.Executor, userid string) error {
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE userid = $1;", userid)
	return err
}
