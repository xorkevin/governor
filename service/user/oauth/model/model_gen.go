// Code generated by go generate forge model v0.3; DO NOT EDIT.

package model

import (
	"context"
	"fmt"
	"strings"

	"xorkevin.dev/governor/service/db"
)

type (
	oauthappModelTable struct {
		TableName string
	}
)

func (t *oauthappModelTable) Setup(ctx context.Context, d db.SQLExecutor) error {
	_, err := d.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS "+t.TableName+" (clientid VARCHAR(31) PRIMARY KEY, name VARCHAR(255) NOT NULL, url VARCHAR(512) NOT NULL, redirect_uri VARCHAR(512) NOT NULL, logo VARCHAR(4095), keyhash VARCHAR(255) NOT NULL, time BIGINT NOT NULL, creation_time BIGINT NOT NULL, creator_id VARCHAR(31) NOT NULL);")
	if err != nil {
		return err
	}
	_, err = d.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS "+t.TableName+"_creation_time_index ON "+t.TableName+" (creation_time);")
	if err != nil {
		return err
	}
	_, err = d.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS "+t.TableName+"_creator_id__creation_time_index ON "+t.TableName+" (creator_id, creation_time);")
	if err != nil {
		return err
	}
	return nil
}

func (t *oauthappModelTable) Insert(ctx context.Context, d db.SQLExecutor, m *Model) error {
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (clientid, name, url, redirect_uri, logo, keyhash, time, creation_time, creator_id) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);", m.ClientID, m.Name, m.URL, m.RedirectURI, m.Logo, m.KeyHash, m.Time, m.CreationTime, m.CreatorID)
	if err != nil {
		return err
	}
	return nil
}

func (t *oauthappModelTable) InsertBulk(ctx context.Context, d db.SQLExecutor, models []*Model, allowConflict bool) error {
	conflictSQL := ""
	if allowConflict {
		conflictSQL = " ON CONFLICT DO NOTHING"
	}
	placeholders := make([]string, 0, len(models))
	args := make([]interface{}, 0, len(models)*9)
	for c, m := range models {
		n := c * 9
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)", n+1, n+2, n+3, n+4, n+5, n+6, n+7, n+8, n+9))
		args = append(args, m.ClientID, m.Name, m.URL, m.RedirectURI, m.Logo, m.KeyHash, m.Time, m.CreationTime, m.CreatorID)
	}
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (clientid, name, url, redirect_uri, logo, keyhash, time, creation_time, creator_id) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
	if err != nil {
		return err
	}
	return nil
}

func (t *oauthappModelTable) GetModelEqClientID(ctx context.Context, d db.SQLExecutor, clientid string) (*Model, error) {
	m := &Model{}
	if err := d.QueryRowContext(ctx, "SELECT clientid, name, url, redirect_uri, logo, keyhash, time, creation_time, creator_id FROM "+t.TableName+" WHERE clientid = $1;", clientid).Scan(&m.ClientID, &m.Name, &m.URL, &m.RedirectURI, &m.Logo, &m.KeyHash, &m.Time, &m.CreationTime, &m.CreatorID); err != nil {
		return nil, err
	}
	return m, nil
}

func (t *oauthappModelTable) GetModelHasClientIDOrdClientID(ctx context.Context, d db.SQLExecutor, clientid []string, orderasc bool, limit, offset int) ([]Model, error) {
	paramCount := 2
	args := make([]interface{}, 0, paramCount+len(clientid))
	args = append(args, limit, offset)
	var placeholdersclientid string
	{
		placeholders := make([]string, 0, len(clientid))
		for _, i := range clientid {
			paramCount++
			placeholders = append(placeholders, fmt.Sprintf("($%d)", paramCount))
			args = append(args, i)
		}
		placeholdersclientid = strings.Join(placeholders, ", ")
	}
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]Model, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT clientid, name, url, redirect_uri, logo, keyhash, time, creation_time, creator_id FROM "+t.TableName+" WHERE clientid IN (VALUES "+placeholdersclientid+") ORDER BY clientid "+order+" LIMIT $1 OFFSET $2;", args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		m := Model{}
		if err := rows.Scan(&m.ClientID, &m.Name, &m.URL, &m.RedirectURI, &m.Logo, &m.KeyHash, &m.Time, &m.CreationTime, &m.CreatorID); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (t *oauthappModelTable) UpdModelEqClientID(ctx context.Context, d db.SQLExecutor, m *Model, clientid string) error {
	_, err := d.ExecContext(ctx, "UPDATE "+t.TableName+" SET (clientid, name, url, redirect_uri, logo, keyhash, time, creation_time, creator_id) = ROW($1, $2, $3, $4, $5, $6, $7, $8, $9) WHERE clientid = $10;", m.ClientID, m.Name, m.URL, m.RedirectURI, m.Logo, m.KeyHash, m.Time, m.CreationTime, m.CreatorID, clientid)
	if err != nil {
		return err
	}
	return nil
}

func (t *oauthappModelTable) DelEqClientID(ctx context.Context, d db.SQLExecutor, clientid string) error {
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE clientid = $1;", clientid)
	return err
}

func (t *oauthappModelTable) GetModelOrdCreationTime(ctx context.Context, d db.SQLExecutor, orderasc bool, limit, offset int) ([]Model, error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]Model, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT clientid, name, url, redirect_uri, logo, keyhash, time, creation_time, creator_id FROM "+t.TableName+" ORDER BY creation_time "+order+" LIMIT $1 OFFSET $2;", limit, offset)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		m := Model{}
		if err := rows.Scan(&m.ClientID, &m.Name, &m.URL, &m.RedirectURI, &m.Logo, &m.KeyHash, &m.Time, &m.CreationTime, &m.CreatorID); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (t *oauthappModelTable) GetModelEqCreatorIDOrdCreationTime(ctx context.Context, d db.SQLExecutor, creatorid string, orderasc bool, limit, offset int) ([]Model, error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]Model, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT clientid, name, url, redirect_uri, logo, keyhash, time, creation_time, creator_id FROM "+t.TableName+" WHERE creator_id = $3 ORDER BY creation_time "+order+" LIMIT $1 OFFSET $2;", limit, offset, creatorid)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		m := Model{}
		if err := rows.Scan(&m.ClientID, &m.Name, &m.URL, &m.RedirectURI, &m.Logo, &m.KeyHash, &m.Time, &m.CreationTime, &m.CreatorID); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (t *oauthappModelTable) DelEqCreatorID(ctx context.Context, d db.SQLExecutor, creatorid string) error {
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE creator_id = $1;", creatorid)
	return err
}
