// Code generated by go generate forge model v0.3; DO NOT EDIT.

package model

import (
	"database/sql"
	"fmt"
	"strings"
)

const (
	sentmsgModelTableName = "mailinglistsentmsgs"
)

func sentmsgModelSetup(db *sql.DB) error {
	_, err := db.Exec("CREATE TABLE IF NOT EXISTS mailinglistsentmsgs (msgid VARCHAR(1023), userid VARCHAR(31), PRIMARY KEY (msgid, userid), sent_time BIGINT NOT NULL);")
	if err != nil {
		return err
	}
	_, err = db.Exec("CREATE INDEX IF NOT EXISTS mailinglistsentmsgs_userid__msgid_index ON mailinglistsentmsgs (userid, msgid);")
	if err != nil {
		return err
	}
	return nil
}

func sentmsgModelInsert(db *sql.DB, m *SentMsgModel) error {
	_, err := db.Exec("INSERT INTO mailinglistsentmsgs (msgid, userid, sent_time) VALUES ($1, $2, $3);", m.Msgid, m.Userid, m.SentTime)
	if err != nil {
		return err
	}
	return nil
}

func sentmsgModelInsertBulk(db *sql.DB, models []*SentMsgModel, allowConflict bool) error {
	conflictSQL := ""
	if allowConflict {
		conflictSQL = " ON CONFLICT DO NOTHING"
	}
	placeholders := make([]string, 0, len(models))
	args := make([]interface{}, 0, len(models)*3)
	for c, m := range models {
		n := c * 3
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d)", n+1, n+2, n+3))
		args = append(args, m.Msgid, m.Userid, m.SentTime)
	}
	_, err := db.Exec("INSERT INTO mailinglistsentmsgs (msgid, userid, sent_time) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
	if err != nil {
		return err
	}
	return nil
}
