// Code generated by go generate forge model v0.3; DO NOT EDIT.

package model

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/lib/pq"
)

const (
	apikeyModelTableName = "userapikeys"
)

func apikeyModelSetup(db *sql.DB) (int, error) {
	_, err := db.Exec("CREATE TABLE IF NOT EXISTS userapikeys (keyid VARCHAR(63) PRIMARY KEY, userid VARCHAR(31) NOT NULL, scope VARCHAR(4095) NOT NULL, keyhash VARCHAR(127) NOT NULL, name VARCHAR(255) NOT NULL, description VARCHAR(255), time BIGINT NOT NULL);")
	if err != nil {
		return 0, err
	}
	_, err = db.Exec("CREATE INDEX IF NOT EXISTS userapikeys_userid_index ON userapikeys (userid);")
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
	_, err = db.Exec("CREATE INDEX IF NOT EXISTS userapikeys_userid__time_index ON userapikeys (userid, time);")
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

func apikeyModelInsert(db *sql.DB, m *Model) (int, error) {
	_, err := db.Exec("INSERT INTO userapikeys (keyid, userid, scope, keyhash, name, description, time) VALUES ($1, $2, $3, $4, $5, $6, $7);", m.Keyid, m.Userid, m.Scope, m.KeyHash, m.Name, m.Desc, m.Time)
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

func apikeyModelInsertBulk(db *sql.DB, models []*Model, allowConflict bool) (int, error) {
	conflictSQL := ""
	if allowConflict {
		conflictSQL = " ON CONFLICT DO NOTHING"
	}
	placeholders := make([]string, 0, len(models))
	args := make([]interface{}, 0, len(models)*7)
	for c, m := range models {
		n := c * 7
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d)", n+1, n+2, n+3, n+4, n+5, n+6, n+7))
		args = append(args, m.Keyid, m.Userid, m.Scope, m.KeyHash, m.Name, m.Desc, m.Time)
	}
	_, err := db.Exec("INSERT INTO userapikeys (keyid, userid, scope, keyhash, name, description, time) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
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

func apikeyModelGetModelEqKeyid(db *sql.DB, keyid string) (*Model, int, error) {
	m := &Model{}
	if err := db.QueryRow("SELECT keyid, userid, scope, keyhash, name, description, time FROM userapikeys WHERE keyid = $1;", keyid).Scan(&m.Keyid, &m.Userid, &m.Scope, &m.KeyHash, &m.Name, &m.Desc, &m.Time); err != nil {
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

func apikeyModelUpdModelEqKeyid(db *sql.DB, m *Model, keyid string) (int, error) {
	_, err := db.Exec("UPDATE userapikeys SET (keyid, userid, scope, keyhash, name, description, time) = ROW($1, $2, $3, $4, $5, $6, $7) WHERE keyid = $8;", m.Keyid, m.Userid, m.Scope, m.KeyHash, m.Name, m.Desc, m.Time, keyid)
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

func apikeyModelDelEqKeyid(db *sql.DB, keyid string) error {
	_, err := db.Exec("DELETE FROM userapikeys WHERE keyid = $1;", keyid)
	return err
}

func apikeyModelDelEqUserid(db *sql.DB, userid string) error {
	_, err := db.Exec("DELETE FROM userapikeys WHERE userid = $1;", userid)
	return err
}

func apikeyModelGetModelEqUseridOrdTime(db *sql.DB, userid string, orderasc bool, limit, offset int) ([]Model, error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]Model, 0, limit)
	rows, err := db.Query("SELECT keyid, userid, scope, keyhash, name, description, time FROM userapikeys WHERE userid = $3 ORDER BY time "+order+" LIMIT $1 OFFSET $2;", limit, offset, userid)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		m := Model{}
		if err := rows.Scan(&m.Keyid, &m.Userid, &m.Scope, &m.KeyHash, &m.Name, &m.Desc, &m.Time); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}
