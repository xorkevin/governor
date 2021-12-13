// Code generated by go generate forge model v0.3; DO NOT EDIT.

package model

import (
	"database/sql"
	"fmt"
	"strings"
)

func stateModelSetup(db *sql.DB, tableName string) error {
	_, err := db.Exec("CREATE TABLE IF NOT EXISTS " + tableName + " (config INT PRIMARY KEY, setup BOOLEAN NOT NULL, version VARCHAR(255) NOT NULL, vhash VARCHAR(255) NOT NULL, creation_time BIGINT NOT NULL);")
	if err != nil {
		return err
	}
	return nil
}

func stateModelInsert(db *sql.DB, tableName string, m *Model) error {
	_, err := db.Exec("INSERT INTO "+tableName+" (config, setup, version, vhash, creation_time) VALUES ($1, $2, $3, $4, $5);", m.config, m.Setup, m.Version, m.VHash, m.CreationTime)
	if err != nil {
		return err
	}
	return nil
}

func stateModelInsertBulk(db *sql.DB, tableName string, models []*Model, allowConflict bool) error {
	conflictSQL := ""
	if allowConflict {
		conflictSQL = " ON CONFLICT DO NOTHING"
	}
	placeholders := make([]string, 0, len(models))
	args := make([]interface{}, 0, len(models)*5)
	for c, m := range models {
		n := c * 5
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d)", n+1, n+2, n+3, n+4, n+5))
		args = append(args, m.config, m.Setup, m.Version, m.VHash, m.CreationTime)
	}
	_, err := db.Exec("INSERT INTO "+tableName+" (config, setup, version, vhash, creation_time) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
	if err != nil {
		return err
	}
	return nil
}

func stateModelGetModelEqconfig(db *sql.DB, tableName string, config int) (*Model, error) {
	m := &Model{}
	if err := db.QueryRow("SELECT config, setup, version, vhash, creation_time FROM "+tableName+" WHERE config = $1;", config).Scan(&m.config, &m.Setup, &m.Version, &m.VHash, &m.CreationTime); err != nil {
		return nil, err
	}
	return m, nil
}

func stateModelUpdModelEqconfig(db *sql.DB, tableName string, m *Model, config int) error {
	_, err := db.Exec("UPDATE "+tableName+" SET (config, setup, version, vhash, creation_time) = ROW($1, $2, $3, $4, $5) WHERE config = $6;", m.config, m.Setup, m.Version, m.VHash, m.CreationTime, config)
	if err != nil {
		return err
	}
	return nil
}
