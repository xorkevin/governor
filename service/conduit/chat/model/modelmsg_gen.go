// Code generated by go generate forge model v0.3; DO NOT EDIT.

package model

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/lib/pq"
)

const (
	msgModelTableName = "chatmessages"
)

func msgModelSetup(db *sql.DB) (int, error) {
	_, err := db.Exec("CREATE TABLE IF NOT EXISTS chatmessages (chatid VARCHAR(31), msgid VARCHAR(31), PRIMARY KEY (chatid, msgid), userid VARCHAR(31) NOT NULL, time_ms BIGINT NOT NULL, kind VARCHAR(31) NOT NULL, value VARCHAR(4095) NOT NULL);")
	if err != nil {
		return 0, err
	}
	_, err = db.Exec("CREATE INDEX IF NOT EXISTS chatmessages_chatid__kind__msgid_index ON chatmessages (chatid, kind, msgid);")
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

func msgModelInsert(db *sql.DB, m *MsgModel) (int, error) {
	_, err := db.Exec("INSERT INTO chatmessages (chatid, msgid, userid, time_ms, kind, value) VALUES ($1, $2, $3, $4, $5, $6);", m.Chatid, m.Msgid, m.Userid, m.Timems, m.Kind, m.Value)
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

func msgModelInsertBulk(db *sql.DB, models []*MsgModel, allowConflict bool) (int, error) {
	conflictSQL := ""
	if allowConflict {
		conflictSQL = " ON CONFLICT DO NOTHING"
	}
	placeholders := make([]string, 0, len(models))
	args := make([]interface{}, 0, len(models)*6)
	for c, m := range models {
		n := c * 6
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d)", n+1, n+2, n+3, n+4, n+5, n+6))
		args = append(args, m.Chatid, m.Msgid, m.Userid, m.Timems, m.Kind, m.Value)
	}
	_, err := db.Exec("INSERT INTO chatmessages (chatid, msgid, userid, time_ms, kind, value) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
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

func msgModelGetMsgModelEqChatidOrdMsgid(db *sql.DB, chatid string, orderasc bool, limit, offset int) ([]MsgModel, error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]MsgModel, 0, limit)
	rows, err := db.Query("SELECT chatid, msgid, userid, time_ms, kind, value FROM chatmessages WHERE chatid = $3 ORDER BY msgid "+order+" LIMIT $1 OFFSET $2;", limit, offset, chatid)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		m := MsgModel{}
		if err := rows.Scan(&m.Chatid, &m.Msgid, &m.Userid, &m.Timems, &m.Kind, &m.Value); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func msgModelGetMsgModelEqChatidLtMsgidOrdMsgid(db *sql.DB, chatid string, msgid string, orderasc bool, limit, offset int) ([]MsgModel, error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]MsgModel, 0, limit)
	rows, err := db.Query("SELECT chatid, msgid, userid, time_ms, kind, value FROM chatmessages WHERE chatid = $3 AND msgid < $4 ORDER BY msgid "+order+" LIMIT $1 OFFSET $2;", limit, offset, chatid, msgid)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		m := MsgModel{}
		if err := rows.Scan(&m.Chatid, &m.Msgid, &m.Userid, &m.Timems, &m.Kind, &m.Value); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func msgModelGetMsgModelEqChatidEqKindOrdMsgid(db *sql.DB, chatid string, kind string, orderasc bool, limit, offset int) ([]MsgModel, error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]MsgModel, 0, limit)
	rows, err := db.Query("SELECT chatid, msgid, userid, time_ms, kind, value FROM chatmessages WHERE chatid = $3 AND kind = $4 ORDER BY msgid "+order+" LIMIT $1 OFFSET $2;", limit, offset, chatid, kind)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		m := MsgModel{}
		if err := rows.Scan(&m.Chatid, &m.Msgid, &m.Userid, &m.Timems, &m.Kind, &m.Value); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func msgModelGetMsgModelEqChatidEqKindLtMsgidOrdMsgid(db *sql.DB, chatid string, kind string, msgid string, orderasc bool, limit, offset int) ([]MsgModel, error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]MsgModel, 0, limit)
	rows, err := db.Query("SELECT chatid, msgid, userid, time_ms, kind, value FROM chatmessages WHERE chatid = $3 AND kind = $4 AND msgid < $5 ORDER BY msgid "+order+" LIMIT $1 OFFSET $2;", limit, offset, chatid, kind, msgid)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		m := MsgModel{}
		if err := rows.Scan(&m.Chatid, &m.Msgid, &m.Userid, &m.Timems, &m.Kind, &m.Value); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func msgModelDelEqChatidHasMsgid(db *sql.DB, chatid string, msgid []string) error {
	paramCount := 1
	args := make([]interface{}, 0, paramCount+len(msgid))
	args = append(args, chatid)
	var placeholdersmsgid string
	{
		placeholders := make([]string, 0, len(msgid))
		for _, i := range msgid {
			paramCount++
			placeholders = append(placeholders, fmt.Sprintf("($%d)", paramCount))
			args = append(args, i)
		}
		placeholdersmsgid = strings.Join(placeholders, ", ")
	}
	_, err := db.Exec("DELETE FROM chatmessages WHERE chatid = $1 AND msgid IN (VALUES "+placeholdersmsgid+");", args...)
	return err
}

func msgModelDelEqChatid(db *sql.DB, chatid string) error {
	_, err := db.Exec("DELETE FROM chatmessages WHERE chatid = $1;", chatid)
	return err
}
