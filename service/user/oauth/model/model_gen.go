// Code generated by go generate forge model v0.3; DO NOT EDIT.

package oauthmodel

import (
	"database/sql"
	"fmt"
	"github.com/lib/pq"
	"strings"
)

const (
	oauthappModelTableName = "oauthapps"
)

func oauthappModelSetup(db *sql.DB) error {
	_, err := db.Exec("CREATE TABLE IF NOT EXISTS oauthapps (clientid VARCHAR(31) PRIMARY KEY, name VARCHAR(255) NOT NULL, url VARCHAR(255) NOT NULL, redirect_uri VARCHAR(2047) NOT NULL, logo VARCHAR(4095), keyhash VARCHAR(255) NOT NULL, time BIGINT NOT NULL, creation_time BIGINT NOT NULL);")
	if err != nil {
		return err
	}
	return nil
}

func oauthappModelInsert(db *sql.DB, m *Model) (int, error) {
	_, err := db.Exec("INSERT INTO oauthapps (clientid, name, url, redirect_uri, logo, keyhash, time, creation_time) VALUES ($1, $2, $3, $4, $5, $6, $7, $8);", m.ClientID, m.Name, m.URL, m.RedirectURI, m.Logo, m.KeyHash, m.Time, m.CreationTime)
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

func oauthappModelInsertBulk(db *sql.DB, models []*Model, allowConflict bool) (int, error) {
	conflictSQL := ""
	if allowConflict {
		conflictSQL = " ON CONFLICT DO NOTHING"
	}
	placeholders := make([]string, 0, len(models))
	args := make([]interface{}, 0, len(models)*8)
	for c, m := range models {
		n := c * 8
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)", n+1, n+2, n+3, n+4, n+5, n+6, n+7, n+8))
		args = append(args, m.ClientID, m.Name, m.URL, m.RedirectURI, m.Logo, m.KeyHash, m.Time, m.CreationTime)
	}
	_, err := db.Exec("INSERT INTO oauthapps (clientid, name, url, redirect_uri, logo, keyhash, time, creation_time) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
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

func oauthappModelGetModelEqClientID(db *sql.DB, clientid string) (*Model, int, error) {
	m := &Model{}
	if err := db.QueryRow("SELECT clientid, name, url, redirect_uri, logo, keyhash, time, creation_time FROM oauthapps WHERE clientid = $1;", clientid).Scan(&m.ClientID, &m.Name, &m.URL, &m.RedirectURI, &m.Logo, &m.KeyHash, &m.Time, &m.CreationTime); err != nil {
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

func oauthappModelUpdModelEqClientID(db *sql.DB, m *Model, clientid string) (int, error) {
	_, err := db.Exec("UPDATE oauthapps SET (clientid, name, url, redirect_uri, logo, keyhash, time, creation_time) = ROW($1, $2, $3, $4, $5, $6, $7, $8) WHERE clientid = $9;", m.ClientID, m.Name, m.URL, m.RedirectURI, m.Logo, m.KeyHash, m.Time, m.CreationTime, clientid)
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

func oauthappModelDelEqClientID(db *sql.DB, clientid string) error {
	_, err := db.Exec("DELETE FROM oauthapps WHERE clientid = $1;", clientid)
	return err
}

func oauthappModelGetModelOrdTime(db *sql.DB, orderasc bool, limit, offset int) ([]Model, error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]Model, 0, limit)
	rows, err := db.Query("SELECT clientid, name, url, redirect_uri, logo, keyhash, time, creation_time FROM oauthapps ORDER BY time "+order+" LIMIT $1 OFFSET $2;", limit, offset)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		m := Model{}
		if err := rows.Scan(&m.ClientID, &m.Name, &m.URL, &m.RedirectURI, &m.Logo, &m.KeyHash, &m.Time, &m.CreationTime); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}
