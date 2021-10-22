// Code generated by go generate forge model v0.3; DO NOT EDIT.

package model

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/lib/pq"
)

const (
	memberModelTableName = "mailinglistmembers"
)

func memberModelSetup(db *sql.DB) (int, error) {
	_, err := db.Exec("CREATE TABLE IF NOT EXISTS mailinglistmembers (listid VARCHAR(255), userid VARCHAR(31), PRIMARY KEY (listid, userid), last_updated BIGINT NOT NULL);")
	if err != nil {
		return 0, err
	}
	_, err = db.Exec("CREATE INDEX IF NOT EXISTS mailinglistmembers_userid__last_updated_index ON mailinglistmembers (userid, last_updated);")
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
	_, err := db.Exec("INSERT INTO mailinglistmembers (listid, userid, last_updated) VALUES ($1, $2, $3);", m.ListID, m.Userid, m.LastUpdated)
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
	args := make([]interface{}, 0, len(models)*3)
	for c, m := range models {
		n := c * 3
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d)", n+1, n+2, n+3))
		args = append(args, m.ListID, m.Userid, m.LastUpdated)
	}
	_, err := db.Exec("INSERT INTO mailinglistmembers (listid, userid, last_updated) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
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

func memberModelDelEqListID(db *sql.DB, listid string) error {
	_, err := db.Exec("DELETE FROM mailinglistmembers WHERE listid = $1;", listid)
	return err
}

func memberModelGetMemberModelHasListIDOrdListID(db *sql.DB, listid []string, orderasc bool, limit, offset int) ([]MemberModel, error) {
	paramCount := 2
	args := make([]interface{}, 0, paramCount+len(listid))
	args = append(args, limit, offset)
	var placeholderslistid string
	{
		placeholders := make([]string, 0, len(listid))
		for _, i := range listid {
			paramCount++
			placeholders = append(placeholders, fmt.Sprintf("($%d)", paramCount))
			args = append(args, i)
		}
		placeholderslistid = strings.Join(placeholders, ", ")
	}
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]MemberModel, 0, limit)
	rows, err := db.Query("SELECT listid, userid, last_updated FROM mailinglistmembers WHERE listid IN (VALUES "+placeholderslistid+") ORDER BY listid "+order+" LIMIT $1 OFFSET $2;", args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		m := MemberModel{}
		if err := rows.Scan(&m.ListID, &m.Userid, &m.LastUpdated); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func memberModelGetMemberModelEqListIDEqUserid(db *sql.DB, listid string, userid string) (*MemberModel, int, error) {
	m := &MemberModel{}
	if err := db.QueryRow("SELECT listid, userid, last_updated FROM mailinglistmembers WHERE listid = $1 AND userid = $2;", listid, userid).Scan(&m.ListID, &m.Userid, &m.LastUpdated); err != nil {
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

func memberModelGetMemberModelEqListIDOrdUserid(db *sql.DB, listid string, orderasc bool, limit, offset int) ([]MemberModel, error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]MemberModel, 0, limit)
	rows, err := db.Query("SELECT listid, userid, last_updated FROM mailinglistmembers WHERE listid = $3 ORDER BY userid "+order+" LIMIT $1 OFFSET $2;", limit, offset, listid)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		m := MemberModel{}
		if err := rows.Scan(&m.ListID, &m.Userid, &m.LastUpdated); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func memberModelGetMemberModelEqListIDHasUseridOrdUserid(db *sql.DB, listid string, userid []string, orderasc bool, limit, offset int) ([]MemberModel, error) {
	paramCount := 3
	args := make([]interface{}, 0, paramCount+len(userid))
	args = append(args, limit, offset, listid)
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
	res := make([]MemberModel, 0, limit)
	rows, err := db.Query("SELECT listid, userid, last_updated FROM mailinglistmembers WHERE listid = $3 AND userid IN (VALUES "+placeholdersuserid+") ORDER BY userid "+order+" LIMIT $1 OFFSET $2;", args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		m := MemberModel{}
		if err := rows.Scan(&m.ListID, &m.Userid, &m.LastUpdated); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func memberModelDelEqListIDHasUserid(db *sql.DB, listid string, userid []string) error {
	paramCount := 1
	args := make([]interface{}, 0, paramCount+len(userid))
	args = append(args, listid)
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
	_, err := db.Exec("DELETE FROM mailinglistmembers WHERE listid = $1 AND userid IN (VALUES "+placeholdersuserid+");", args...)
	return err
}

func memberModelDelEqUserid(db *sql.DB, userid string) error {
	_, err := db.Exec("DELETE FROM mailinglistmembers WHERE userid = $1;", userid)
	return err
}

func memberModelGetMemberModelEqUseridOrdLastUpdated(db *sql.DB, userid string, orderasc bool, limit, offset int) ([]MemberModel, error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]MemberModel, 0, limit)
	rows, err := db.Query("SELECT listid, userid, last_updated FROM mailinglistmembers WHERE userid = $3 ORDER BY last_updated "+order+" LIMIT $1 OFFSET $2;", limit, offset, userid)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		m := MemberModel{}
		if err := rows.Scan(&m.ListID, &m.Userid, &m.LastUpdated); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func memberModelUpdlistLastUpdatedEqListID(db *sql.DB, m *listLastUpdated, listid string) (int, error) {
	_, err := db.Exec("UPDATE mailinglistmembers SET (last_updated) = ROW($1) WHERE listid = $2;", m.LastUpdated, listid)
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
