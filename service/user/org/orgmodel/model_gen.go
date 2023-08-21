// Code generated by go generate forge model v0.5.1; DO NOT EDIT.

package orgmodel

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"xorkevin.dev/forge/model/sqldb"
)

type (
	orgModelTable struct {
		TableName string
	}
)

func (t *orgModelTable) Setup(ctx context.Context, d sqldb.Executor) error {
	_, err := d.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS "+t.TableName+" (orgid VARCHAR(31) PRIMARY KEY, name VARCHAR(255) NOT NULL UNIQUE, display_name VARCHAR(255) NOT NULL, description VARCHAR(255) NOT NULL, creation_time BIGINT NOT NULL);")
	if err != nil {
		return err
	}
	return nil
}

func (t *orgModelTable) Insert(ctx context.Context, d sqldb.Executor, m *Model) error {
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (orgid, name, display_name, description, creation_time) VALUES ($1, $2, $3, $4, $5);", m.OrgID, m.Name, m.DisplayName, m.Desc, m.CreationTime)
	if err != nil {
		return err
	}
	return nil
}

func (t *orgModelTable) InsertBulk(ctx context.Context, d sqldb.Executor, models []*Model, allowConflict bool) error {
	conflictSQL := ""
	if allowConflict {
		conflictSQL = " ON CONFLICT DO NOTHING"
	}
	placeholders := make([]string, 0, len(models))
	args := make([]interface{}, 0, len(models)*5)
	for c, m := range models {
		n := c * 5
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d)", n+1, n+2, n+3, n+4, n+5))
		args = append(args, m.OrgID, m.Name, m.DisplayName, m.Desc, m.CreationTime)
	}
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (orgid, name, display_name, description, creation_time) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
	if err != nil {
		return err
	}
	return nil
}

func (t *orgModelTable) GetModelByID(ctx context.Context, d sqldb.Executor, orgid string) (*Model, error) {
	m := &Model{}
	if err := d.QueryRowContext(ctx, "SELECT orgid, name, display_name, description, creation_time FROM "+t.TableName+" WHERE orgid = $1;", orgid).Scan(&m.OrgID, &m.Name, &m.DisplayName, &m.Desc, &m.CreationTime); err != nil {
		return nil, err
	}
	return m, nil
}

