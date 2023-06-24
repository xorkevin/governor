// Code generated by go generate forge model v0.4.4; DO NOT EDIT.

package servermodel

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"xorkevin.dev/forge/model/sqldb"
)

type (
	serverModelTable struct {
		TableName string
	}
)

func (t *serverModelTable) Setup(ctx context.Context, d sqldb.Executor) error {
	_, err := d.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS "+t.TableName+" (serverid VARCHAR(31) PRIMARY KEY, name VARCHAR(255) NOT NULL, desc VARCHAR(255), theme VARCHAR(4095) NOT NULL, creation_time BIGINT NOT NULL);")
	if err != nil {
		return err
	}
	return nil
}

func (t *serverModelTable) Insert(ctx context.Context, d sqldb.Executor, m *Model) error {
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (serverid, name, desc, theme, creation_time) VALUES ($1, $2, $3, $4, $5);", m.ServerID, m.Name, m.Desc, m.Theme, m.CreationTime)
	if err != nil {
		return err
	}
	return nil
}

func (t *serverModelTable) InsertBulk(ctx context.Context, d sqldb.Executor, models []*Model, allowConflict bool) error {
	conflictSQL := ""
	if allowConflict {
		conflictSQL = " ON CONFLICT DO NOTHING"
	}
	placeholders := make([]string, 0, len(models))
	args := make([]interface{}, 0, len(models)*5)
	for c, m := range models {
		n := c * 5
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d)", n+1, n+2, n+3, n+4, n+5))
		args = append(args, m.ServerID, m.Name, m.Desc, m.Theme, m.CreationTime)
	}
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (serverid, name, desc, theme, creation_time) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
	if err != nil {
		return err
	}
	return nil
}

func (t *serverModelTable) GetModelEqServerID(ctx context.Context, d sqldb.Executor, serverid string) (*Model, error) {
	m := &Model{}
	if err := d.QueryRowContext(ctx, "SELECT serverid, name, desc, theme, creation_time FROM "+t.TableName+" WHERE serverid = $1;", serverid).Scan(&m.ServerID, &m.Name, &m.Desc, &m.Theme, &m.CreationTime); err != nil {
		return nil, err
	}
	return m, nil
}

func (t *serverModelTable) DelEqServerID(ctx context.Context, d sqldb.Executor, serverid string) error {
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE serverid = $1;", serverid)
	return err
}

func (t *serverModelTable) UpdserverPropsEqServerID(ctx context.Context, d sqldb.Executor, m *serverProps, serverid string) error {
	_, err := d.ExecContext(ctx, "UPDATE "+t.TableName+" SET (name, desc, theme) = ($1, $2, $3) WHERE serverid = $4;", m.Name, m.Desc, m.Theme, serverid)
	if err != nil {
		return err
	}
	return nil
}

type (
	channelModelTable struct {
		TableName string
	}
)

func (t *channelModelTable) Setup(ctx context.Context, d sqldb.Executor) error {
	_, err := d.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS "+t.TableName+" (serverid VARCHAR(31), channelid VARCHAR(31), PRIMARY KEY (serverid, channelid), chatid VARCHAR(31) UNIQUE, name VARCHAR(255) NOT NULL, desc VARCHAR(255), theme VARCHAR(4095) NOT NULL, creation_time BIGINT NOT NULL);")
	if err != nil {
		return err
	}
	return nil
}

func (t *channelModelTable) Insert(ctx context.Context, d sqldb.Executor, m *ChannelModel) error {
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (serverid, channelid, chatid, name, desc, theme, creation_time) VALUES ($1, $2, $3, $4, $5, $6, $7);", m.ServerID, m.ChannelID, m.Chatid, m.Name, m.Desc, m.Theme, m.CreationTime)
	if err != nil {
		return err
	}
	return nil
}

func (t *channelModelTable) InsertBulk(ctx context.Context, d sqldb.Executor, models []*ChannelModel, allowConflict bool) error {
	conflictSQL := ""
	if allowConflict {
		conflictSQL = " ON CONFLICT DO NOTHING"
	}
	placeholders := make([]string, 0, len(models))
	args := make([]interface{}, 0, len(models)*7)
	for c, m := range models {
		n := c * 7
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d)", n+1, n+2, n+3, n+4, n+5, n+6, n+7))
		args = append(args, m.ServerID, m.ChannelID, m.Chatid, m.Name, m.Desc, m.Theme, m.CreationTime)
	}
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (serverid, channelid, chatid, name, desc, theme, creation_time) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
	if err != nil {
		return err
	}
	return nil
}

func (t *channelModelTable) DelEqServerID(ctx context.Context, d sqldb.Executor, serverid string) error {
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE serverid = $1;", serverid)
	return err
}

func (t *channelModelTable) GetChannelModelEqServerIDEqChannelID(ctx context.Context, d sqldb.Executor, serverid string, channelid string) (*ChannelModel, error) {
	m := &ChannelModel{}
	if err := d.QueryRowContext(ctx, "SELECT serverid, channelid, chatid, name, desc, theme, creation_time FROM "+t.TableName+" WHERE serverid = $1 AND channelid = $2;", serverid, channelid).Scan(&m.ServerID, &m.ChannelID, &m.Chatid, &m.Name, &m.Desc, &m.Theme, &m.CreationTime); err != nil {
		return nil, err
	}
	return m, nil
}

