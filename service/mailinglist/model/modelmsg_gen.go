// Code generated by go generate forge model v0.3; DO NOT EDIT.

package model

import (
	"context"
	"fmt"
	"strings"

	"xorkevin.dev/governor/service/db"
)

type (
	msgModelTable struct {
		TableName string
	}
)

func (t *msgModelTable) Setup(ctx context.Context, d db.SQLExecutor) error {
	_, err := d.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS "+t.TableName+" (listid VARCHAR(255), msgid VARCHAR(1023), PRIMARY KEY (listid, msgid), userid VARCHAR(31) NOT NULL, creation_time BIGINT NOT NULL, spf_pass VARCHAR(255) NOT NULL, dkim_pass VARCHAR(255) NOT NULL, subject VARCHAR(255) NOT NULL, in_reply_to VARCHAR(1023) NOT NULL, parent_id VARCHAR(1023) NOT NULL, thread_id VARCHAR(1023) NOT NULL, processed BOOL NOT NULL, sent BOOL NOT NULL, deleted BOOL NOT NULL);")
	if err != nil {
		return err
	}
	_, err = d.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS "+t.TableName+"_listid__creation_time_index ON "+t.TableName+" (listid, creation_time);")
	if err != nil {
		return err
	}
	_, err = d.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS "+t.TableName+"_listid__thread_id__creation_time_index ON "+t.TableName+" (listid, thread_id, creation_time);")
	if err != nil {
		return err
	}
	_, err = d.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS "+t.TableName+"_listid__in_reply_to_index ON "+t.TableName+" (listid, in_reply_to);")
	if err != nil {
		return err
	}
	_, err = d.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS "+t.TableName+"_listid__thread_id__in_reply_to_index ON "+t.TableName+" (listid, thread_id, in_reply_to);")
	if err != nil {
		return err
	}
	return nil
}

func (t *msgModelTable) Insert(ctx context.Context, d db.SQLExecutor, m *MsgModel) error {
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (listid, msgid, userid, creation_time, spf_pass, dkim_pass, subject, in_reply_to, parent_id, thread_id, processed, sent, deleted) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13);", m.ListID, m.Msgid, m.Userid, m.CreationTime, m.SPFPass, m.DKIMPass, m.Subject, m.InReplyTo, m.ParentID, m.ThreadID, m.Processed, m.Sent, m.Deleted)
	if err != nil {
		return err
	}
	return nil
}

func (t *msgModelTable) InsertBulk(ctx context.Context, d db.SQLExecutor, models []*MsgModel, allowConflict bool) error {
	conflictSQL := ""
	if allowConflict {
		conflictSQL = " ON CONFLICT DO NOTHING"
	}
	placeholders := make([]string, 0, len(models))
	args := make([]interface{}, 0, len(models)*13)
	for c, m := range models {
		n := c * 13
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)", n+1, n+2, n+3, n+4, n+5, n+6, n+7, n+8, n+9, n+10, n+11, n+12, n+13))
		args = append(args, m.ListID, m.Msgid, m.Userid, m.CreationTime, m.SPFPass, m.DKIMPass, m.Subject, m.InReplyTo, m.ParentID, m.ThreadID, m.Processed, m.Sent, m.Deleted)
	}
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (listid, msgid, userid, creation_time, spf_pass, dkim_pass, subject, in_reply_to, parent_id, thread_id, processed, sent, deleted) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
	if err != nil {
		return err
	}
	return nil
}

func (t *msgModelTable) GetMsgModelEqListIDEqMsgid(ctx context.Context, d db.SQLExecutor, listid string, msgid string) (*MsgModel, error) {
	m := &MsgModel{}
	if err := d.QueryRowContext(ctx, "SELECT listid, msgid, userid, creation_time, spf_pass, dkim_pass, subject, in_reply_to, parent_id, thread_id, processed, sent, deleted FROM "+t.TableName+" WHERE listid = $1 AND msgid = $2;", listid, msgid).Scan(&m.ListID, &m.Msgid, &m.Userid, &m.CreationTime, &m.SPFPass, &m.DKIMPass, &m.Subject, &m.InReplyTo, &m.ParentID, &m.ThreadID, &m.Processed, &m.Sent, &m.Deleted); err != nil {
		return nil, err
	}
	return m, nil
}

