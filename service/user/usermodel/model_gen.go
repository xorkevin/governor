// Code generated by go generate forge model v0.4.2; DO NOT EDIT.

package usermodel

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"xorkevin.dev/governor/service/db"
)

type (
	userModelTable struct {
		TableName string
	}
)

func (t *userModelTable) Setup(ctx context.Context, d db.SQLExecutor) error {
	_, err := d.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS "+t.TableName+" (userid VARCHAR(31) PRIMARY KEY, username VARCHAR(255) NOT NULL UNIQUE, pass_hash VARCHAR(255) NOT NULL, otp_enabled BOOLEAN NOT NULL, otp_secret VARCHAR(255) NOT NULL, otp_backup VARCHAR(255) NOT NULL, email VARCHAR(255) NOT NULL UNIQUE, first_name VARCHAR(255) NOT NULL, last_name VARCHAR(255) NOT NULL, creation_time BIGINT NOT NULL, failed_login_time BIGINT NOT NULL, failed_login_count INT NOT NULL);")
	if err != nil {
		return err
	}
	return nil
}

func (t *userModelTable) Insert(ctx context.Context, d db.SQLExecutor, m *Model) error {
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (userid, username, pass_hash, otp_enabled, otp_secret, otp_backup, email, first_name, last_name, creation_time, failed_login_time, failed_login_count) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12);", m.Userid, m.Username, m.PassHash, m.OTPEnabled, m.OTPSecret, m.OTPBackup, m.Email, m.FirstName, m.LastName, m.CreationTime, m.FailedLoginTime, m.FailedLoginCount)
	if err != nil {
		return err
	}
	return nil
}

func (t *userModelTable) InsertBulk(ctx context.Context, d db.SQLExecutor, models []*Model, allowConflict bool) error {
	conflictSQL := ""
	if allowConflict {
		conflictSQL = " ON CONFLICT DO NOTHING"
	}
	placeholders := make([]string, 0, len(models))
	args := make([]interface{}, 0, len(models)*12)
	for c, m := range models {
		n := c * 12
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)", n+1, n+2, n+3, n+4, n+5, n+6, n+7, n+8, n+9, n+10, n+11, n+12))
		args = append(args, m.Userid, m.Username, m.PassHash, m.OTPEnabled, m.OTPSecret, m.OTPBackup, m.Email, m.FirstName, m.LastName, m.CreationTime, m.FailedLoginTime, m.FailedLoginCount)
	}
	_, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (userid, username, pass_hash, otp_enabled, otp_secret, otp_backup, email, first_name, last_name, creation_time, failed_login_time, failed_login_count) VALUES "+strings.Join(placeholders, ", ")+conflictSQL+";", args...)
	if err != nil {
		return err
	}
	return nil
}

func (t *userModelTable) GetModelEqUserid(ctx context.Context, d db.SQLExecutor, userid string) (*Model, error) {
	m := &Model{}
	if err := d.QueryRowContext(ctx, "SELECT userid, username, pass_hash, otp_enabled, otp_secret, otp_backup, email, first_name, last_name, creation_time, failed_login_time, failed_login_count FROM "+t.TableName+" WHERE userid = $1;", userid).Scan(&m.Userid, &m.Username, &m.PassHash, &m.OTPEnabled, &m.OTPSecret, &m.OTPBackup, &m.Email, &m.FirstName, &m.LastName, &m.CreationTime, &m.FailedLoginTime, &m.FailedLoginCount); err != nil {
		return nil, err
	}
	return m, nil
}

func (t *userModelTable) DelEqUserid(ctx context.Context, d db.SQLExecutor, userid string) error {
	_, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE userid = $1;", userid)
	return err
}

func (t *userModelTable) GetModelEqUsername(ctx context.Context, d db.SQLExecutor, username string) (*Model, error) {
	m := &Model{}
	if err := d.QueryRowContext(ctx, "SELECT userid, username, pass_hash, otp_enabled, otp_secret, otp_backup, email, first_name, last_name, creation_time, failed_login_time, failed_login_count FROM "+t.TableName+" WHERE username = $1;", username).Scan(&m.Userid, &m.Username, &m.PassHash, &m.OTPEnabled, &m.OTPSecret, &m.OTPBackup, &m.Email, &m.FirstName, &m.LastName, &m.CreationTime, &m.FailedLoginTime, &m.FailedLoginCount); err != nil {
		return nil, err
	}
	return m, nil
}

