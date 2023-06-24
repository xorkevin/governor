// Code generated by go generate forge model v0.4.4; DO NOT EDIT.

package gdmmodel

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"xorkevin.dev/forge/model/sqldb"
)

type (
	gdmModelTable struct {
		TableName string
	}
)

func (t *gdmModelTable) Setup(ctx context.Context, d sqldb.Executor) error {
	_, err := d.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS "+t.TableName+" (chatid VARCHAR(31) PRIMARY KEY, name VARCHAR(255) NOT NULL, theme VARCHAR(4095) NOT NULL, last_updated BIGINT NOT NULL, creation_time BIGINT NOT NULL);")
	if err != nil {
		return err
	}
	return nil
}

func (t *gdmModelTable) Insert(ctx context.Context, d sqldb.Executor, m *Model) error {
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (chatid, name, theme, last_updated, creation_time) VALUES ($1, $2, $3, $4, $5);", m.Chatid, m.Name, m.Theme, m.LastUpdated, m.CreationTime)
	if err != nil {
		return err
	}
	return nil
}

func (t *gdmModelTable) InsertBulk(ctx context.Context, d sqldb.Executor, models []*Model, allowConflict bool) error {
	conflictSQL := ""
	if allowConflict {
		conflictSQL = " ON CONFLICT DO NOTHING"
	}
	placeholders := make([]string, 0, len(models))
	args := make([]interface{}, 0, len(models)*5)
	for c, m := range models {
		n := c * 5
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d)", n+1, n+2, n+3, n+4, n+5))
		args = append(args, m.Chatid, m.Name, m.Theme, m.LastUpdated, m.CreationTime)
	}
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (chatid, name, theme, last_updated, creation_time) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
	if err != nil {
		return err
	}
	return nil
}

func (t *gdmModelTable) GetModelEqChatid(ctx context.Context, d sqldb.Executor, chatid string) (*Model, error) {
	m := &Model{}
	if err := d.QueryRowContext(ctx, "SELECT chatid, name, theme, last_updated, creation_time FROM "+t.TableName+" WHERE chatid = $1;", chatid).Scan(&m.Chatid, &m.Name, &m.Theme, &m.LastUpdated, &m.CreationTime); err != nil {
		return nil, err
	}
	return m, nil
}

func (t *gdmModelTable) GetModelHasChatidOrdChatid(ctx context.Context, d sqldb.Executor, chatids []string, orderasc bool, limit, offset int) (_ []Model, retErr error) {
	paramCount := 2
	args := make([]interface{}, 0, paramCount+len(chatids))
	args = append(args, limit, offset)
	var placeholderschatids string
	{
		placeholders := make([]string, 0, len(chatids))
		for _, i := range chatids {
			paramCount++
			placeholders = append(placeholders, fmt.Sprintf("($%d)", paramCount))
			args = append(args, i)
		}
		placeholderschatids = strings.Join(placeholders, ", ")
	}
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]Model, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT chatid, name, theme, last_updated, creation_time FROM "+t.TableName+" WHERE chatid IN (VALUES "+placeholderschatids+") ORDER BY chatid "+order+" LIMIT $1 OFFSET $2;", args...)
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
		if err := rows.Scan(&m.Chatid, &m.Name, &m.Theme, &m.LastUpdated, &m.CreationTime); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (t *gdmModelTable) UpdModelEqChatid(ctx context.Context, d sqldb.Executor, m *Model, chatid string) error {
	_, err := d.ExecContext(ctx, "UPDATE "+t.TableName+" SET (chatid, name, theme, last_updated, creation_time) = ($1, $2, $3, $4, $5) WHERE chatid = $6;", m.Chatid, m.Name, m.Theme, m.LastUpdated, m.CreationTime, chatid)
	if err != nil {
		return err
	}
	return nil
}

func (t *gdmModelTable) DelEqChatid(ctx context.Context, d sqldb.Executor, chatid string) error {
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE chatid = $1;", chatid)
	return err
}

func (t *gdmModelTable) UpdgdmPropsEqChatid(ctx context.Context, d sqldb.Executor, m *gdmProps, chatid string) error {
	_, err := d.ExecContext(ctx, "UPDATE "+t.TableName+" SET (name, theme) = ($1, $2) WHERE chatid = $3;", m.Name, m.Theme, chatid)
	if err != nil {
		return err
	}
	return nil
}

