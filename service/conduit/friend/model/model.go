package model

import (
	"errors"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
)

//go:generate forge model -m Model -p friend -o model_gen.go Model friendUsername

type (
	Repo interface {
		GetByID(userid1, userid2 string) (*Model, error)
		GetFriends(userid string, prefix string, limit, offset int) ([]Model, error)
		Insert(userid1, userid2 string, username1, username2 string) error
		Remove(userid1, userid2 string) error
		UpdateUsername(userid, username string) error
		DeleteUser(userid string) error
		Setup() error
	}

	repo struct {
		table string
		db    db.Database
	}

	// Model is the db friend relationship model
	Model struct {
		Userid1  string `model:"userid_1,VARCHAR(31)" query:"userid_1;deleq,userid_1"`
		Userid2  string `model:"userid_2,VARCHAR(31), PRIMARY KEY (userid_1, userid_2);index" query:"userid_2;getoneeq,userid_1,userid_2;deleq,userid_2"`
		Username string `model:"username,VARCHAR(255) NOT NULL;index,userid_1" query:"username;getgroupeq,userid_1;getgroupeq,userid_1,username|like"`
	}

	friendUsername struct {
		Username string `query:"username;updeq,userid_2"`
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

// NewInCtx creates a new chat repo from a context and sets it in the context
func NewInCtx(inj governor.Injector, table string) {
	SetCtxRepo(inj, NewCtx(inj, table))
}

// NewCtx creates a new chat repo from a context
func NewCtx(inj governor.Injector, table string) Repo {
	dbService := db.GetCtxDB(inj)
	return New(dbService, table)
}

// New creates a new user repository
func New(database db.Database, table string) Repo {
	return &repo{
		table: table,
		db:    database,
	}
}

func (r *repo) GetByID(userid1, userid2 string) (*Model, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := friendModelGetModelEqUserid1EqUserid2(d, r.table, userid1, userid2)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get friend")
	}
	return m, nil
}

func (r *repo) GetFriends(userid string, prefix string, limit, offset int) ([]Model, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	if prefix == "" {
		m, err := friendModelGetModelEqUserid1OrdUsername(d, r.table, userid, true, limit, offset)
		if err != nil {
			return nil, db.WrapErr(err, "Failed to get friends")
		}
		return m, nil
	}
	m, err := friendModelGetModelEqUserid1LikeUsernameOrdUsername(d, r.table, userid, prefix+"%", true, limit, offset)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get friends")
	}
	return m, nil
}

func (r *repo) Insert(userid1, userid2 string, username1, username2 string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := friendModelInsertBulk(d, r.table, []*Model{
		{
			Userid1:  userid1,
			Userid2:  userid2,
			Username: username2,
		},
		{
			Userid1:  userid2,
			Userid2:  userid1,
			Username: username1,
		},
	}, false); err != nil {
		return db.WrapErr(err, "Failed to add friend")
	}
	return nil
}

func (r *repo) Remove(userid1, userid2 string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if _, err := d.Exec("DELETE FROM "+r.table+" WHERE (userid_1 = $1 AND userid_2 = $2) OR (userid_1 = $2 AND userid_2 = $1);", userid1, userid2); err != nil {
		return db.WrapErr(err, "Failed to remove friend")
	}
	return nil
}

func (r *repo) UpdateUsername(userid, username string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := friendModelUpdfriendUsernameEqUserid2(d, r.table, &friendUsername{
		Username: username,
	}, userid); err != nil {
		return db.WrapErr(err, "Failed to update username")
	}
	return nil
}

func (r *repo) DeleteUser(userid string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := friendModelDelEqUserid2(d, r.table, userid); err != nil {
		return db.WrapErr(err, "Failed to delete friends")
	}
	if err := friendModelDelEqUserid1(d, r.table, userid); err != nil {
		return db.WrapErr(err, "Failed to delete friends")
	}
	return nil
}

func (r *repo) Setup() error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := friendModelSetup(d, r.table); err != nil {
		err = db.WrapErr(err, "Failed to setup friend model")
		if !errors.Is(err, db.ErrAuthz{}) {
			return err
		}
	}
	return nil
}
