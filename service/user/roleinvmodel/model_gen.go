// Code generated by go generate forge model v0.4.1; DO NOT EDIT.

package roleinvmodel

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"xorkevin.dev/governor/service/db"
)

type (
	invModelTable struct {
		TableName string
	}
)

func (t *invModelTable) Setup(ctx context.Context, d db.SQLExecutor) error {
	_, err := d.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS "+t.TableName+" (userid VARCHAR(31), role VARCHAR(255), PRIMARY KEY (userid, role), invited_by VARCHAR(31) NOT NULL, creation_time BIGINT NOT NULL);")
	if err != nil {
		return err
	}
	_, err = d.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS "+t.TableName+"_creation_time_index ON "+t.TableName+" (creation_time);")
	if err != nil {
		return err
	}
	_, err = d.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS "+t.TableName+"_userid__creation_time_index ON "+t.TableName+" (userid, creation_time);")
	if err != nil {
		return err
	}
	_, err = d.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS "+t.TableName+"_role__creation_time_index ON "+t.TableName+" (role, creation_time);")
	if err != nil {
		return err
	}
	return nil
}

func (t *invModelTable) Insert(ctx context.Context, d db.SQLExecutor, m *Model) error {
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (userid, role, invited_by, creation_time) VALUES ($1, $2, $3, $4);", m.Userid, m.Role, m.InvitedBy, m.CreationTime)
	if err != nil {
		return err
	}
	return nil
}

func (t *invModelTable) InsertBulk(ctx context.Context, d db.SQLExecutor, models []*Model, allowConflict bool) error {
	conflictSQL := ""
	if allowConflict {
		conflictSQL = " ON CONFLICT DO NOTHING"
	}
	placeholders := make([]string, 0, len(models))
	args := make([]interface{}, 0, len(models)*4)
	for c, m := range models {
		n := c * 4
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d)", n+1, n+2, n+3, n+4))
		args = append(args, m.Userid, m.Role, m.InvitedBy, m.CreationTime)
	}
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (userid, role, invited_by, creation_time) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
	if err != nil {
		return err
	}
	return nil
}

func (t *invModelTable) GetModelEqUseridEqRoleGtCreationTime(ctx context.Context, d db.SQLExecutor, userid string, role string, creationtime int64) (*Model, error) {
	m := &Model{}
	if err := d.QueryRowContext(ctx, "SELECT userid, role, invited_by, creation_time FROM "+t.TableName+" WHERE userid = $1 AND role = $2 AND creation_time > $3;", userid, role, creationtime).Scan(&m.Userid, &m.Role, &m.InvitedBy, &m.CreationTime); err != nil {
		return nil, err
	}
	return m, nil
}

func (t *invModelTable) DelEqUseridEqRole(ctx context.Context, d db.SQLExecutor, userid string, role string) error {
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE userid = $1 AND role = $2;", userid, role)
	return err
}

func (t *invModelTable) DelEqUseridHasRole(ctx context.Context, d db.SQLExecutor, userid string, roles []string) error {
	paramCount := 1
	args := make([]interface{}, 0, paramCount+len(roles))
	args = append(args, userid)
	var placeholdersroles string
	{
		placeholders := make([]string, 0, len(roles))
		for _, i := range roles {
			paramCount++
			placeholders = append(placeholders, fmt.Sprintf("($%d)", paramCount))
			args = append(args, i)
		}
		placeholdersroles = strings.Join(placeholders, ", ")
	}
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE userid = $1 AND role IN (VALUES "+placeholdersroles+");", args...)
	return err
}

func (t *invModelTable) DelEqRole(ctx context.Context, d db.SQLExecutor, role string) error {
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE role = $1;", role)
	return err
}

func (t *invModelTable) GetModelEqUseridGtCreationTimeOrdCreationTime(ctx context.Context, d db.SQLExecutor, userid string, creationtime int64, orderasc bool, limit, offset int) (_ []Model, retErr error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]Model, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT userid, role, invited_by, creation_time FROM "+t.TableName+" WHERE userid = $3 AND creation_time > $4 ORDER BY creation_time "+order+" LIMIT $1 OFFSET $2;", limit, offset, userid, creationtime)
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
		if err := rows.Scan(&m.Userid, &m.Role, &m.InvitedBy, &m.CreationTime); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (t *invModelTable) GetModelEqRoleGtCreationTimeOrdCreationTime(ctx context.Context, d db.SQLExecutor, role string, creationtime int64, orderasc bool, limit, offset int) (_ []Model, retErr error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]Model, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT userid, role, invited_by, creation_time FROM "+t.TableName+" WHERE role = $3 AND creation_time > $4 ORDER BY creation_time "+order+" LIMIT $1 OFFSET $2;", limit, offset, role, creationtime)
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
		if err := rows.Scan(&m.Userid, &m.Role, &m.InvitedBy, &m.CreationTime); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (t *invModelTable) DelLeqCreationTime(ctx context.Context, d db.SQLExecutor, creationtime int64) error {
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE creation_time <= $1;", creationtime)
	return err
}
