// Code generated by go generate forge model v0.3; DO NOT EDIT.

package model

import (
	"database/sql"
	"fmt"
	"strings"
)

func dmModelSetup(db *sql.DB, tableName string) error {
	_, err := db.Exec("CREATE TABLE IF NOT EXISTS " + tableName + " (userid_1 VARCHAR(31), userid_2 VARCHAR(31), PRIMARY KEY (userid_1, userid_2), chatid VARCHAR(31) NOT NULL UNIQUE, name VARCHAR(255) NOT NULL, theme VARCHAR(4095) NOT NULL, last_updated BIGINT NOT NULL, creation_time BIGINT NOT NULL);")
	if err != nil {
		return err
	}
	_, err = db.Exec("CREATE INDEX IF NOT EXISTS " + tableName + "_userid_1__last_updated_index ON " + tableName + " (userid_1, last_updated);")
	if err != nil {
		return err
	}
	_, err = db.Exec("CREATE INDEX IF NOT EXISTS " + tableName + "_userid_2__last_updated_index ON " + tableName + " (userid_2, last_updated);")
	if err != nil {
		return err
	}
	return nil
}

func dmModelInsert(db *sql.DB, tableName string, m *Model) error {
	_, err := db.Exec("INSERT INTO "+tableName+" (userid_1, userid_2, chatid, name, theme, last_updated, creation_time) VALUES ($1, $2, $3, $4, $5, $6, $7);", m.Userid1, m.Userid2, m.Chatid, m.Name, m.Theme, m.LastUpdated, m.CreationTime)
	if err != nil {
		return err
	}
	return nil
}

func dmModelInsertBulk(db *sql.DB, tableName string, models []*Model, allowConflict bool) error {
	conflictSQL := ""
	if allowConflict {
		conflictSQL = " ON CONFLICT DO NOTHING"
	}
	placeholders := make([]string, 0, len(models))
	args := make([]interface{}, 0, len(models)*7)
	for c, m := range models {
		n := c * 7
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d)", n+1, n+2, n+3, n+4, n+5, n+6, n+7))
		args = append(args, m.Userid1, m.Userid2, m.Chatid, m.Name, m.Theme, m.LastUpdated, m.CreationTime)
	}
	_, err := db.Exec("INSERT INTO "+tableName+" (userid_1, userid_2, chatid, name, theme, last_updated, creation_time) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
	if err != nil {
		return err
	}
	return nil
}

func dmModelGetModelEqUserid1EqUserid2(db *sql.DB, tableName string, userid1 string, userid2 string) (*Model, error) {
	m := &Model{}
	if err := db.QueryRow("SELECT userid_1, userid_2, chatid, name, theme, last_updated, creation_time FROM "+tableName+" WHERE userid_1 = $1 AND userid_2 = $2;", userid1, userid2).Scan(&m.Userid1, &m.Userid2, &m.Chatid, &m.Name, &m.Theme, &m.LastUpdated, &m.CreationTime); err != nil {
		return nil, err
	}
	return m, nil
}

func dmModelUpdModelEqUserid1EqUserid2(db *sql.DB, tableName string, m *Model, userid1 string, userid2 string) error {
	_, err := db.Exec("UPDATE "+tableName+" SET (userid_1, userid_2, chatid, name, theme, last_updated, creation_time) = ROW($1, $2, $3, $4, $5, $6, $7) WHERE userid_1 = $8 AND userid_2 = $9;", m.Userid1, m.Userid2, m.Chatid, m.Name, m.Theme, m.LastUpdated, m.CreationTime, userid1, userid2)
	if err != nil {
		return err
	}
	return nil
}

func dmModelDelEqUserid1EqUserid2(db *sql.DB, tableName string, userid1 string, userid2 string) error {
	_, err := db.Exec("DELETE FROM "+tableName+" WHERE userid_1 = $1 AND userid_2 = $2;", userid1, userid2)
	return err
}

func dmModelGetModelEqChatid(db *sql.DB, tableName string, chatid string) (*Model, error) {
	m := &Model{}
	if err := db.QueryRow("SELECT userid_1, userid_2, chatid, name, theme, last_updated, creation_time FROM "+tableName+" WHERE chatid = $1;", chatid).Scan(&m.Userid1, &m.Userid2, &m.Chatid, &m.Name, &m.Theme, &m.LastUpdated, &m.CreationTime); err != nil {
		return nil, err
	}
	return m, nil
}

func dmModelGetModelHasChatidOrdLastUpdated(db *sql.DB, tableName string, chatid []string, orderasc bool, limit, offset int) ([]Model, error) {
	paramCount := 2
	args := make([]interface{}, 0, paramCount+len(chatid))
	args = append(args, limit, offset)
	var placeholderschatid string
	{
		placeholders := make([]string, 0, len(chatid))
		for _, i := range chatid {
			paramCount++
			placeholders = append(placeholders, fmt.Sprintf("($%d)", paramCount))
			args = append(args, i)
		}
		placeholderschatid = strings.Join(placeholders, ", ")
	}
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]Model, 0, limit)
	rows, err := db.Query("SELECT userid_1, userid_2, chatid, name, theme, last_updated, creation_time FROM "+tableName+" WHERE chatid IN (VALUES "+placeholderschatid+") ORDER BY last_updated "+order+" LIMIT $1 OFFSET $2;", args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		m := Model{}
		if err := rows.Scan(&m.Userid1, &m.Userid2, &m.Chatid, &m.Name, &m.Theme, &m.LastUpdated, &m.CreationTime); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func dmModelUpddmLastUpdatedEqUserid1EqUserid2(db *sql.DB, tableName string, m *dmLastUpdated, userid1 string, userid2 string) error {
	_, err := db.Exec("UPDATE "+tableName+" SET (last_updated) = ROW($1) WHERE userid_1 = $2 AND userid_2 = $3;", m.LastUpdated, userid1, userid2)
	if err != nil {
		return err
	}
	return nil
}