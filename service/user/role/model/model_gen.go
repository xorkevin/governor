// Code generated by go generate forge model v0.3; DO NOT EDIT.

package model

import (
	"context"
	"fmt"
	"strings"

	"xorkevin.dev/governor/service/db"
)

type (
	roleModelTable struct {
		TableName string
	}
)

func (t *roleModelTable) Setup(ctx context.Context, d db.SQLExecutor) error {
	_, err := d.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS "+t.TableName+" (userid VARCHAR(31), role VARCHAR(255), PRIMARY KEY (userid, role));")
	if err != nil {
		return err
	}
	_, err = d.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS "+t.TableName+"_role__userid_index ON "+t.TableName+" (role, userid);")
	if err != nil {
		return err
	}
	return nil
}

func (t *roleModelTable) Insert(ctx context.Context, d db.SQLExecutor, m *Model) error {
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (userid, role) VALUES ($1, $2);", m.Userid, m.Role)
	if err != nil {
		return err
	}
	return nil
}

func (t *roleModelTable) InsertBulk(ctx context.Context, d db.SQLExecutor, models []*Model, allowConflict bool) error {
	conflictSQL := ""
	if allowConflict {
		conflictSQL = " ON CONFLICT DO NOTHING"
	}
	placeholders := make([]string, 0, len(models))
	args := make([]interface{}, 0, len(models)*2)
	for c, m := range models {
		n := c * 2
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d)", n+1, n+2))
		args = append(args, m.Userid, m.Role)
	}
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (userid, role) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
	if err != nil {
		return err
	}
	return nil
}

func (t *roleModelTable) GetModelEqRoleOrdUserid(ctx context.Context, d db.SQLExecutor, role string, orderasc bool, limit, offset int) ([]Model, error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]Model, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT userid, role FROM "+t.TableName+" WHERE role = $3 ORDER BY userid "+order+" LIMIT $1 OFFSET $2;", limit, offset, role)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		m := Model{}
		if err := rows.Scan(&m.Userid, &m.Role); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (t *roleModelTable) DelEqUserid(ctx context.Context, d db.SQLExecutor, userid string) error {
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE userid = $1;", userid)
	return err
}

func (t *roleModelTable) GetModelEqUseridEqRole(ctx context.Context, d db.SQLExecutor, userid string, role string) (*Model, error) {
	m := &Model{}
	if err := d.QueryRowContext(ctx, "SELECT userid, role FROM "+t.TableName+" WHERE userid = $1 AND role = $2;", userid, role).Scan(&m.Userid, &m.Role); err != nil {
		return nil, err
	}
	return m, nil
}

func (t *roleModelTable) GetModelEqUseridOrdRole(ctx context.Context, d db.SQLExecutor, userid string, orderasc bool, limit, offset int) ([]Model, error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]Model, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT userid, role FROM "+t.TableName+" WHERE userid = $3 ORDER BY role "+order+" LIMIT $1 OFFSET $2;", limit, offset, userid)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		m := Model{}
		if err := rows.Scan(&m.Userid, &m.Role); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (t *roleModelTable) GetModelEqUseridHasRoleOrdRole(ctx context.Context, d db.SQLExecutor, userid string, role []string, orderasc bool, limit, offset int) ([]Model, error) {
	paramCount := 3
	args := make([]interface{}, 0, paramCount+len(role))
	args = append(args, limit, offset, userid)
	var placeholdersrole string
	{
		placeholders := make([]string, 0, len(role))
		for _, i := range role {
			paramCount++
			placeholders = append(placeholders, fmt.Sprintf("($%d)", paramCount))
			args = append(args, i)
		}
		placeholdersrole = strings.Join(placeholders, ", ")
	}
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]Model, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT userid, role FROM "+t.TableName+" WHERE userid = $3 AND role IN (VALUES "+placeholdersrole+") ORDER BY role "+order+" LIMIT $1 OFFSET $2;", args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		m := Model{}
		if err := rows.Scan(&m.Userid, &m.Role); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (t *roleModelTable) GetModelEqUseridLikeRoleOrdRole(ctx context.Context, d db.SQLExecutor, userid string, role string, orderasc bool, limit, offset int) ([]Model, error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]Model, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT userid, role FROM "+t.TableName+" WHERE userid = $3 AND role LIKE $4 ORDER BY role "+order+" LIMIT $1 OFFSET $2;", limit, offset, userid, role)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		m := Model{}
		if err := rows.Scan(&m.Userid, &m.Role); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (t *roleModelTable) DelEqRoleHasUserid(ctx context.Context, d db.SQLExecutor, role string, userid []string) error {
	paramCount := 1
	args := make([]interface{}, 0, paramCount+len(userid))
	args = append(args, role)
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
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE role = $1 AND userid IN (VALUES "+placeholdersuserid+");", args...)
	return err
}

func (t *roleModelTable) DelEqUseridEqRole(ctx context.Context, d db.SQLExecutor, userid string, role string) error {
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE userid = $1 AND role = $2;", userid, role)
	return err
}

func (t *roleModelTable) DelEqUseridHasRole(ctx context.Context, d db.SQLExecutor, userid string, role []string) error {
	paramCount := 1
	args := make([]interface{}, 0, paramCount+len(role))
	args = append(args, userid)
	var placeholdersrole string
	{
		placeholders := make([]string, 0, len(role))
		for _, i := range role {
			paramCount++
			placeholders = append(placeholders, fmt.Sprintf("($%d)", paramCount))
			args = append(args, i)
		}
		placeholdersrole = strings.Join(placeholders, ", ")
	}
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE userid = $1 AND role IN (VALUES "+placeholdersrole+");", args...)
	return err
}
