// Code generated by go generate forge model v0.3; DO NOT EDIT.

package model

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/lib/pq"
)

const (
	invModelTableName = "userroleinvitations"
)

func invModelSetup(db *sql.DB) (int, error) {
	_, err := db.Exec("CREATE TABLE IF NOT EXISTS userroleinvitations (userid VARCHAR(31), role VARCHAR(255), PRIMARY KEY (userid, role), invited_by VARCHAR(31) NOT NULL, creation_time BIGINT NOT NULL);")
	if err != nil {
		return 0, err
	}
	_, err = db.Exec("CREATE INDEX IF NOT EXISTS userroleinvitations_userid_index ON userroleinvitations (userid);")
	if err != nil {
		if postgresErr, ok := err.(*pq.Error); ok {
			switch postgresErr.Code {
			case "42501": // insufficient_privilege
				return 5, err
			default:
				return 0, err
			}
		}
	}
	_, err = db.Exec("CREATE INDEX IF NOT EXISTS userroleinvitations_role_index ON userroleinvitations (role);")
	if err != nil {
		if postgresErr, ok := err.(*pq.Error); ok {
			switch postgresErr.Code {
			case "42501": // insufficient_privilege
				return 5, err
			default:
				return 0, err
			}
		}
	}
	_, err = db.Exec("CREATE INDEX IF NOT EXISTS userroleinvitations_creation_time_index ON userroleinvitations (creation_time);")
	if err != nil {
		if postgresErr, ok := err.(*pq.Error); ok {
			switch postgresErr.Code {
			case "42501": // insufficient_privilege
				return 5, err
			default:
				return 0, err
			}
		}
	}
	return 0, nil
}

func invModelInsert(db *sql.DB, m *Model) (int, error) {
	_, err := db.Exec("INSERT INTO userroleinvitations (userid, role, invited_by, creation_time) VALUES ($1, $2, $3, $4);", m.Userid, m.Role, m.InvitedBy, m.CreationTime)
	if err != nil {
		if postgresErr, ok := err.(*pq.Error); ok {
			switch postgresErr.Code {
			case "23505": // unique_violation
				return 3, err
			default:
				return 0, err
			}
		}
	}
	return 0, nil
}

func invModelInsertBulk(db *sql.DB, models []*Model, allowConflict bool) (int, error) {
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
	_, err := db.Exec("INSERT INTO userroleinvitations (userid, role, invited_by, creation_time) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
	if err != nil {
		if postgresErr, ok := err.(*pq.Error); ok {
			switch postgresErr.Code {
			case "23505": // unique_violation
				return 3, err
			default:
				return 0, err
			}
		}
	}
	return 0, nil
}

func invModelGetModelEqUseridEqRoleGtCreationTime(db *sql.DB, userid string, role string, creationtime int64) (*Model, int, error) {
	m := &Model{}
	if err := db.QueryRow("SELECT userid, role, invited_by, creation_time FROM userroleinvitations WHERE userid = $1 AND role = $2 AND creation_time > $3;", userid, role, creationtime).Scan(&m.Userid, &m.Role, &m.InvitedBy, &m.CreationTime); err != nil {
		if err == sql.ErrNoRows {
			return nil, 2, err
		}
		if postgresErr, ok := err.(*pq.Error); ok {
			switch postgresErr.Code {
			case "42P01": // undefined_table
				return nil, 4, err
			default:
				return nil, 0, err
			}
		}
		return nil, 0, err
	}
	return m, 0, nil
}

func invModelDelEqUseridEqRole(db *sql.DB, userid string, role string) error {
	_, err := db.Exec("DELETE FROM userroleinvitations WHERE userid = $1 AND role = $2;", userid, role)
	return err
}

func invModelDelEqUseridHasRole(db *sql.DB, userid string, role []string) error {
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
	_, err := db.Exec("DELETE FROM userroleinvitations WHERE userid = $1 AND role IN (VALUES "+placeholdersrole+");", args...)
	return err
}

func invModelGetModelEqUseridGtCreationTimeOrdCreationTime(db *sql.DB, userid string, creationtime int64, orderasc bool, limit, offset int) ([]Model, error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]Model, 0, limit)
	rows, err := db.Query("SELECT userid, role, invited_by, creation_time FROM userroleinvitations WHERE userid = $3 AND creation_time > $4 ORDER BY creation_time "+order+" LIMIT $1 OFFSET $2;", limit, offset, userid, creationtime)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		m := Model{}
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

func invModelGetModelEqRoleGtCreationTimeOrdCreationTime(db *sql.DB, role string, creationtime int64, orderasc bool, limit, offset int) ([]Model, error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]Model, 0, limit)
	rows, err := db.Query("SELECT userid, role, invited_by, creation_time FROM userroleinvitations WHERE role = $3 AND creation_time > $4 ORDER BY creation_time "+order+" LIMIT $1 OFFSET $2;", limit, offset, role, creationtime)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		m := Model{}
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

func invModelDelLeqCreationTime(db *sql.DB, creationtime int64) error {
	_, err := db.Exec("DELETE FROM userroleinvitations WHERE creation_time <= $1;", creationtime)
	return err
}
