package model

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/kerrors"
)

//go:generate forge model

const (
	chatUIDSize = 16
)

type (
	Repo interface {
		New(userid1, userid2 string) (*Model, error)
		GetByID(ctx context.Context, userid1, userid2 string) (*Model, error)
		GetByChatID(ctx context.Context, chatid string) (*Model, error)
		GetLatest(ctx context.Context, userid string, before int64, limit int) ([]Model, error)
		GetByUser(ctx context.Context, userid string, userids []string) ([]DMInfo, error)
		GetChats(ctx context.Context, chatids []string) ([]Model, error)
		Insert(ctx context.Context, m *Model) error
		UpdateProps(ctx context.Context, m *Model) error
		UpdateLastUpdated(ctx context.Context, userid1, userid2 string, t int64) error
		Delete(ctx context.Context, userid1, userid2 string) error
		Setup(ctx context.Context) error
	}

	repo struct {
		table *dmModelTable
		db    db.Database
	}

	// Model is the db dm chat model
	//forge:model dm
	//forge:model:query dm
	Model struct {
		Userid1      string `model:"userid_1,VARCHAR(31)" query:"userid_1"`
		Userid2      string `model:"userid_2,VARCHAR(31), PRIMARY KEY (userid_1, userid_2)" query:"userid_2;getoneeq,userid_1,userid_2;deleq,userid_1,userid_2"`
		Chatid       string `model:"chatid,VARCHAR(31) NOT NULL UNIQUE" query:"chatid;getoneeq,chatid"`
		Name         string `model:"name,VARCHAR(255) NOT NULL" query:"name"`
		Theme        string `model:"theme,VARCHAR(4095) NOT NULL" query:"theme"`
		LastUpdated  int64  `model:"last_updated,BIGINT NOT NULL;index,userid_1;index,userid_2" query:"last_updated;getgroupeq,chatid|in"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL" query:"creation_time"`
	}

	DMInfo struct {
		Userid1 string
		Userid2 string
		Chatid  string
		Name    string
	}

	//forge:model:query dm
	dmProps struct {
		Name  string `query:"name;updeq,userid_1,userid_2"`
		Theme string `query:"theme"`
	}

	//forge:model:query dm
	dmLastUpdated struct {
		LastUpdated int64 `query:"last_updated;updeq,userid_1,userid_2"`
	}

	ctxKeyRepo struct{}
)

// GetCtxRepo returns a Repo from the context
func GetCtxRepo(inj governor.Injector) Repo {
	v := inj.Get(ctxKeyRepo{})
	if v == nil {
		return nil
	}
	return v.(Repo)
}

// SetCtxRepo sets a Repo in the context
func SetCtxRepo(inj governor.Injector, r Repo) {
	inj.Set(ctxKeyRepo{}, r)
}

func NewInCtx(inj governor.Injector, table string) {
	SetCtxRepo(inj, NewCtx(inj, table))
}

func NewCtx(inj governor.Injector, table string) Repo {
	dbService := db.GetCtxDB(inj)
	return New(dbService, table)
}

func New(database db.Database, table string) Repo {
	return &repo{
		table: &dmModelTable{
			TableName: table,
		},
		db: database,
	}
}

func tupleSort(a, b string) (string, string) {
	if a < b {
		return a, b
	}
	return b, a
}

// New creates new dm
func (r *repo) New(userid1, userid2 string) (*Model, error) {
	userid1, userid2 = tupleSort(userid1, userid2)
	u, err := uid.New(chatUIDSize)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to create new uid")
	}
	now := time.Now().Round(0)
	return &Model{
		Userid1:      userid1,
		Userid2:      userid2,
		Chatid:       u.Base64(),
		Name:         "",
		Theme:        "{}",
		LastUpdated:  now.UnixMilli(),
		CreationTime: now.Unix(),
	}, nil
}

func (r *repo) GetByID(ctx context.Context, userid1, userid2 string) (*Model, error) {
	userid1, userid2 = tupleSort(userid1, userid2)
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelEqUserid1EqUserid2(ctx, d, userid1, userid2)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get dm")
	}
	return m, nil
}

func (r *repo) GetByChatID(ctx context.Context, chatid string) (*Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelEqChatid(ctx, d, chatid)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get dm")
	}
	return m, nil
}