func (t *gdmModelTable) UpdmodelLastUpdatedEqChatid(ctx context.Context, d sqldb.Executor, m *modelLastUpdated, chatid string) error {
	_, err := d.ExecContext(ctx, "UPDATE "+t.TableName+" SET last_updated = $1 WHERE chatid = $2;", m.LastUpdated, chatid)
	if err != nil {
		return err
	}
	return nil
}

type (
	memberModelTable struct {
		TableName string
	}
)

func (t *memberModelTable) Setup(ctx context.Context, d sqldb.Executor) error {
	_, err := d.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS "+t.TableName+" (chatid VARCHAR(31), userid VARCHAR(31), PRIMARY KEY (chatid, userid), last_updated BIGINT NOT NULL);")
	if err != nil {
		return err
	}
	_, err = d.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS "+t.TableName+"_userid__chatid_index ON "+t.TableName+" (userid, chatid);")
	if err != nil {
		return err
	}
	_, err = d.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS "+t.TableName+"_userid__last_updated_index ON "+t.TableName+" (userid, last_updated);")
	if err != nil {
		return err
	}
	return nil
}

func (t *memberModelTable) Insert(ctx context.Context, d sqldb.Executor, m *MemberModel) error {
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (chatid, userid, last_updated) VALUES ($1, $2, $3);", m.Chatid, m.Userid, m.LastUpdated)
	if err != nil {
		return err
	}
	return nil
}

func (t *memberModelTable) InsertBulk(ctx context.Context, d sqldb.Executor, models []*MemberModel, allowConflict bool) error {
	conflictSQL := ""
	if allowConflict {
		conflictSQL = " ON CONFLICT DO NOTHING"
	}
	placeholders := make([]string, 0, len(models))
	args := make([]interface{}, 0, len(models)*3)
	for c, m := range models {
		n := c * 3
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d)", n+1, n+2, n+3))
		args = append(args, m.Chatid, m.Userid, m.LastUpdated)
	}
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (chatid, userid, last_updated) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
	if err != nil {
		return err
	}
	return nil
}

func (t *memberModelTable) DelEqChatid(ctx context.Context, d sqldb.Executor, chatid string) error {
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE chatid = $1;", chatid)
	return err
}

