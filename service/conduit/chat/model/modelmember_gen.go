// Code generated by go generate forge model v0.3; DO NOT EDIT.

package model

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/lib/pq"
)

const (
	memberModelTableName = "chatmembers"
)

func memberModelSetup(db *sql.DB) (int, error) {
	_, err := db.Exec("CREATE TABLE IF NOT EXISTS chatmembers (chatid VARCHAR(31), userid VARCHAR(31), PRIMARY KEY (chatid, userid));")
	if err != nil {
		return 0, err
	}
	_, err = db.Exec("CREATE INDEX IF NOT EXISTS chatmembers_userid__chatid_index ON chatmembers (userid, chatid);")
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

func memberModelInsert(db *sql.DB, m *MemberModel) (int, error) {
	_, err := db.Exec("INSERT INTO chatmembers (chatid, userid) VALUES ($1, $2);", m.Chatid, m.Userid)
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

func memberModelInsertBulk(db *sql.DB, models []*MemberModel, allowConflict bool) (int, error) {
	conflictSQL := ""
	if allowConflict {
		conflictSQL = " ON CONFLICT DO NOTHING"
	}
	placeholders := make([]string, 0, len(models))
	args := make([]interface{}, 0, len(models)*2)
	for c, m := range models {
		n := c * 2
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d)", n+1, n+2))
		args = append(args, m.Chatid, m.Userid)
	}
	_, err := db.Exec("INSERT INTO chatmembers (chatid, userid) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
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

func memberModelGetMemberModelEqChatidOrdUserid(db *sql.DB, chatid string, orderasc bool, limit, offset int) ([]MemberModel, error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]MemberModel, 0, limit)
	rows, err := db.Query("SELECT chatid, userid FROM chatmembers WHERE chatid = $3 ORDER BY userid "+order+" LIMIT $1 OFFSET $2;", limit, offset, chatid)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		m := MemberModel{}
		if err := rows.Scan(&m.Chatid, &m.Userid); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func memberModelDelEqChatidEqUserid(db *sql.DB, chatid string, userid string) error {
	_, err := db.Exec("DELETE FROM chatmembers WHERE chatid = $1 AND userid = $2;", chatid, userid)
	return err
}
