package friendmodel

import (
	"context"
	"errors"

	"xorkevin.dev/forge/model/sqldb"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/kerrors"
)

//go:generate forge model

type (
	Repo interface {
		GetByID(ctx context.Context, userid1, userid2 string) (*Model, error)
		GetFriends(ctx context.Context, userid string, prefix string, limit, offset int) ([]Model, error)
		GetFriendsByID(ctx context.Context, userid string, userids []string) ([]Model, error)
		Insert(ctx context.Context, userid1, userid2 string, username1, username2 string) error
		Remove(ctx context.Context, userid1, userid2 string) error
		UpdateUsername(ctx context.Context, userid, username string) error
		Setup(ctx context.Context) error
	}

	repo struct {
		table *friendModelTable
		db    db.Database
	}

	// Model is the db friend relationship model
	//forge:model friend
	//forge:model:query friend
	Model struct {
		Userid1  string `model:"userid_1,VARCHAR(31)"`
		Userid2  string `model:"userid_2,VARCHAR(31)"`
		Username string `model:"username,VARCHAR(255) NOT NULL"`
	}

	//forge:model:query friend
	friendUsername struct {
		Username string `model:"username"`
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
		table: &friendModelTable{
			TableName: table,
		},
		db: database,
	}
}

func (r *repo) GetByID(ctx context.Context, userid1, userid2 string) (*Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelByUser1User2(ctx, d, userid1, userid2)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get friend")
	}
	return m, nil
}

func (r *repo) GetFriends(ctx context.Context, userid string, prefix string, limit, offset int) ([]Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	if prefix == "" {
		m, err := r.table.GetModelByUser1(ctx, d, userid, limit, offset)
		if err != nil {
			return nil, kerrors.WithMsg(err, "Failed to get friends")
		}
		return m, nil
	}
	m, err := r.table.GetModelByUser1UsernamePrefix(ctx, d, userid, prefix+"%", limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get friends")
	}
	return m, nil
}

func (r *repo) GetFriendsByID(ctx context.Context, userid string, userids []string) ([]Model, error) {
	if len(userids) == 0 {
		return nil, nil
	}

	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelByUser1User2s(ctx, d, userid, userids, len(userids), 0)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get friends")
	}
	return m, nil
}

func (r *repo) Insert(ctx context.Context, userid1, userid2 string, username1, username2 string) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.InsertBulk(ctx, d, []*Model{
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
		return kerrors.WithMsg(err, "Failed to add friend")
	}
	return nil
}

func (t *friendModelTable) DelFriendPairs(ctx context.Context, d sqldb.Executor, userid1, userid2 string) error {
	if _, err := d.ExecContext(ctx, "DELETE FROM "+t.TableName+" WHERE (userid_1 = $1 AND userid_2 = $2) OR (userid_1 = $2 AND userid_2 = $1);", userid1, userid2); err != nil {
		return err
	}
	return nil
}

func (r *repo) Remove(ctx context.Context, userid1, userid2 string) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelFriendPairs(ctx, d, userid1, userid2); err != nil {
		return kerrors.WithMsg(err, "Failed to remove friend")
	}
	return nil
}

func (r *repo) UpdateUsername(ctx context.Context, userid, username string) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.UpdfriendUsernameByUser2(ctx, d, &friendUsername{
		Username: username,
	}, userid); err != nil {
		return kerrors.WithMsg(err, "Failed to update username")
	}
	return nil
}

func (r *repo) Setup(ctx context.Context) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.Setup(ctx, d); err != nil {
		err = kerrors.WithMsg(err, "Failed to setup friend model")
		if !errors.Is(err, db.ErrAuthz) {
			return err
		}
	}
	return nil
}