func (t *msgModelTable) GetMsgModelEqListIDOrdCreationTime(ctx context.Context, d db.SQLExecutor, listid string, orderasc bool, limit, offset int) ([]MsgModel, error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]MsgModel, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT listid, msgid, userid, creation_time, spf_pass, dkim_pass, subject, in_reply_to, parent_id, thread_id, processed, sent, deleted FROM "+t.TableName+" WHERE listid = $3 ORDER BY creation_time "+order+" LIMIT $1 OFFSET $2;", limit, offset, listid)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		m := MsgModel{}
		if err := rows.Scan(&m.ListID, &m.Msgid, &m.Userid, &m.CreationTime, &m.SPFPass, &m.DKIMPass, &m.Subject, &m.InReplyTo, &m.ParentID, &m.ThreadID, &m.Processed, &m.Sent, &m.Deleted); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (t *msgModelTable) GetMsgModelEqListIDEqThreadIDOrdCreationTime(ctx context.Context, d db.SQLExecutor, listid string, threadid string, orderasc bool, limit, offset int) ([]MsgModel, error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]MsgModel, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT listid, msgid, userid, creation_time, spf_pass, dkim_pass, subject, in_reply_to, parent_id, thread_id, processed, sent, deleted FROM "+t.TableName+" WHERE listid = $3 AND thread_id = $4 ORDER BY creation_time "+order+" LIMIT $1 OFFSET $2;", limit, offset, listid, threadid)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		m := MsgModel{}
		if err := rows.Scan(&m.ListID, &m.Msgid, &m.Userid, &m.CreationTime, &m.SPFPass, &m.DKIMPass, &m.Subject, &m.InReplyTo, &m.ParentID, &m.ThreadID, &m.Processed, &m.Sent, &m.Deleted); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (t *msgModelTable) UpdmsgProcessedEqListIDEqMsgid(ctx context.Context, d db.SQLExecutor, m *msgProcessed, listid string, msgid string) error {
	_, err := d.ExecContext(ctx, "UPDATE "+t.TableName+" SET (processed) = ROW($1) WHERE listid = $2 AND msgid = $3;", m.Processed, listid, msgid)
	if err != nil {
		return err
	}
	return nil
}

func (t *msgModelTable) UpdmsgSentEqListIDEqMsgid(ctx context.Context, d db.SQLExecutor, m *msgSent, listid string, msgid string) error {
	_, err := d.ExecContext(ctx, "UPDATE "+t.TableName+" SET (sent) = ROW($1) WHERE listid = $2 AND msgid = $3;", m.Sent, listid, msgid)
	if err != nil {
		return err
	}
	return nil
}

func (t *msgModelTable) UpdmsgDeletedEqListIDHasMsgid(ctx context.Context, d db.SQLExecutor, m *msgDeleted, listid string, msgid []string) error {
	paramCount := 6
	args := make([]interface{}, 0, paramCount+len(msgid))
	args = append(args, m.Userid, m.SPFPass, m.DKIMPass, m.Subject, m.Deleted, listid)
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
	_, err := d.ExecContext(ctx, "UPDATE "+t.TableName+" SET (userid, spf_pass, dkim_pass, subject, deleted) = ROW($1, $2, $3, $4, $5) WHERE listid = $6 AND msgid IN (VALUES "+placeholdersmsgid+");", args...)
	if err != nil {
		return err
	}
	return nil
}

func (t *msgModelTable) UpdmsgParentEqListIDEqMsgidEqThreadID(ctx context.Context, d db.SQLExecutor, m *msgParent, listid string, msgid string, threadid string) error {
	_, err := d.ExecContext(ctx, "UPDATE "+t.TableName+" SET (parent_id, thread_id) = ROW($1, $2) WHERE listid = $3 AND msgid = $4 AND thread_id = $5;", m.ParentID, m.ThreadID, listid, msgid, threadid)
	if err != nil {
		return err
	}
	return nil
}

func (t *msgModelTable) UpdmsgChildrenEqListIDEqThreadIDEqInReplyTo(ctx context.Context, d db.SQLExecutor, m *msgChildren, listid string, threadid string, inreplyto string) error {
	_, err := d.ExecContext(ctx, "UPDATE "+t.TableName+" SET (parent_id, thread_id) = ROW($1, $2) WHERE listid = $3 AND thread_id = $4 AND in_reply_to = $5;", m.ParentID, m.ThreadID, listid, threadid, inreplyto)
	if err != nil {
		return err
	}
	return nil
}