func (t *memberModelTable) GetMemberModelEqUseridHasChatidOrdChatid(ctx context.Context, d sqldb.Executor, userid string, chatids []string, orderasc bool, limit, offset int) (_ []MemberModel, retErr error) {
	paramCount := 3
	args := make([]interface{}, 0, paramCount+len(chatids))
	args = append(args, limit, offset, userid)
	var placeholderschatids string
	{
		placeholders := make([]string, 0, len(chatids))
		for _, i := range chatids {
			paramCount++
			placeholders = append(placeholders, fmt.Sprintf("($%d)", paramCount))
			args = append(args, i)
		}
		placeholderschatids = strings.Join(placeholders, ", ")
	}
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]MemberModel, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT chatid, userid, last_updated FROM "+t.TableName+" WHERE userid = $3 AND chatid IN (VALUES "+placeholderschatids+") ORDER BY chatid "+order+" LIMIT $1 OFFSET $2;", args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("Failed to close db rows: %w", err))
		}
	}()
	for rows.Next() {
		var m MemberModel
		if err := rows.Scan(&m.Chatid, &m.Userid, &m.LastUpdated); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (t *memberModelTable) GetMemberModelHasChatidOrdChatid(ctx context.Context, d sqldb.Executor, chatids []string, orderasc bool, limit, offset int) (_ []MemberModel, retErr error) {
	paramCount := 2
	args := make([]interface{}, 0, paramCount+len(chatids))
	args = append(args, limit, offset)
	var placeholderschatids string
	{
		placeholders := make([]string, 0, len(chatids))
		for _, i := range chatids {
			paramCount++
			placeholders = append(placeholders, fmt.Sprintf("($%d)", paramCount))
			args = append(args, i)
		}
		placeholderschatids = strings.Join(placeholders, ", ")
	}
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]MemberModel, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT chatid, userid, last_updated FROM "+t.TableName+" WHERE chatid IN (VALUES "+placeholderschatids+") ORDER BY chatid "+order+" LIMIT $1 OFFSET $2;", args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("Failed to close db rows: %w", err))
		}
	}()
	for rows.Next() {
		var m MemberModel
		if err := rows.Scan(&m.Chatid, &m.Userid, &m.LastUpdated); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (t *memberModelTable) GetMemberModelEqChatidHasUseridOrdUserid(ctx context.Context, d sqldb.Executor, chatid string, userids []string, orderasc bool, limit, offset int) (_ []MemberModel, retErr error) {
	paramCount := 3
	args := make([]interface{}, 0, paramCount+len(userids))
	args = append(args, limit, offset, chatid)
	var placeholdersuserids string
	{
		placeholders := make([]string, 0, len(userids))
		for _, i := range userids {
			paramCount++
			placeholders = append(placeholders, fmt.Sprintf("($%d)", paramCount))
			args = append(args, i)
		}
		placeholdersuserids = strings.Join(placeholders, ", ")
	}
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]MemberModel, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT chatid, userid, last_updated FROM "+t.TableName+" WHERE chatid = $3 AND userid IN (VALUES "+placeholdersuserids+") ORDER BY userid "+order+" LIMIT $1 OFFSET $2;", args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("Failed to close db rows: %w", err))
		}
	}()
	for rows.Next() {
		var m MemberModel
		if err := rows.Scan(&m.Chatid, &m.Userid, &m.LastUpdated); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (t *memberModelTable) DelEqChatidHasUserid(ctx context.Context, d sqldb.Executor, chatid string, userids []string) error {
	paramCount := 1
	args := make([]interface{}, 0, paramCount+len(userids))
	args = append(args, chatid)
	var placeholdersuserids string
	{
		placeholders := make([]string, 0, len(userids))
		for _, i := range userids {
			paramCount++
			placeholders = append(placeholders, fmt.Sprintf("($%d)", paramCount))
			args = append(args, i)
		}
		placeholdersuserids = strings.Join(placeholders, ", ")
	}
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE chatid = $1 AND userid IN (VALUES "+placeholdersuserids+");", args...)
	return err
}

func (t *memberModelTable) GetMemberModelEqUseridOrdLastUpdated(ctx context.Context, d sqldb.Executor, userid string, orderasc bool, limit, offset int) (_ []MemberModel, retErr error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]MemberModel, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT chatid, userid, last_updated FROM "+t.TableName+" WHERE userid = $3 ORDER BY last_updated "+order+" LIMIT $1 OFFSET $2;", limit, offset, userid)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("Failed to close db rows: %w", err))
		}
	}()
	for rows.Next() {
		var m MemberModel
		if err := rows.Scan(&m.Chatid, &m.Userid, &m.LastUpdated); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (t *memberModelTable) GetMemberModelEqUseridLtLastUpdatedOrdLastUpdated(ctx context.Context, d sqldb.Executor, userid string, lastupdated int64, orderasc bool, limit, offset int) (_ []MemberModel, retErr error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]MemberModel, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT chatid, userid, last_updated FROM "+t.TableName+" WHERE userid = $3 AND last_updated < $4 ORDER BY last_updated "+order+" LIMIT $1 OFFSET $2;", limit, offset, userid, lastupdated)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("Failed to close db rows: %w", err))
		}
	}()
	for rows.Next() {
		var m MemberModel
		if err := rows.Scan(&m.Chatid, &m.Userid, &m.LastUpdated); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (t *memberModelTable) UpdmodelLastUpdatedEqChatid(ctx context.Context, d sqldb.Executor, m *modelLastUpdated, chatid string) error {
	_, err := d.ExecContext(ctx, "UPDATE "+t.TableName+" SET last_updated = $1 WHERE chatid = $2;", m.LastUpdated, chatid)
	if err != nil {
		return err
	}
	return nil
}

type (
	assocModelTable struct {
		TableName string
	}
)

func (t *assocModelTable) Setup(ctx context.Context, d sqldb.Executor) error {
	_, err := d.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS "+t.TableName+" (chatid VARCHAR(31), userid_1 VARCHAR(31), userid_2 VARCHAR(31), PRIMARY KEY (chatid, userid_1, userid_2), last_updated BIGINT NOT NULL);")
	if err != nil {
		return err
	}
	_, err = d.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS "+t.TableName+"_userid_2_index ON "+t.TableName+" (userid_2);")
	if err != nil {
		return err
	}
	_, err = d.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS "+t.TableName+"_chatid__userid_2_index ON "+t.TableName+" (chatid, userid_2);")
	if err != nil {
		return err
	}
	_, err = d.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS "+t.TableName+"_userid_1__userid_2__last_updated_index ON "+t.TableName+" (userid_1, userid_2, last_updated);")
	if err != nil {
		return err
	}
	return nil
}

func (t *assocModelTable) Insert(ctx context.Context, d sqldb.Executor, m *AssocModel) error {
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (chatid, userid_1, userid_2, last_updated) VALUES ($1, $2, $3, $4);", m.Chatid, m.Userid1, m.Userid2, m.LastUpdated)
	if err != nil {
		return err
	}
	return nil
}

func (t *assocModelTable) InsertBulk(ctx context.Context, d sqldb.Executor, models []*AssocModel, allowConflict bool) error {
	conflictSQL := ""
	if allowConflict {
		conflictSQL = " ON CONFLICT DO NOTHING"
	}
	placeholders := make([]string, 0, len(models))
	args := make([]interface{}, 0, len(models)*4)
	for c, m := range models {
		n := c * 4
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d)", n+1, n+2, n+3, n+4))
		args = append(args, m.Chatid, m.Userid1, m.Userid2, m.LastUpdated)
	}
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (chatid, userid_1, userid_2, last_updated) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
	if err != nil {
		return err
	}
	return nil
}