func (t *userModelTable) GetModelEqEmail(ctx context.Context, d db.SQLExecutor, email string) (*Model, error) {
	m := &Model{}
	if err := d.QueryRowContext(ctx, "SELECT userid, username, pass_hash, otp_enabled, otp_secret, otp_backup, email, first_name, last_name, creation_time, failed_login_time, failed_login_count FROM "+t.TableName+" WHERE email = $1;", email).Scan(&m.Userid, &m.Username, &m.PassHash, &m.OTPEnabled, &m.OTPSecret, &m.OTPBackup, &m.Email, &m.FirstName, &m.LastName, &m.CreationTime, &m.FailedLoginTime, &m.FailedLoginCount); err != nil {
		return nil, err
	}
	return m, nil
}

func (t *userModelTable) GetInfoOrdUserid(ctx context.Context, d db.SQLExecutor, orderasc bool, limit, offset int) (_ []Info, retErr error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]Info, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT userid, username, email, first_name, last_name FROM "+t.TableName+" ORDER BY userid "+order+" LIMIT $1 OFFSET $2;", limit, offset)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("Failed to close db rows: %w", err))
		}
	}()
	for rows.Next() {
		var m Info
		if err := rows.Scan(&m.Userid, &m.Username, &m.Email, &m.FirstName, &m.LastName); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (t *userModelTable) GetInfoHasUseridOrdUserid(ctx context.Context, d db.SQLExecutor, userids []string, orderasc bool, limit, offset int) (_ []Info, retErr error) {
	paramCount := 2
	args := make([]interface{}, 0, paramCount+len(userids))
	args = append(args, limit, offset)
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
	res := make([]Info, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT userid, username, email, first_name, last_name FROM "+t.TableName+" WHERE userid IN (VALUES "+placeholdersuserids+") ORDER BY userid "+order+" LIMIT $1 OFFSET $2;", args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("Failed to close db rows: %w", err))
		}
	}()
	for rows.Next() {
		var m Info
		if err := rows.Scan(&m.Userid, &m.Username, &m.Email, &m.FirstName, &m.LastName); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (t *userModelTable) GetInfoLikeUsernameOrdUsername(ctx context.Context, d db.SQLExecutor, usernamePrefix string, orderasc bool, limit, offset int) (_ []Info, retErr error) {
	order := "DESC"
	if orderasc {
		order = "ASC"
	}
	res := make([]Info, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT userid, username, email, first_name, last_name FROM "+t.TableName+" WHERE username LIKE $3 ORDER BY username "+order+" LIMIT $1 OFFSET $2;", limit, offset, usernamePrefix)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("Failed to close db rows: %w", err))
		}
	}()
	for rows.Next() {
		var m Info
		if err := rows.Scan(&m.Userid, &m.Username, &m.Email, &m.FirstName, &m.LastName); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (t *userModelTable) UpduserPropsEqUserid(ctx context.Context, d db.SQLExecutor, m *userProps, userid string) error {
	_, err := d.ExecContext(ctx, "UPDATE "+t.TableName+" SET (username, first_name, last_name) = ROW($1, $2, $3) WHERE userid = $4;", m.Username, m.FirstName, m.LastName, userid)
	if err != nil {
		return err
	}
	return nil
}

func (t *userModelTable) UpduserEmailEqUserid(ctx context.Context, d db.SQLExecutor, m *userEmail, userid string) error {
	_, err := d.ExecContext(ctx, "UPDATE "+t.TableName+" SET (email) = ROW($1) WHERE userid = $2;", m.Email, userid)
	if err != nil {
		return err
	}
	return nil
}

func (t *userModelTable) UpduserPassHashEqUserid(ctx context.Context, d db.SQLExecutor, m *userPassHash, userid string) error {
	_, err := d.ExecContext(ctx, "UPDATE "+t.TableName+" SET (pass_hash) = ROW($1) WHERE userid = $2;", m.PassHash, userid)
	if err != nil {
		return err
	}
	return nil
}

func (t *userModelTable) UpduserGenOTPEqUserid(ctx context.Context, d db.SQLExecutor, m *userGenOTP, userid string) error {
	_, err := d.ExecContext(ctx, "UPDATE "+t.TableName+" SET (otp_enabled, otp_secret, otp_backup, failed_login_time, failed_login_count) = ROW($1, $2, $3, $4, $5) WHERE userid = $6;", m.OTPEnabled, m.OTPSecret, m.OTPBackup, m.FailedLoginTime, m.FailedLoginCount, userid)
	if err != nil {
		return err
	}
	return nil
}

func (t *userModelTable) UpduserFailLoginEqUserid(ctx context.Context, d db.SQLExecutor, m *userFailLogin, userid string) error {
	_, err := d.ExecContext(ctx, "UPDATE "+t.TableName+" SET (failed_login_time, failed_login_count) = ROW($1, $2) WHERE userid = $3;", m.FailedLoginTime, m.FailedLoginCount, userid)
	if err != nil {
		return err
	}
	return nil
}
