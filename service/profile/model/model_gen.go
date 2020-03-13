// Code generated by go generate forge model v0.3; DO NOT EDIT.

package profilemodel

import (
	"database/sql"
	"fmt"
	"github.com/lib/pq"
	"strings"
)

const (
	profileModelTableName = "profiles"
)

func profileModelSetup(db *sql.DB) error {
	_, err := db.Exec("CREATE TABLE IF NOT EXISTS profiles (userid VARCHAR(31) PRIMARY KEY, contact_email VARCHAR(255), bio VARCHAR(4095), profile_image_url VARCHAR(4095));")
	if err != nil {
		return err
	}
	return nil
}

func profileModelInsert(db *sql.DB, m *Model) (int, error) {
	_, err := db.Exec("INSERT INTO profiles (userid, contact_email, bio, profile_image_url) VALUES ($1, $2, $3, $4);", m.Userid, m.Email, m.Bio, m.Image)
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

func profileModelInsertBulk(db *sql.DB, models []*Model, allowConflict bool) (int, error) {
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
	_, err := db.Exec("INSERT INTO profiles (userid, contact_email, bio, profile_image_url) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
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

func profileModelGetModelEqUserid(db *sql.DB, userid string) (*Model, int, error) {
	m := &Model{}
	if err := db.QueryRow("SELECT userid, contact_email, bio, profile_image_url FROM profiles WHERE userid = $1;", userid).Scan(&m.Userid, &m.Email, &m.Bio, &m.Image); err != nil {
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

func profileModelUpdModelEqUserid(db *sql.DB, m *Model, userid string) (int, error) {
	_, err := db.Exec("UPDATE profiles SET (userid, contact_email, bio, profile_image_url) = ($1, $2, $3, $4) WHERE userid = $5;", m.Userid, m.Email, m.Bio, m.Image, userid)
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

func profileModelDelEqUserid(db *sql.DB, userid string) error {
	_, err := db.Exec("DELETE FROM profiles WHERE userid = $1;", userid)
	return err
}
