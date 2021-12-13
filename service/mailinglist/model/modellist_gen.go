// Code generated by go generate forge model v0.3; DO NOT EDIT.

package model

import (
	"database/sql"
	"fmt"
	"strings"
)

func listModelSetup(db *sql.DB, tableName string) error {
	_, err := db.Exec("CREATE TABLE IF NOT EXISTS " + tableName + " (listid VARCHAR(255) PRIMARY KEY, creatorid VARCHAR(31) NOT NULL, listname VARCHAR(127) NOT NULL, name VARCHAR(255) NOT NULL, description VARCHAR(255), archive BOOLEAN NOT NULL, sender_policy VARCHAR(255) NOT NULL, member_policy VARCHAR(255) NOT NULL, last_updated BIGINT NOT NULL, creation_time BIGINT NOT NULL);")
	if err != nil {
		return err
	}
	_, err = db.Exec("CREATE INDEX IF NOT EXISTS " + tableName + "_creatorid__last_updated_index ON " + tableName + " (creatorid, last_updated);")
	if err != nil {
		return err
	}
	return nil
}

func listModelInsert(db *sql.DB, tableName string, m *ListModel) error {
	_, err := db.Exec("INSERT INTO "+tableName+" (listid, creatorid, listname, name, description, archive, sender_policy, member_policy, last_updated, creation_time) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);", m.ListID, m.CreatorID, m.Listname, m.Name, m.Description, m.Archive, m.SenderPolicy, m.MemberPolicy, m.LastUpdated, m.CreationTime)
	if err != nil {
		return err
	}
	return nil
}

func listModelInsertBulk(db *sql.DB, tableName string, models []*ListModel, allowConflict bool) error {
	conflictSQL := ""
	if allowConflict {
		conflictSQL = " ON CONFLICT DO NOTHING"
	}
	placeholders := make([]string, 0, len(models))
	args := make([]interface{}, 0, len(models)*10)
	for c, m := range models {
		n := c * 10
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)", n+1, n+2, n+3, n+4, n+5, n+6, n+7, n+8, n+9, n+10))
		args = append(args, m.ListID, m.CreatorID, m.Listname, m.Name, m.Description, m.Archive, m.SenderPolicy, m.MemberPolicy, m.LastUpdated, m.CreationTime)
	}
	_, err := db.Exec("INSERT INTO "+tableName+" (listid, creatorid, listname, name, description, archive, sender_policy, member_policy, last_updated, creation_time) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
	if err != nil {
		return err
	}
	return nil
}

func listModelGetListModelEqListID(db *sql.DB, tableName string, listid string) (*ListModel, error) {
	m := &ListModel{}
	if err := db.QueryRow("SELECT listid, creatorid, listname, name, description, archive, sender_policy, member_policy, last_updated, creation_time FROM "+tableName+" WHERE listid = $1;", listid).Scan(&m.ListID, &m.CreatorID, &m.Listname, &m.Name, &m.Description, &m.Archive, &m.SenderPolicy, &m.MemberPolicy, &m.LastUpdated, &m.CreationTime); err != nil {
		return nil, err
	}
	return m, nil
}

func listModelGetListModelHasListIDOrdListID(db *sql.DB, tableName string, listid []string, orderasc bool, limit, offset int) ([]ListModel, error) {
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
	res := make([]ListModel, 0, limit)
	rows, err := db.Query("SELECT listid, creatorid, listname, name, description, archive, sender_policy, member_policy, last_updated, creation_time FROM "+tableName+" WHERE listid IN (VALUES "+placeholderslistid+") ORDER BY listid "+order+" LIMIT $1 OFFSET $2;", args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		m := ListModel{}
		if err := rows.Scan(&m.ListID, &m.CreatorID, &m.Listname, &m.Name, &m.Description, &m.Archive, &m.SenderPolicy, &m.MemberPolicy, &m.LastUpdated, &m.CreationTime); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func listModelUpdListModelEqListID(db *sql.DB, tableName string, m *ListModel, listid string) error {
	_, err := db.Exec("UPDATE "+tableName+" SET (listid, creatorid, listname, name, description, archive, sender_policy, member_policy, last_updated, creation_time) = ROW($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) WHERE listid = $11;", m.ListID, m.CreatorID, m.Listname, m.Name, m.Description, m.Archive, m.SenderPolicy, m.MemberPolicy, m.LastUpdated, m.CreationTime, listid)
	if err != nil {
		return err
	}
	return nil
}

func listModelDelEqListID(db *sql.DB, tableName string, listid string) error {
	_, err := db.Exec("DELETE FROM "+tableName+" WHERE listid = $1;", listid)
	return err
}

func listModelDelEqCreatorID(db *sql.DB, tableName string, creatorid string) error {
	_, err := db.Exec("DELETE FROM "+tableName+" WHERE creatorid = $1;", creatorid)
	return err
}

func listModelGetListModelEqCreatorIDOrdLastUpdated(db *sql.DB, tableName string, creatorid string, orderasc bool, limit, offset int) ([]ListModel, error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]ListModel, 0, limit)
	rows, err := db.Query("SELECT listid, creatorid, listname, name, description, archive, sender_policy, member_policy, last_updated, creation_time FROM "+tableName+" WHERE creatorid = $3 ORDER BY last_updated "+order+" LIMIT $1 OFFSET $2;", limit, offset, creatorid)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		m := ListModel{}
		if err := rows.Scan(&m.ListID, &m.CreatorID, &m.Listname, &m.Name, &m.Description, &m.Archive, &m.SenderPolicy, &m.MemberPolicy, &m.LastUpdated, &m.CreationTime); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func listModelUpdlistLastUpdatedEqListID(db *sql.DB, tableName string, m *listLastUpdated, listid string) error {
	_, err := db.Exec("UPDATE "+tableName+" SET (last_updated) = ROW($1) WHERE listid = $2;", m.LastUpdated, listid)
	if err != nil {
		return err
	}
	return nil
}
