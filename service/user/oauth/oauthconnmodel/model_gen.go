// Code generated by go generate forge model v0.4.1; DO NOT EDIT.

package oauthconnmodel

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"xorkevin.dev/governor/service/db"
)

type (
	connectionModelTable struct {
		TableName string
	}
)

func (t *connectionModelTable) Setup(ctx context.Context, d db.SQLExecutor) error {
	_, err := d.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS "+t.TableName+" (userid VARCHAR(31), clientid VARCHAR(31), PRIMARY KEY (userid, clientid), scope VARCHAR(4095) NOT NULL, nonce VARCHAR(255), challenge VARCHAR(128), challenge_method VARCHAR(31), codehash VARCHAR(255) NOT NULL, auth_time BIGINT NOT NULL, code_time BIGINT NOT NULL, access_time BIGINT NOT NULL, creation_time BIGINT NOT NULL, keyhash VARCHAR(255) NOT NULL);")
	if err != nil {
		return err
	}
	_, err = d.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS "+t.TableName+"_clientid_index ON "+t.TableName+" (clientid);")
	if err != nil {
		return err
	}
	_, err = d.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS "+t.TableName+"_userid__access_time_index ON "+t.TableName+" (userid, access_time);")
	if err != nil {
		return err
	}
	return nil
}

func (t *connectionModelTable) Insert(ctx context.Context, d db.SQLExecutor, m *Model) error {
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (userid, clientid, scope, nonce, challenge, challenge_method, codehash, auth_time, code_time, access_time, creation_time, keyhash) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12);", m.Userid, m.ClientID, m.Scope, m.Nonce, m.Challenge, m.ChallengeMethod, m.CodeHash, m.AuthTime, m.CodeTime, m.AccessTime, m.CreationTime, m.KeyHash)
	if err != nil {
		return err
	}
	return nil
}

func (t *connectionModelTable) InsertBulk(ctx context.Context, d db.SQLExecutor, models []*Model, allowConflict bool) error {
	conflictSQL := ""
	if allowConflict {
		conflictSQL = " ON CONFLICT DO NOTHING"
	}
	placeholders := make([]string, 0, len(models))
	args := make([]interface{}, 0, len(models)*12)
	for c, m := range models {
		n := c * 12
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)", n+1, n+2, n+3, n+4, n+5, n+6, n+7, n+8, n+9, n+10, n+11, n+12))
		args = append(args, m.Userid, m.ClientID, m.Scope, m.Nonce, m.Challenge, m.ChallengeMethod, m.CodeHash, m.AuthTime, m.CodeTime, m.AccessTime, m.CreationTime, m.KeyHash)
	}
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (userid, clientid, scope, nonce, challenge, challenge_method, codehash, auth_time, code_time, access_time, creation_time, keyhash) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
	if err != nil {
		return err
	}
	return nil
}

func (t *connectionModelTable) DelEqUserid(ctx context.Context, d db.SQLExecutor, userid string) error {
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE userid = $1;", userid)
	return err
}

func (t *connectionModelTable) GetModelEqUseridEqClientID(ctx context.Context, d db.SQLExecutor, userid string, clientid string) (*Model, error) {
	m := &Model{}
	if err := d.QueryRowContext(ctx, "SELECT userid, clientid, scope, nonce, challenge, challenge_method, codehash, auth_time, code_time, access_time, creation_time, keyhash FROM "+t.TableName+" WHERE userid = $1 AND clientid = $2;", userid, clientid).Scan(&m.Userid, &m.ClientID, &m.Scope, &m.Nonce, &m.Challenge, &m.ChallengeMethod, &m.CodeHash, &m.AuthTime, &m.CodeTime, &m.AccessTime, &m.CreationTime, &m.KeyHash); err != nil {
		return nil, err
	}
	return m, nil
}

func (t *connectionModelTable) UpdModelEqUseridEqClientID(ctx context.Context, d db.SQLExecutor, m *Model, userid string, clientid string) error {
	_, err := d.ExecContext(ctx, "UPDATE "+t.TableName+" SET (userid, clientid, scope, nonce, challenge, challenge_method, codehash, auth_time, code_time, access_time, creation_time, keyhash) = ROW($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) WHERE userid = $13 AND clientid = $14;", m.Userid, m.ClientID, m.Scope, m.Nonce, m.Challenge, m.ChallengeMethod, m.CodeHash, m.AuthTime, m.CodeTime, m.AccessTime, m.CreationTime, m.KeyHash, userid, clientid)
	if err != nil {
		return err
	}
	return nil
}

func (t *connectionModelTable) DelEqUseridHasClientID(ctx context.Context, d db.SQLExecutor, userid string, clientids []string) error {
	paramCount := 1
	args := make([]interface{}, 0, paramCount+len(clientids))
	args = append(args, userid)
	var placeholdersclientids string
	{
		placeholders := make([]string, 0, len(clientids))
		for _, i := range clientids {
			paramCount++
			placeholders = append(placeholders, fmt.Sprintf("($%d)", paramCount))
			args = append(args, i)
		}
		placeholdersclientids = strings.Join(placeholders, ", ")
	}
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE userid = $1 AND clientid IN (VALUES "+placeholdersclientids+");", args...)
	return err
}

func (t *connectionModelTable) GetModelEqUseridOrdAccessTime(ctx context.Context, d db.SQLExecutor, userid string, orderasc bool, limit, offset int) (_ []Model, retErr error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]Model, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT userid, clientid, scope, nonce, challenge, challenge_method, codehash, auth_time, code_time, access_time, creation_time, keyhash FROM "+t.TableName+" WHERE userid = $3 ORDER BY access_time "+order+" LIMIT $1 OFFSET $2;", limit, offset, userid)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("Failed to close db rows: %w", err))
		}
	}()
	for rows.Next() {
		var m Model
		if err := rows.Scan(&m.Userid, &m.ClientID, &m.Scope, &m.Nonce, &m.Challenge, &m.ChallengeMethod, &m.CodeHash, &m.AuthTime, &m.CodeTime, &m.AccessTime, &m.CreationTime, &m.KeyHash); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}
