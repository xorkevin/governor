// Code generated by go generate. DO NOT EDIT.
package sessionmodel

import (
	"database/sql"
	"fmt"
	"github.com/lib/pq"
	"strings"
)

const (
	sessionModelTableName = "usersessions"
)

func sessionModelSetup(db *sql.DB) error {
	_, err := db.Exec("CREATE TABLE IF NOT EXISTS usersessions (sessionid VARCHAR(31) PRIMARY KEY, userid VARCHAR(31) NOT NULL, keyhash VARCHAR(127) NOT NULL, time BIGINT NOT NULL, ipaddr VARCHAR(63), user_agent VARCHAR(1023));")
	return err
}

func sessionModelGet(db *sql.DB, key string) (*Model, int, error) {
	m := &Model{}
	if err := db.QueryRow("SELECT sessionid, userid, keyhash, time, ipaddr, user_agent FROM usersessions WHERE sessionid = $1;", key).Scan(&m.SessionID, &m.Userid, &m.KeyHash, &m.Time, &m.IPAddr, &m.UserAgent); err != nil {
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

func sessionModelInsert(db *sql.DB, m *Model) (int, error) {
	_, err := db.Exec("INSERT INTO usersessions (sessionid, userid, keyhash, time, ipaddr, user_agent) VALUES ($1, $2, $3, $4, $5, $6);", m.SessionID, m.Userid, m.KeyHash, m.Time, m.IPAddr, m.UserAgent)
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

func sessionModelInsertBulk(db *sql.DB, models []*Model, allowConflict bool) (int, error) {
	conflictSQL := ""
	if allowConflict {
		conflictSQL = " ON CONFLICT DO NOTHING"
	}
	placeholders := make([]string, 0, len(models))
	args := make([]interface{}, 0, len(models)*6)
	for c, m := range models {
		n := c * 6
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d)", n+1, n+2, n+3, n+4, n+5, n+6))
		args = append(args, m.SessionID, m.Userid, m.KeyHash, m.Time, m.IPAddr, m.UserAgent)
	}
	_, err := db.Exec("INSERT INTO usersessions (sessionid, userid, keyhash, time, ipaddr, user_agent) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
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

func sessionModelUpdate(db *sql.DB, m *Model) error {
	_, err := db.Exec("UPDATE usersessions SET (sessionid, userid, keyhash, time, ipaddr, user_agent) = ($1, $2, $3, $4, $5, $6) WHERE sessionid = $1;", m.SessionID, m.Userid, m.KeyHash, m.Time, m.IPAddr, m.UserAgent)
	return err
}

func sessionModelDelete(db *sql.DB, m *Model) error {
	_, err := db.Exec("DELETE FROM usersessions WHERE sessionid = $1;", m.SessionID)
	return err
}

func sessionModelDelEqUserid(db *sql.DB, userid string) error {
	_, err := db.Exec("DELETE FROM usersessions WHERE userid = $1;", userid)
	return err
}

func sessionModelDelSetSessionID(db *sql.DB, keys []string) error {
	placeholderStart := 1
	placeholders := make([]string, 0, len(keys))
	args := make([]interface{}, 0, len(keys))
	for n, i := range keys {
		placeholders = append(placeholders, fmt.Sprintf("($%d)", n+placeholderStart))
		args = append(args, i)
	}
	_, err := db.Exec("DELETE FROM usersessions WHERE sessionid IN (VALUES "+strings.Join(placeholders, ", ")+");", args...)
	return err
}

func sessionModelGetModelEqUseridOrdTime(db *sql.DB, userid string, orderasc bool, limit, offset int) ([]Model, error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]Model, 0, limit)
	rows, err := db.Query("SELECT sessionid, userid, time, ipaddr, user_agent FROM usersessions WHERE userid = $3 ORDER BY time "+order+" LIMIT $1 OFFSET $2;", limit, offset, userid)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		m := Model{}
		if err := rows.Scan(&m.SessionID, &m.Userid, &m.Time, &m.IPAddr, &m.UserAgent); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func sessionModelGetqIDEqUseridOrdSessionID(db *sql.DB, userid string, orderasc bool, limit, offset int) ([]qID, error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]qID, 0, limit)
	rows, err := db.Query("SELECT sessionid FROM usersessions WHERE userid = $3 ORDER BY sessionid "+order+" LIMIT $1 OFFSET $2;", limit, offset, userid)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		m := qID{}
		if err := rows.Scan(&m.SessionID); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}
