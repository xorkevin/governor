// Code generated by go generate forge model v0.3; DO NOT EDIT.

package model

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/lib/pq"
)

const (
	msgModelTableName = "mailinglistmsgs"
)

func msgModelSetup(db *sql.DB) (int, error) {
	_, err := db.Exec("CREATE TABLE IF NOT EXISTS mailinglistmsgs (listid VARCHAR(255), msgid VARCHAR(1023), PRIMARY KEY (listid, msgid), userid VARCHAR(31) NOT NULL, creation_time BIGINT NOT NULL, spf_pass VARCHAR(255) NOT NULL, dkim_pass VARCHAR(255) NOT NULL, subject VARCHAR(255) NOT NULL);")
	if err != nil {
		return 0, err
	}
	_, err = db.Exec("CREATE INDEX IF NOT EXISTS mailinglistmsgs_listid__creation_time_index ON mailinglistmsgs (listid, creation_time);")
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
	_, err := db.Exec("INSERT INTO mailinglistmsgs (listid, msgid, userid, creation_time, spf_pass, dkim_pass, subject) VALUES ($1, $2, $3, $4, $5, $6, $7);", m.ListID, m.Msgid, m.Userid, m.CreationTime, m.SPFPass, m.DKIMPass, m.Subject)
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
	args := make([]interface{}, 0, len(models)*7)
	for c, m := range models {
		n := c * 7
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d)", n+1, n+2, n+3, n+4, n+5, n+6, n+7))
		args = append(args, m.ListID, m.Msgid, m.Userid, m.CreationTime, m.SPFPass, m.DKIMPass, m.Subject)
	}
	_, err := db.Exec("INSERT INTO mailinglistmsgs (listid, msgid, userid, creation_time, spf_pass, dkim_pass, subject) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
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

func msgModelDelEqListID(db *sql.DB, listid string) error {
	_, err := db.Exec("DELETE FROM mailinglistmsgs WHERE listid = $1;", listid)
	return err
}

func msgModelGetMsgModelEqListIDEqMsgid(db *sql.DB, listid string, msgid string) (*MsgModel, int, error) {
	m := &MsgModel{}
	if err := db.QueryRow("SELECT listid, msgid, userid, creation_time, spf_pass, dkim_pass, subject FROM mailinglistmsgs WHERE listid = $1 AND msgid = $2;", listid, msgid).Scan(&m.ListID, &m.Msgid, &m.Userid, &m.CreationTime, &m.SPFPass, &m.DKIMPass, &m.Subject); err != nil {
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

func msgModelDelEqListIDHasMsgid(db *sql.DB, listid string, msgid []string) error {
	paramCount := 1
	args := make([]interface{}, 0, paramCount+len(msgid))
	args = append(args, listid)
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
	_, err := db.Exec("DELETE FROM mailinglistmsgs WHERE listid = $1 AND msgid IN (VALUES "+placeholdersmsgid+");", args...)
	return err
}

func msgModelGetMsgModelEqListIDOrdCreationTime(db *sql.DB, listid string, orderasc bool, limit, offset int) ([]MsgModel, error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]MsgModel, 0, limit)
	rows, err := db.Query("SELECT listid, msgid, userid, creation_time, spf_pass, dkim_pass, subject FROM mailinglistmsgs WHERE listid = $3 ORDER BY creation_time "+order+" LIMIT $1 OFFSET $2;", limit, offset, listid)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		m := MsgModel{}
		if err := rows.Scan(&m.ListID, &m.Msgid, &m.Userid, &m.CreationTime, &m.SPFPass, &m.DKIMPass, &m.Subject); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}
