// Code generated by go generate forge model v0.3; DO NOT EDIT.

package model

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/lib/pq"
)

const (
	treeModelTableName = "mailinglisttree"
)

func treeModelSetup(db *sql.DB) (int, error) {
	_, err := db.Exec("CREATE TABLE IF NOT EXISTS mailinglisttree (listid VARCHAR(255), msgid VARCHAR(1023), parent_id VARCHAR(1023), PRIMARY KEY (listid, msgid, parent_id), depth INT NOT NULL, creation_time BIGINT NOT NULL);")
	if err != nil {
		return 0, err
	}
	_, err = db.Exec("CREATE INDEX IF NOT EXISTS mailinglisttree_listid__msgid__depth_index ON mailinglisttree (listid, msgid, depth);")
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
	_, err = db.Exec("CREATE INDEX IF NOT EXISTS mailinglisttree_listid__parent_id__depth__creation_time_index ON mailinglisttree (listid, parent_id, depth, creation_time);")
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

func treeModelInsert(db *sql.DB, m *TreeModel) (int, error) {
	_, err := db.Exec("INSERT INTO mailinglisttree (listid, msgid, parent_id, depth, creation_time) VALUES ($1, $2, $3, $4, $5);", m.ListID, m.Msgid, m.ParentID, m.Depth, m.CreationTime)
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

func treeModelInsertBulk(db *sql.DB, models []*TreeModel, allowConflict bool) (int, error) {
	conflictSQL := ""
	if allowConflict {
		conflictSQL = " ON CONFLICT DO NOTHING"
	}
	placeholders := make([]string, 0, len(models))
	args := make([]interface{}, 0, len(models)*5)
	for c, m := range models {
		n := c * 5
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d)", n+1, n+2, n+3, n+4, n+5))
		args = append(args, m.ListID, m.Msgid, m.ParentID, m.Depth, m.CreationTime)
	}
	_, err := db.Exec("INSERT INTO mailinglisttree (listid, msgid, parent_id, depth, creation_time) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
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

func treeModelDelEqListID(db *sql.DB, listid string) error {
	_, err := db.Exec("DELETE FROM mailinglisttree WHERE listid = $1;", listid)
	return err
}

func treeModelGetTreeModelEqListIDEqMsgidEqParentID(db *sql.DB, listid string, msgid string, parentid string) (*TreeModel, int, error) {
	m := &TreeModel{}
	if err := db.QueryRow("SELECT listid, msgid, parent_id, depth, creation_time FROM mailinglisttree WHERE listid = $1 AND msgid = $2 AND parent_id = $3;", listid, msgid, parentid).Scan(&m.ListID, &m.Msgid, &m.ParentID, &m.Depth, &m.CreationTime); err != nil {
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

func treeModelGetTreeModelEqListIDEqMsgidOrdDepth(db *sql.DB, listid string, msgid string, orderasc bool, limit, offset int) ([]TreeModel, error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]TreeModel, 0, limit)
	rows, err := db.Query("SELECT listid, msgid, parent_id, depth, creation_time FROM mailinglisttree WHERE listid = $3 AND msgid = $4 ORDER BY depth "+order+" LIMIT $1 OFFSET $2;", limit, offset, listid, msgid)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		m := TreeModel{}
		if err := rows.Scan(&m.ListID, &m.Msgid, &m.ParentID, &m.Depth, &m.CreationTime); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func treeModelGetTreeModelEqListIDEqParentIDEqDepthOrdCreationTime(db *sql.DB, listid string, parentid string, depth int, orderasc bool, limit, offset int) ([]TreeModel, error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]TreeModel, 0, limit)
	rows, err := db.Query("SELECT listid, msgid, parent_id, depth, creation_time FROM mailinglisttree WHERE listid = $3 AND parent_id = $4 AND depth = $5 ORDER BY creation_time "+order+" LIMIT $1 OFFSET $2;", limit, offset, listid, parentid, depth)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		m := TreeModel{}
		if err := rows.Scan(&m.ListID, &m.Msgid, &m.ParentID, &m.Depth, &m.CreationTime); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}
