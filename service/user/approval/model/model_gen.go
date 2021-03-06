// Code generated by go generate forge model v0.3; DO NOT EDIT.

package model

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/lib/pq"
)

const (
	approvalModelTableName = "userapprovals"
)

func approvalModelSetup(db *sql.DB) (int, error) {
	_, err := db.Exec("CREATE TABLE IF NOT EXISTS userapprovals (userid VARCHAR(31) PRIMARY KEY, username VARCHAR(255) NOT NULL, pass_hash VARCHAR(255) NOT NULL, email VARCHAR(255) NOT NULL, first_name VARCHAR(255) NOT NULL, last_name VARCHAR(255) NOT NULL, creation_time BIGINT NOT NULL, approved BOOL NOT NULL, code_hash VARCHAR(255) NOT NULL, code_time BIGINT NOT NULL);")
	if err != nil {
		return 0, err
	}
	_, err = db.Exec("CREATE INDEX IF NOT EXISTS userapprovals_creation_time_index ON userapprovals (creation_time);")
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

func approvalModelInsert(db *sql.DB, m *Model) (int, error) {
	_, err := db.Exec("INSERT INTO userapprovals (userid, username, pass_hash, email, first_name, last_name, creation_time, approved, code_hash, code_time) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);", m.Userid, m.Username, m.PassHash, m.Email, m.FirstName, m.LastName, m.CreationTime, m.Approved, m.CodeHash, m.CodeTime)
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

func approvalModelInsertBulk(db *sql.DB, models []*Model, allowConflict bool) (int, error) {
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
	_, err := db.Exec("INSERT INTO userapprovals (userid, username, pass_hash, email, first_name, last_name, creation_time, approved, code_hash, code_time) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
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

func approvalModelGetModelEqUserid(db *sql.DB, userid string) (*Model, int, error) {
	m := &Model{}
	if err := db.QueryRow("SELECT userid, username, pass_hash, email, first_name, last_name, creation_time, approved, code_hash, code_time FROM userapprovals WHERE userid = $1;", userid).Scan(&m.Userid, &m.Username, &m.PassHash, &m.Email, &m.FirstName, &m.LastName, &m.CreationTime, &m.Approved, &m.CodeHash, &m.CodeTime); err != nil {
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

func approvalModelUpdModelEqUserid(db *sql.DB, m *Model, userid string) (int, error) {
	_, err := db.Exec("UPDATE userapprovals SET (userid, username, pass_hash, email, first_name, last_name, creation_time, approved, code_hash, code_time) = ROW($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) WHERE userid = $11;", m.Userid, m.Username, m.PassHash, m.Email, m.FirstName, m.LastName, m.CreationTime, m.Approved, m.CodeHash, m.CodeTime, userid)
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

func approvalModelDelEqUserid(db *sql.DB, userid string) error {
	_, err := db.Exec("DELETE FROM userapprovals WHERE userid = $1;", userid)
	return err
}

func approvalModelGetModelOrdCreationTime(db *sql.DB, orderasc bool, limit, offset int) ([]Model, error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]Model, 0, limit)
	rows, err := db.Query("SELECT userid, username, pass_hash, email, first_name, last_name, creation_time, approved, code_hash, code_time FROM userapprovals ORDER BY creation_time "+order+" LIMIT $1 OFFSET $2;", limit, offset)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		m := Model{}
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