func (t *assocModelTable) DelEqChatid(ctx context.Context, d sqldb.Executor, chatid string) error {
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE chatid = $1;", chatid)
	return err
}

func (t *assocModelTable) DelEqChatidHasUserid1(ctx context.Context, d sqldb.Executor, chatid string, userid1s []string) error {
	paramCount := 1
	args := make([]interface{}, 0, paramCount+len(userid1s))
	args = append(args, chatid)
	var placeholdersuserid1s string
	{
		placeholders := make([]string, 0, len(userid1s))
		for _, i := range userid1s {
			paramCount++
			placeholders = append(placeholders, fmt.Sprintf("($%d)", paramCount))
			args = append(args, i)
		}
		placeholdersuserid1s = strings.Join(placeholders, ", ")
	}
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE chatid = $1 AND userid_1 IN (VALUES "+placeholdersuserid1s+");", args...)
	return err
}

func (t *assocModelTable) DelEqChatidHasUserid2(ctx context.Context, d sqldb.Executor, chatid string, userid2s []string) error {
	paramCount := 1
	args := make([]interface{}, 0, paramCount+len(userid2s))
	args = append(args, chatid)
	var placeholdersuserid2s string
	{
		placeholders := make([]string, 0, len(userid2s))
		for _, i := range userid2s {
			paramCount++
			placeholders = append(placeholders, fmt.Sprintf("($%d)", paramCount))
			args = append(args, i)
		}
		placeholdersuserid2s = strings.Join(placeholders, ", ")
	}
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE chatid = $1 AND userid_2 IN (VALUES "+placeholdersuserid2s+");", args...)
	return err
}

func (t *assocModelTable) GetAssocModelEqUserid1EqUserid2OrdLastUpdated(ctx context.Context, d sqldb.Executor, userid1 string, userid2 string, orderasc bool, limit, offset int) (_ []AssocModel, retErr error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]AssocModel, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT chatid, userid_1, userid_2, last_updated FROM "+t.TableName+" WHERE userid_1 = $3 AND userid_2 = $4 ORDER BY last_updated "+order+" LIMIT $1 OFFSET $2;", limit, offset, userid1, userid2)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("Failed to close db rows: %w", err))
		}
	}()
	for rows.Next() {
		var m AssocModel
		if err := rows.Scan(&m.Chatid, &m.Userid1, &m.Userid2, &m.LastUpdated); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (t *assocModelTable) UpdmodelLastUpdatedEqChatid(ctx context.Context, d sqldb.Executor, m *modelLastUpdated, chatid string) error {
	_, err := d.ExecContext(ctx, "UPDATE "+t.TableName+" SET last_updated = $1 WHERE chatid = $2;", m.LastUpdated, chatid)
	if err != nil {
		return err
	}
	return nil
}
