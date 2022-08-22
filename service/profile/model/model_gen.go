// Code generated by go generate forge model v0.3; DO NOT EDIT.

package model

import (
	"context"
	"fmt"
	"strings"

	"xorkevin.dev/governor/service/db"
)

type (
	profileModelTable struct {
		TableName string
	}
)

func (t *profileModelTable) Setup(ctx context.Context, d db.SQLExecutor) error {
	_, err := d.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS "+t.TableName+" (userid VARCHAR(31) PRIMARY KEY, contact_email VARCHAR(255), bio VARCHAR(4095), profile_image_url VARCHAR(4095));")
	if err != nil {
		return err
	}
	return nil
}

func (t *profileModelTable) Insert(ctx context.Context, d db.SQLExecutor, m *Model) error {
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (userid, contact_email, bio, profile_image_url) VALUES ($1, $2, $3, $4);", m.Userid, m.Email, m.Bio, m.Image)
	if err != nil {
		return err
	}
	return nil
}

func (t *profileModelTable) InsertBulk(ctx context.Context, d db.SQLExecutor, models []*Model, allowConflict bool) error {
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

func (t *profileModelTable) GetModelEqUserid(ctx context.Context, d db.SQLExecutor, userid string) (*Model, error) {
	m := &Model{}
	if err := d.QueryRowContext(ctx, "SELECT userid, contact_email, bio, profile_image_url FROM "+t.TableName+" WHERE userid = $1;", userid).Scan(&m.Userid, &m.Email, &m.Bio, &m.Image); err != nil {
		return nil, err
	}
	return m, nil
}

func (t *profileModelTable) GetModelHasUseridOrdUserid(ctx context.Context, d db.SQLExecutor, userid []string, orderasc bool, limit, offset int) ([]Model, error) {
	paramCount := 2
	args := make([]interface{}, 0, paramCount+len(userid))
	args = append(args, limit, offset)
	var placeholdersuserid string
	{
		placeholders := make([]string, 0, len(userid))
		for _, i := range userid {
			paramCount++
			placeholders = append(placeholders, fmt.Sprintf("($%d)", paramCount))
			args = append(args, i)
		}
		placeholdersuserid = strings.Join(placeholders, ", ")
	}
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]Model, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT userid, contact_email, bio, profile_image_url FROM "+t.TableName+" WHERE userid IN (VALUES "+placeholdersuserid+") ORDER BY userid "+order+" LIMIT $1 OFFSET $2;", args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		m := Model{}
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

func (t *profileModelTable) UpdModelEqUserid(ctx context.Context, d db.SQLExecutor, m *Model, userid string) error {
	_, err := d.ExecContext(ctx, "UPDATE "+t.TableName+" SET (userid, contact_email, bio, profile_image_url) = ROW($1, $2, $3, $4) WHERE userid = $5;", m.Userid, m.Email, m.Bio, m.Image, userid)
	if err != nil {
		return err
	}
	return nil
}

func (t *profileModelTable) DelEqUserid(ctx context.Context, d db.SQLExecutor, userid string) error {
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE userid = $1;", userid)
	return err
}