func (t *orgModelTable) GetModelByIDs(ctx context.Context, d sqldb.Executor, orgids []string, limit, offset int) (_ []Model, retErr error) {
	paramCount := 2
	args := make([]interface{}, 0, paramCount+len(orgids))
	args = append(args, limit, offset)
	var placeholdersorgids string
	{
		placeholders := make([]string, 0, len(orgids))
		for _, i := range orgids {
			paramCount++
			placeholders = append(placeholders, fmt.Sprintf("($%d)", paramCount))
			args = append(args, i)
		}
		placeholdersorgids = strings.Join(placeholders, ", ")
	}
	res := make([]Model, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT orgid, name, display_name, description, creation_time FROM "+t.TableName+" WHERE orgid IN (VALUES "+placeholdersorgids+") LIMIT $1 OFFSET $2;", args...)
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
		if err := rows.Scan(&m.OrgID, &m.Name, &m.DisplayName, &m.Desc, &m.CreationTime); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (t *orgModelTable) UpdModelByID(ctx context.Context, d sqldb.Executor, m *Model, orgid string) error {
	_, err := d.ExecContext(ctx, "UPDATE "+t.TableName+" SET (orgid, name, display_name, description, creation_time) = ($1, $2, $3, $4, $5) WHERE orgid = $6;", m.OrgID, m.Name, m.DisplayName, m.Desc, m.CreationTime, orgid)
	if err != nil {
		return err
	}
	return nil
}

func (t *orgModelTable) DelByID(ctx context.Context, d sqldb.Executor, orgid string) error {
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE orgid = $1;", orgid)
	return err
}

func (t *orgModelTable) GetModelByName(ctx context.Context, d sqldb.Executor, name string) (*Model, error) {
	m := &Model{}
	if err := d.QueryRowContext(ctx, "SELECT orgid, name, display_name, description, creation_time FROM "+t.TableName+" WHERE name = $1;", name).Scan(&m.OrgID, &m.Name, &m.DisplayName, &m.Desc, &m.CreationTime); err != nil {
		return nil, err
	}
	return m, nil
}

func (t *orgModelTable) GetModelAll(ctx context.Context, d sqldb.Executor, limit, offset int) (_ []Model, retErr error) {
	res := make([]Model, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT orgid, name, display_name, description, creation_time FROM "+t.TableName+" ORDER BY name LIMIT $1 OFFSET $2;", limit, offset)
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
		if err := rows.Scan(&m.OrgID, &m.Name, &m.DisplayName, &m.Desc, &m.CreationTime); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

type (
	memberModelTable struct {
		TableName string
	}
)

func (t *memberModelTable) Setup(ctx context.Context, d sqldb.Executor) error {
	_, err := d.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS "+t.TableName+" (orgid VARCHAR(31), userid VARCHAR(31), name VARCHAR(255) NOT NULL, username VARCHAR(255) NOT NULL, PRIMARY KEY (orgid, userid));")
	if err != nil {
		return err
	}
	_, err = d.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS "+t.TableName+"_userid__name_index ON "+t.TableName+" (userid, name);")
	if err != nil {
		return err
	}
	_, err = d.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS "+t.TableName+"_orgid__username_index ON "+t.TableName+" (orgid, username);")
	if err != nil {
		return err
	}
	return nil
}

func (t *memberModelTable) Insert(ctx context.Context, d sqldb.Executor, m *MemberModel) error {
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (orgid, userid, name, username) VALUES ($1, $2, $3, $4);", m.OrgID, m.Userid, m.Name, m.Username)
	if err != nil {
		return err
	}
	return nil
}

func (t *memberModelTable) InsertBulk(ctx context.Context, d sqldb.Executor, models []*MemberModel, allowConflict bool) error {
	conflictSQL := ""
	if allowConflict {
		conflictSQL = " ON CONFLICT DO NOTHING"
	}
	placeholders := make([]string, 0, len(models))
	args := make([]interface{}, 0, len(models)*4)
	for c, m := range models {
		n := c * 4
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d)", n+1, n+2, n+3, n+4))
		args = append(args, m.OrgID, m.Userid, m.Name, m.Username)
	}
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (orgid, userid, name, username) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
	if err != nil {
		return err
	}
	return nil
}

func (t *memberModelTable) DelByOrgid(ctx context.Context, d sqldb.Executor, orgid string) error {
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE orgid = $1;", orgid)
	return err
}

func (t *memberModelTable) DelByUserOrgs(ctx context.Context, d sqldb.Executor, userid string, orgids []string) error {
	paramCount := 1
	args := make([]interface{}, 0, paramCount+len(orgids))
	args = append(args, userid)
	var placeholdersorgids string
	{
		placeholders := make([]string, 0, len(orgids))
		for _, i := range orgids {
			paramCount++
			placeholders = append(placeholders, fmt.Sprintf("($%d)", paramCount))
			args = append(args, i)
		}
		placeholdersorgids = strings.Join(placeholders, ", ")
	}
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE userid = $1 AND orgid IN (VALUES "+placeholdersorgids+");", args...)
	return err
}

func (t *memberModelTable) GetMemberModelByUserid(ctx context.Context, d sqldb.Executor, userid string, limit, offset int) (_ []MemberModel, retErr error) {
	res := make([]MemberModel, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT orgid, userid, name, username FROM "+t.TableName+" WHERE userid = $3 ORDER BY name LIMIT $1 OFFSET $2;", limit, offset, userid)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("Failed to close db rows: %w", err))
		}
	}()
	for rows.Next() {
		var m MemberModel
		if err := rows.Scan(&m.OrgID, &m.Userid, &m.Name, &m.Username); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (t *memberModelTable) GetMemberModelByUserOrgNamePrefix(ctx context.Context, d sqldb.Executor, userid string, namePrefix string, limit, offset int) (_ []MemberModel, retErr error) {
	res := make([]MemberModel, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT orgid, userid, name, username FROM "+t.TableName+" WHERE userid = $3 AND name LIKE $4 ORDER BY name LIMIT $1 OFFSET $2;", limit, offset, userid, namePrefix)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("Failed to close db rows: %w", err))
		}
	}()
	for rows.Next() {
		var m MemberModel
		if err := rows.Scan(&m.OrgID, &m.Userid, &m.Name, &m.Username); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (t *memberModelTable) GetMemberModelByOrgid(ctx context.Context, d sqldb.Executor, orgid string, limit, offset int) (_ []MemberModel, retErr error) {
	res := make([]MemberModel, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT orgid, userid, name, username FROM "+t.TableName+" WHERE orgid = $3 ORDER BY username LIMIT $1 OFFSET $2;", limit, offset, orgid)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("Failed to close db rows: %w", err))
		}
	}()
	for rows.Next() {
		var m MemberModel
		if err := rows.Scan(&m.OrgID, &m.Userid, &m.Name, &m.Username); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (t *memberModelTable) GetMemberModelByOrgUsernamePrefix(ctx context.Context, d sqldb.Executor, orgid string, usernamePrefix string, limit, offset int) (_ []MemberModel, retErr error) {
	res := make([]MemberModel, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT orgid, userid, name, username FROM "+t.TableName+" WHERE orgid = $3 AND username LIKE $4 ORDER BY username LIMIT $1 OFFSET $2;", limit, offset, orgid, usernamePrefix)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("Failed to close db rows: %w", err))
		}
	}()
	for rows.Next() {
		var m MemberModel
		if err := rows.Scan(&m.OrgID, &m.Userid, &m.Name, &m.Username); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (t *memberModelTable) UpdorgNameByID(ctx context.Context, d sqldb.Executor, m *orgName, orgid string) error {
	_, err := d.ExecContext(ctx, "UPDATE "+t.TableName+" SET name = $1 WHERE orgid = $2;", m.Name, orgid)
	if err != nil {
		return err
	}
	return nil
}

func (t *memberModelTable) UpdmemberUsernameByUserid(ctx context.Context, d sqldb.Executor, m *memberUsername, userid string) error {
	_, err := d.ExecContext(ctx, "UPDATE "+t.TableName+" SET username = $1 WHERE userid = $2;", m.Username, userid)
	if err != nil {
		return err
	}
	return nil
}
