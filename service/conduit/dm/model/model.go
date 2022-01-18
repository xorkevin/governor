package model

import (
	"errors"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/uid"
)

//go:generate forge model -m Model -p dm -o model_gen.go Model dmLastUpdated

const (
	chatUIDSize = 16
)

type (
	Repo interface {
		New(userid1, userid2 string) (*Model, error)
		GetByID(userid1, userid2 string) (*Model, error)
		GetLatest(userid string, before int64, limit int) ([]string, error)
		GetChats(chatids []string) ([]Model, error)
		Insert(m *Model) error
		Update(m *Model) error
		UpdateLastUpdated(userid1, userid2 string, t int64) error
		Delete(userid1, userid2 string) error
		Setup() error
	}

	repo struct {
		table string
		db    db.Database
	}

	// Model is the db dm chat model
	Model struct {
		Userid1      string `model:"userid_1,VARCHAR(31)" query:"userid_1"`
		Userid2      string `model:"userid_2,VARCHAR(31), PRIMARY KEY (userid_1, userid_2)" query:"userid_2;getoneeq,userid_1,userid_2;updeq,userid_1,userid_2;deleq,userid_1,userid_2"`
		Chatid       string `model:"chatid,VARCHAR(31) NOT NULL UNIQUE" query:"chatid"`
		Name         string `model:"name,VARCHAR(255) NOT NULL" query:"name"`
		Theme        string `model:"theme,VARCHAR(4095) NOT NULL" query:"theme"`
		LastUpdated  int64  `model:"last_updated,BIGINT NOT NULL;index,userid_1;index,userid_2" query:"last_updated;getgroupeq,chatid|arr"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL" query:"creation_time"`
	}

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
		table: table,
		db:    database,
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
		return nil, governor.ErrWithMsg(err, "Failed to create new uid")
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

func (r *repo) GetByID(userid1, userid2 string) (*Model, error) {
	userid1, userid2 = tupleSort(userid1, userid2)
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := dmModelGetModelEqUserid1EqUserid2(d, r.table, userid1, userid2)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get dm")
	}
	return m, nil
}

func (r *repo) GetLatest(userid string, before int64, limit int) ([]string, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}

	res := make([]string, 0, limit)
	if before == 0 {
		rows, err := d.Query("SELECT chatid FROM "+r.table+" WHERE (userid_1 = $2 OR userid_2 = $2) ORDER BY last_updated DESC LIMIT $1;", limit, userid)
		if err != nil {
			return nil, db.WrapErr(err, "Failed to get latest dms")
		}
		defer func() {
			if err := rows.Close(); err != nil {
			}
		}()
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				return nil, db.WrapErr(err, "Failed to get latest dms")
			}
			res = append(res, s)
		}
		if err := rows.Err(); err != nil {
			return nil, db.WrapErr(err, "Failed to get latest dms")
		}
	} else {
		rows, err := d.Query("SELECT chatid FROM "+r.table+" WHERE (userid_1 = $2 OR userid_2 = $2) AND last_updated < $3 ORDER BY last_updated DESC LIMIT $1;", limit, userid, before)
		if err != nil {
			return nil, db.WrapErr(err, "Failed to get latest dms")
		}
		defer func() {
			if err := rows.Close(); err != nil {
			}
		}()
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				return nil, db.WrapErr(err, "Failed to get latest dms")
			}
			res = append(res, s)
		}
		if err := rows.Err(); err != nil {
			return nil, db.WrapErr(err, "Failed to get latest dms")
		}
	}
	return res, nil
}

func (r *repo) GetChats(chatids []string) ([]Model, error) {
	if len(chatids) == 0 {
		return nil, nil
	}
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := dmModelGetModelHasChatidOrdLastUpdated(d, r.table, chatids, false, len(chatids), 0)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get dms")
	}
	return m, nil
}

func (r *repo) Insert(m *Model) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := dmModelInsert(d, r.table, m); err != nil {
		return db.WrapErr(err, "Failed to insert dm")
	}
	return nil
}

func (r *repo) Update(m *Model) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := dmModelUpdModelEqUserid1EqUserid2(d, r.table, m, m.Userid1, m.Userid2); err != nil {
		return db.WrapErr(err, "Failed to update dm")
	}
	return nil
}

func (r *repo) UpdateLastUpdated(userid1, userid2 string, t int64) error {
	userid1, userid2 = tupleSort(userid1, userid2)
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := dmModelUpddmLastUpdatedEqUserid1EqUserid2(d, r.table, &dmLastUpdated{
		LastUpdated: t,
	}, userid1, userid2); err != nil {
		return db.WrapErr(err, "Failed to update dm last updated")
	}
	return nil
}

func (r *repo) Delete(userid1, userid2 string) error {
	userid1, userid2 = tupleSort(userid1, userid2)
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := dmModelDelEqUserid1EqUserid2(d, r.table, userid1, userid2); err != nil {
		return db.WrapErr(err, "Failed to delete dm")
	}
	return nil
}

func (r *repo) Setup() error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := dmModelSetup(d, r.table); err != nil {
		err = db.WrapErr(err, "Failed to setup dm model")
		if !errors.Is(err, db.ErrAuthz{}) {
			return err
		}
	}
	return nil
}
