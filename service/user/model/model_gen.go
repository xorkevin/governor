// Code generated by go generate forge model v0.2; DO NOT EDIT.
package usermodel

import (
	"database/sql"
	"fmt"
	"github.com/lib/pq"
	"strings"
)

const (
	userModelTableName = "users"
)

func userModelSetup(db *sql.DB) error {
	_, err := db.Exec("CREATE TABLE IF NOT EXISTS users (userid VARCHAR(31) PRIMARY KEY, username VARCHAR(255) NOT NULL UNIQUE, pass_hash VARCHAR(255) NOT NULL, email VARCHAR(255) NOT NULL UNIQUE, first_name VARCHAR(255) NOT NULL, last_name VARCHAR(255) NOT NULL, creation_time BIGINT NOT NULL);")
	if err != nil {
		return err
	}
	return nil
}

func userModelInsert(db *sql.DB, m *Model) (int, error) {
	_, err := db.Exec("INSERT INTO users (userid, username, pass_hash, email, first_name, last_name, creation_time) VALUES ($1, $2, $3, $4, $5, $6, $7);", m.Userid, m.Username, m.PassHash, m.Email, m.FirstName, m.LastName, m.CreationTime)
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

func userModelInsertBulk(db *sql.DB, models []*Model, allowConflict bool) (int, error) {
	conflictSQL := ""
	if allowConflict {
		conflictSQL = " ON CONFLICT DO NOTHING"
	}
	placeholders := make([]string, 0, len(models))
	args := make([]interface{}, 0, len(models)*7)
	for c, m := range models {
		n := c * 7
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d)", n+1, n+2, n+3, n+4, n+5, n+6, n+7))
		args = append(args, m.Userid, m.Username, m.PassHash, m.Email, m.FirstName, m.LastName, m.CreationTime)
	}
	_, err := db.Exec("INSERT INTO users (userid, username, pass_hash, email, first_name, last_name, creation_time) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
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

func userModelGetModelEqUserid(db *sql.DB, userid string) (*Model, int, error) {
	m := &Model{}
	if err := db.QueryRow("SELECT userid, username, pass_hash, email, first_name, last_name, creation_time FROM users WHERE userid = $1;", userid).Scan(&m.Userid, &m.Username, &m.PassHash, &m.Email, &m.FirstName, &m.LastName, &m.CreationTime); err != nil {
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

func userModelUpdateModelEqUserid(db *sql.DB, m *Model, userid string) error {
	_, err := db.Exec("UPDATE users SET (userid, username, pass_hash, email, first_name, last_name, creation_time) = ($1, $2, $3, $4, $5, $6, $7) WHERE userid = $8;", m.Userid, m.Username, m.PassHash, m.Email, m.FirstName, m.LastName, m.CreationTime, userid)
	return err
}

func userModelDelEqUserid(db *sql.DB, userid string) error {
	_, err := db.Exec("DELETE FROM users WHERE userid = $1;", userid)
	return err
}

func userModelGetModelEqUsername(db *sql.DB, username string) (*Model, int, error) {
	m := &Model{}
	if err := db.QueryRow("SELECT userid, username, pass_hash, email, first_name, last_name, creation_time FROM users WHERE username = $1;", username).Scan(&m.Userid, &m.Username, &m.PassHash, &m.Email, &m.FirstName, &m.LastName, &m.CreationTime); err != nil {
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

func userModelGetModelEqEmail(db *sql.DB, email string) (*Model, int, error) {
	m := &Model{}
	if err := db.QueryRow("SELECT userid, username, pass_hash, email, first_name, last_name, creation_time FROM users WHERE email = $1;", email).Scan(&m.Userid, &m.Username, &m.PassHash, &m.Email, &m.FirstName, &m.LastName, &m.CreationTime); err != nil {
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

func userModelGetInfoOrdUserid(db *sql.DB, orderasc bool, limit, offset int) ([]Info, error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]Info, 0, limit)
	rows, err := db.Query("SELECT userid, username, email, first_name, last_name FROM users ORDER BY userid "+order+" LIMIT $1 OFFSET $2;", limit, offset)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		m := Info{}
		if err := rows.Scan(&m.Userid, &m.Username, &m.Email, &m.FirstName, &m.LastName); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func userModelGetInfoEqHasUseridOrdUserid(db *sql.DB, userid []string, orderasc bool, limit, offset int) ([]Info, error) {
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
	res := make([]Info, 0, limit)
	rows, err := db.Query("SELECT userid, username, email, first_name, last_name FROM users WHERE userid IN (VALUES "+placeholdersuserid+") ORDER BY userid "+order+" LIMIT $1 OFFSET $2;", args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		m := Info{}
		if err := rows.Scan(&m.Userid, &m.Username, &m.Email, &m.FirstName, &m.LastName); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}