func (t *dmModelTable) GetDMsEqUserOrdLastUpdated(ctx context.Context, d db.SQLExecutor, userid string, limit int) ([]Model, error) {
	res := make([]Model, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT userid_1, userid_2, chatid, name, theme, last_updated, creation_time FROM "+t.TableName+" WHERE (userid_1 = $2 OR userid_2 = $2) ORDER BY last_updated DESC LIMIT $1;", limit, userid)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		m := Model{}
		if err := rows.Scan(&m.Userid1, &m.Userid2, &m.Chatid, &m.Name, &m.Theme, &m.LastUpdated, &m.CreationTime); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (t *dmModelTable) GetDMsEqUserLtBeforeOrdLastUpdated(ctx context.Context, d db.SQLExecutor, userid string, before int64, limit int) ([]Model, error) {
	res := make([]Model, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT userid_1, userid_2, chatid, name, theme, last_updated, creation_time FROM "+t.TableName+" WHERE (userid_1 = $2 OR userid_2 = $2) AND last_updated < $3 ORDER BY last_updated DESC LIMIT $1;", limit, userid, before)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		m := Model{}
		if err := rows.Scan(&m.Userid1, &m.Userid2, &m.Chatid, &m.Name, &m.Theme, &m.LastUpdated, &m.CreationTime); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (r *repo) GetLatest(ctx context.Context, userid string, before int64, limit int) ([]Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}

	if before == 0 {
		m, err := r.table.GetDMsEqUserOrdLastUpdated(ctx, d, userid, limit)
		if err != nil {
			return nil, kerrors.WithMsg(err, "Failed to get latest dms")
		}
		return m, nil
	}

	m, err := r.table.GetDMsEqUserLtBeforeOrdLastUpdated(ctx, d, userid, before, limit)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get latest dms")
	}
	return m, nil
}

func (t *dmModelTable) GetDMsEqUserHasUser(ctx context.Context, d db.SQLExecutor, userid string, userids []string) ([]DMInfo, error) {
	args := make([]interface{}, 0, len(userids)*2)
	var placeholdersid string
	{
		paramCount := 1
		placeholders := make([]string, 0, len(userids))
		for _, i := range userids {
			placeholders = append(placeholders, fmt.Sprintf("($%d, $%d)", paramCount, paramCount+1))
			paramCount += 2
			a, b := tupleSort(userid, i)
			args = append(args, a, b)
		}
		placeholdersid = strings.Join(placeholders, ", ")
	}
	rows, err := d.QueryContext(ctx, "SELECT userid_1, userid_2, chatid, name FROM "+t.TableName+" WHERE (userid_1, userid_2) IN (VALUES "+placeholdersid+") ORDER BY last_updated DESC;", args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	res := make([]DMInfo, 0, len(userids))
	for rows.Next() {
		k := DMInfo{}
		if err := rows.Scan(&k.Userid1, &k.Userid2, &k.Chatid, &k.Name); err != nil {
			return nil, err
		}
		res = append(res, k)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (r *repo) GetByUser(ctx context.Context, userid string, userids []string) ([]DMInfo, error) {
	if len(userids) == 0 {
		return nil, nil
	}

	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}

	m, err := r.table.GetDMsEqUserHasUser(ctx, d, userid, userids)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get user dms")
	}
	return m, nil
}

func (r *repo) GetChats(ctx context.Context, chatids []string) ([]Model, error) {
	if len(chatids) == 0 {
		return nil, nil
	}

	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelHasChatidOrdLastUpdated(ctx, d, chatids, false, len(chatids), 0)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get dms")
	}
	return m, nil
}

func (r *repo) Insert(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.Insert(ctx, d, m); err != nil {
		return kerrors.WithMsg(err, "Failed to insert dm")
	}
	return nil
}

func (r *repo) UpdateProps(ctx context.Context, m *Model) error {
	userid1, userid2 := tupleSort(m.Userid1, m.Userid2)
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.UpddmPropsEqUserid1EqUserid2(ctx, d, &dmProps{
		Name:  m.Name,
		Theme: m.Theme,
	}, userid1, userid2); err != nil {
		return kerrors.WithMsg(err, "Failed to update dm")
	}
	return nil
}

func (r *repo) UpdateLastUpdated(ctx context.Context, userid1, userid2 string, t int64) error {
	userid1, userid2 = tupleSort(userid1, userid2)
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.UpddmLastUpdatedEqUserid1EqUserid2(ctx, d, &dmLastUpdated{
		LastUpdated: t,
	}, userid1, userid2); err != nil {
		return kerrors.WithMsg(err, "Failed to update dm last updated")
	}
	return nil
}

func (r *repo) Delete(ctx context.Context, userid1, userid2 string) error {
	userid1, userid2 = tupleSort(userid1, userid2)
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelEqUserid1EqUserid2(ctx, d, userid1, userid2); err != nil {
		return kerrors.WithMsg(err, "Failed to delete dm")
	}
	return nil
}

func (r *repo) Setup(ctx context.Context) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.Setup(ctx, d); err != nil {
		err = kerrors.WithMsg(err, "Failed to setup dm model")
		if !errors.Is(err, db.ErrorAuthz{}) {
			return err
		}
	}
	return nil
}