func (t *channelModelTable) GetChannelModelEqServerIDOrdChannelID(ctx context.Context, d sqldb.Executor, serverid string, orderasc bool, limit, offset int) (_ []ChannelModel, retErr error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]ChannelModel, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT serverid, channelid, chatid, name, desc, theme, creation_time FROM "+t.TableName+" WHERE serverid = $3 ORDER BY channelid "+order+" LIMIT $1 OFFSET $2;", limit, offset, serverid)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("Failed to close db rows: %w", err))
		}
	}()
	for rows.Next() {
		var m ChannelModel
		if err := rows.Scan(&m.ServerID, &m.ChannelID, &m.Chatid, &m.Name, &m.Desc, &m.Theme, &m.CreationTime); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (t *channelModelTable) GetChannelModelEqServerIDLikeChannelIDOrdChannelID(ctx context.Context, d sqldb.Executor, serverid string, channelidPrefix string, orderasc bool, limit, offset int) (_ []ChannelModel, retErr error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]ChannelModel, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT serverid, channelid, chatid, name, desc, theme, creation_time FROM "+t.TableName+" WHERE serverid = $3 AND channelid LIKE $4 ORDER BY channelid "+order+" LIMIT $1 OFFSET $2;", limit, offset, serverid, channelidPrefix)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("Failed to close db rows: %w", err))
		}
	}()
	for rows.Next() {
		var m ChannelModel
		if err := rows.Scan(&m.ServerID, &m.ChannelID, &m.Chatid, &m.Name, &m.Desc, &m.Theme, &m.CreationTime); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (t *channelModelTable) DelEqServerIDHasChannelID(ctx context.Context, d sqldb.Executor, serverid string, channelids []string) error {
	paramCount := 1
	args := make([]interface{}, 0, paramCount+len(channelids))
	args = append(args, serverid)
	var placeholderschannelids string
	{
		placeholders := make([]string, 0, len(channelids))
		for _, i := range channelids {
			paramCount++
			placeholders = append(placeholders, fmt.Sprintf("($%d)", paramCount))
			args = append(args, i)
		}
		placeholderschannelids = strings.Join(placeholders, ", ")
	}
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE serverid = $1 AND channelid IN (VALUES "+placeholderschannelids+");", args...)
	return err
}

func (t *channelModelTable) UpdchannelPropsEqServerIDEqChannelID(ctx context.Context, d sqldb.Executor, m *channelProps, serverid string, channelid string) error {
	_, err := d.ExecContext(ctx, "UPDATE "+t.TableName+" SET (name, desc, theme) = ($1, $2, $3) WHERE serverid = $4 AND channelid = $5;", m.Name, m.Desc, m.Theme, serverid, channelid)
	if err != nil {
		return err
	}
	return nil
}

type (
	presenceModelTable struct {
		TableName string
	}
)

func (t *presenceModelTable) Setup(ctx context.Context, d sqldb.Executor) error {
	_, err := d.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS "+t.TableName+" (serverid VARCHAR(31), userid VARCHAR(31), PRIMARY KEY (serverid, userid), last_updated BIGINT NOT NULL);")
	if err != nil {
		return err
	}
	_, err = d.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS "+t.TableName+"_serverid__last_updated_index ON "+t.TableName+" (serverid, last_updated);")
	if err != nil {
		return err
	}
	return nil
}

func (t *presenceModelTable) Insert(ctx context.Context, d sqldb.Executor, m *PresenceModel) error {
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (serverid, userid, last_updated) VALUES ($1, $2, $3);", m.ServerID, m.Userid, m.LastUpdated)
	if err != nil {
		return err
	}
	return nil
}

func (t *presenceModelTable) InsertBulk(ctx context.Context, d sqldb.Executor, models []*PresenceModel, allowConflict bool) error {
	conflictSQL := ""
	if allowConflict {
		conflictSQL = " ON CONFLICT DO NOTHING"
	}
	placeholders := make([]string, 0, len(models))
	args := make([]interface{}, 0, len(models)*3)
	for c, m := range models {
		n := c * 3
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d)", n+1, n+2, n+3))
		args = append(args, m.ServerID, m.Userid, m.LastUpdated)
	}
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (serverid, userid, last_updated) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
	if err != nil {
		return err
	}
	return nil
}

func (t *presenceModelTable) DelEqServerID(ctx context.Context, d sqldb.Executor, serverid string) error {
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE serverid = $1;", serverid)
	return err
}

func (t *presenceModelTable) GetPresenceModelEqServerIDGtLastUpdatedOrdLastUpdated(ctx context.Context, d sqldb.Executor, serverid string, lastupdated int64, orderasc bool, limit, offset int) (_ []PresenceModel, retErr error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]PresenceModel, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT serverid, userid, last_updated FROM "+t.TableName+" WHERE serverid = $3 AND last_updated > $4 ORDER BY last_updated "+order+" LIMIT $1 OFFSET $2;", limit, offset, serverid, lastupdated)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("Failed to close db rows: %w", err))
		}
	}()
	for rows.Next() {
		var m PresenceModel
		if err := rows.Scan(&m.ServerID, &m.Userid, &m.LastUpdated); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (t *presenceModelTable) DelEqServerIDLeqLastUpdated(ctx context.Context, d sqldb.Executor, serverid string, lastupdated int64) error {
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE serverid = $1 AND last_updated <= $2;", serverid, lastupdated)
	return err
}
