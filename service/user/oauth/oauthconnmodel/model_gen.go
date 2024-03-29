// Code generated by go generate forge model v0.5.2; DO NOT EDIT.

package oauthconnmodel

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"xorkevin.dev/forge/model/sqldb"
)

type (
	connModelTable struct {
		TableName string
	}
)

func (t *connModelTable) Setup(ctx context.Context, d sqldb.Executor) error {
	_, err := d.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS "+t.TableName+" (userid VARCHAR(31), clientid VARCHAR(31), scope VARCHAR(4095) NOT NULL, nonce VARCHAR(255), challenge VARCHAR(128), challenge_method VARCHAR(31), codehash VARCHAR(255) NOT NULL, auth_time BIGINT NOT NULL, code_time BIGINT NOT NULL, access_time BIGINT NOT NULL, creation_time BIGINT NOT NULL, keyhash VARCHAR(255) NOT NULL, PRIMARY KEY (userid, clientid));")
	if err != nil {
		return err
	}
	_, err = d.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS "+t.TableName+"_clientid_index ON "+t.TableName+" (clientid);")
	if err != nil {
		return err
	}
	_, err = d.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS "+t.TableName+"_userid_access_time_index ON "+t.TableName+" (userid, access_time);")
	if err != nil {
		return err
	}
	return nil
}

func (t *connModelTable) Insert(ctx context.Context, d sqldb.Executor, m *Model) error {
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (userid, clientid, scope, nonce, challenge, challenge_method, codehash, auth_time, code_time, access_time, creation_time, keyhash) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12);", m.Userid, m.ClientID, m.Scope, m.Nonce, m.Challenge, m.ChallengeMethod, m.CodeHash, m.AuthTime, m.CodeTime, m.AccessTime, m.CreationTime, m.KeyHash)
	if err != nil {
		return err
	}
	return nil
}

func (t *connModelTable) InsertBulk(ctx context.Context, d sqldb.Executor, models []*Model, allowConflict bool) error {
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

func (t *connModelTable) DelByUserid(ctx context.Context, d sqldb.Executor, userid string) error {
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE userid = $1;", userid)
	return err
}

func (t *connModelTable) GetModelByUserClient(ctx context.Context, d sqldb.Executor, userid string, clientid string) (*Model, error) {
	m := &Model{}
	if err := d.QueryRowContext(ctx, "SELECT userid, clientid, scope, nonce, challenge, challenge_method, codehash, auth_time, code_time, access_time, creation_time, keyhash FROM "+t.TableName+" WHERE userid = $1 AND clientid = $2;", userid, clientid).Scan(&m.Userid, &m.ClientID, &m.Scope, &m.Nonce, &m.Challenge, &m.ChallengeMethod, &m.CodeHash, &m.AuthTime, &m.CodeTime, &m.AccessTime, &m.CreationTime, &m.KeyHash); err != nil {
		return nil, err
	}
	return m, nil
}

func (t *connModelTable) UpdModelByUserClient(ctx context.Context, d sqldb.Executor, m *Model, userid string, clientid string) error {
	_, err := d.ExecContext(ctx, "UPDATE "+t.TableName+" SET (userid, clientid, scope, nonce, challenge, challenge_method, codehash, auth_time, code_time, access_time, creation_time, keyhash) = ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) WHERE userid = $13 AND clientid = $14;", m.Userid, m.ClientID, m.Scope, m.Nonce, m.Challenge, m.ChallengeMethod, m.CodeHash, m.AuthTime, m.CodeTime, m.AccessTime, m.CreationTime, m.KeyHash, userid, clientid)
	if err != nil {
		return err
	}
	return nil
}

func (t *connModelTable) DelByUserClients(ctx context.Context, d sqldb.Executor, userid string, clientids []string) error {
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

func (t *connModelTable) GetModelByUserid(ctx context.Context, d sqldb.Executor, userid string, limit, offset int) (_ []Model, retErr error) {
	res := make([]Model, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT userid, clientid, scope, nonce, challenge, challenge_method, codehash, auth_time, code_time, access_time, creation_time, keyhash FROM "+t.TableName+" WHERE userid = $3 ORDER BY access_time DESC LIMIT $1 OFFSET $2;", limit, offset, userid)
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
