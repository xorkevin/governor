// Code generated by go generate forge model v0.5.2; DO NOT EDIT.

package couriermodel

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"xorkevin.dev/forge/model/sqldb"
)

type (
	linkModelTable struct {
		TableName string
	}
)

func (t *linkModelTable) Setup(ctx context.Context, d sqldb.Executor) error {
	_, err := d.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS "+t.TableName+" (linkid VARCHAR(63) PRIMARY KEY, url VARCHAR(2047) NOT NULL, creatorid VARCHAR(31) NOT NULL, creation_time BIGINT NOT NULL);")
	if err != nil {
		return err
	}
	_, err = d.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS "+t.TableName+"_creator_creation_time_index ON "+t.TableName+" (creatorid, creation_time);")
	if err != nil {
		return err
	}
	return nil
}

func (t *linkModelTable) Insert(ctx context.Context, d sqldb.Executor, m *LinkModel) error {
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (linkid, url, creatorid, creation_time) VALUES ($1, $2, $3, $4);", m.LinkID, m.URL, m.CreatorID, m.CreationTime)
	if err != nil {
		return err
	}
	return nil
}

func (t *linkModelTable) InsertBulk(ctx context.Context, d sqldb.Executor, models []*LinkModel, allowConflict bool) error {
	conflictSQL := ""
	if allowConflict {
		conflictSQL = " ON CONFLICT DO NOTHING"
	}
	placeholders := make([]string, 0, len(models))
	args := make([]interface{}, 0, len(models)*4)
	for c, m := range models {
		n := c * 4
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d)", n+1, n+2, n+3, n+4))
		args = append(args, m.LinkID, m.URL, m.CreatorID, m.CreationTime)
	}
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (linkid, url, creatorid, creation_time) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
	if err != nil {
		return err
	}
	return nil
}

func (t *linkModelTable) GetLinkModelByID(ctx context.Context, d sqldb.Executor, linkid string) (*LinkModel, error) {
	m := &LinkModel{}
	if err := d.QueryRowContext(ctx, "SELECT linkid, url, creatorid, creation_time FROM "+t.TableName+" WHERE linkid = $1;", linkid).Scan(&m.LinkID, &m.URL, &m.CreatorID, &m.CreationTime); err != nil {
		return nil, err
	}
	return m, nil
}

func (t *linkModelTable) DelByID(ctx context.Context, d sqldb.Executor, linkid string) error {
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE linkid = $1;", linkid)
	return err
}

func (t *linkModelTable) DelByIDs(ctx context.Context, d sqldb.Executor, linkids []string) error {
	paramCount := 0
	args := make([]interface{}, 0, paramCount+len(linkids))
	var placeholderslinkids string
	{
		placeholders := make([]string, 0, len(linkids))
		for _, i := range linkids {
			paramCount++
			placeholders = append(placeholders, fmt.Sprintf("($%d)", paramCount))
			args = append(args, i)
		}
		placeholderslinkids = strings.Join(placeholders, ", ")
	}
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE linkid IN (VALUES "+placeholderslinkids+");", args...)
	return err
}

func (t *linkModelTable) GetLinkModelByCreator(ctx context.Context, d sqldb.Executor, creatorid string, limit, offset int) (_ []LinkModel, retErr error) {
	res := make([]LinkModel, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT linkid, url, creatorid, creation_time FROM "+t.TableName+" WHERE creatorid = $3 ORDER BY creation_time DESC LIMIT $1 OFFSET $2;", limit, offset, creatorid)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("Failed to close db rows: %w", err))
		}
	}()
	for rows.Next() {
		var m LinkModel
		if err := rows.Scan(&m.LinkID, &m.URL, &m.CreatorID, &m.CreationTime); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}
