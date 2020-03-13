// Code generated by go generate forge model v0.3; DO NOT EDIT.

package statemodel

import (
	"database/sql"
	"fmt"
	"github.com/lib/pq"
	"strings"
)

const (
	stateModelTableName = "govstate"
)

func stateModelSetup(db *sql.DB) error {
	_, err := db.Exec("CREATE TABLE IF NOT EXISTS govstate (config INT PRIMARY KEY, orgname VARCHAR(255) NOT NULL, setup BOOLEAN NOT NULL, creation_time BIGINT NOT NULL);")
	if err != nil {
		return err
	}
	return nil
}

func stateModelInsert(db *sql.DB, m *Model) (int, error) {
	_, err := db.Exec("INSERT INTO govstate (config, orgname, setup, creation_time) VALUES ($1, $2, $3, $4);", m.config, m.Orgname, m.Setup, m.CreationTime)
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

func stateModelInsertBulk(db *sql.DB, models []*Model, allowConflict bool) (int, error) {
	conflictSQL := ""
	if allowConflict {
		conflictSQL = " ON CONFLICT DO NOTHING"
	}
	placeholders := make([]string, 0, len(models))
	args := make([]interface{}, 0, len(models)*4)
	for c, m := range models {
		n := c * 4
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d)", n+1, n+2, n+3, n+4))
		args = append(args, m.config, m.Orgname, m.Setup, m.CreationTime)
	}
	_, err := db.Exec("INSERT INTO govstate (config, orgname, setup, creation_time) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
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

func stateModelGetModelEqconfig(db *sql.DB, config int) (*Model, int, error) {
	m := &Model{}
	if err := db.QueryRow("SELECT config, orgname, setup, creation_time FROM govstate WHERE config = $1;", config).Scan(&m.config, &m.Orgname, &m.Setup, &m.CreationTime); err != nil {
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

func stateModelUpdModelEqconfig(db *sql.DB, m *Model, config int) (int, error) {
	_, err := db.Exec("UPDATE govstate SET (config, orgname, setup, creation_time) = ($1, $2, $3, $4) WHERE config = $5;", m.config, m.Orgname, m.Setup, m.CreationTime, config)
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
