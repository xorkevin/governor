package msgmodel

import (
	"context"
	"errors"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/kerrors"
)

//go:generate forge model

const (
	msgUIDRandSize = 8
)

type (
	Repo interface {
		New(chatid string, userid string, kind string, value string) (*Model, error)
		GetMsgs(ctx context.Context, chatid string, kind string, msgid string, limit int) ([]Model, error)
		Insert(ctx context.Context, m *Model) error
		EraseMsgs(ctx context.Context, chatid string, msgids []string) error
		DeleteChatMsgs(ctx context.Context, chatid string) error
		Setup(ctx context.Context) error
	}

	repo struct {
		table *msgModelTable
		db    db.Database
	}

	// Model is the db chat msg model
	//forge:model msg
	//forge:model:query msg
	Model struct {
		Chatid string `model:"chatid,VARCHAR(31)"`
		Msgid  string `model:"msgid,VARCHAR(31)"`
		Userid string `model:"userid,VARCHAR(31) NOT NULL"`
		Timems int64  `model:"time_ms,BIGINT NOT NULL"`
		Kind   string `model:"kind,VARCHAR(31) NOT NULL"`
		Value  string `model:"value,VARCHAR(4095) NOT NULL"`
	}

	//forge:model:query msg
	msgValue struct {
		Value string `model:"value"`
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
		table: &msgModelTable{
			TableName: table,
		},
		db: database,
	}
}

func (r *repo) New(chatid string, userid string, kind string, value string) (*Model, error) {
	u, err := uid.NewSnowflake(msgUIDRandSize)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to create new uid")
	}
	return &Model{
		Chatid: chatid,
		Msgid:  u.Base32(),
		Userid: userid,
		Timems: time.Now().Round(0).UnixMilli(),
		Kind:   kind,
		Value:  value,
	}, nil
}

func (r *repo) GetMsgs(ctx context.Context, chatid string, kind string, msgid string, limit int) ([]Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	var m []Model
	if kind == "" {
		if msgid == "" {
			m, err = r.table.GetModelByChat(ctx, d, chatid, limit, 0)
		} else {
			m, err = r.table.GetModelByChatBeforeMsg(ctx, d, chatid, msgid, limit, 0)
		}
	} else {
		if msgid == "" {
			m, err = r.table.GetModelByChatKind(ctx, d, chatid, kind, limit, 0)
		} else {
			m, err = r.table.GetModelByChatKindBeforeMsg(ctx, d, chatid, kind, msgid, limit, 0)
		}
	}
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get chat msgs")
	}
	return m, nil
}

func (r *repo) Insert(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.Insert(ctx, d, m); err != nil {
		return kerrors.WithMsg(err, "Failed to insert chat msg")
	}
	return nil
}

func (r *repo) EraseMsgs(ctx context.Context, chatid string, msgids []string) error {
	if len(msgids) == 0 {
		return nil
	}

	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.UpdmsgValueByChatMsgs(ctx, d, &msgValue{
		Value: "",
	}, chatid, msgids); err != nil {
		return kerrors.WithMsg(err, "Failed to erase chat msgs")
	}
	return nil
}

func (r *repo) DeleteChatMsgs(ctx context.Context, chatid string) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelByChat(ctx, d, chatid); err != nil {
		return kerrors.WithMsg(err, "Failed to delete chat msgs")
	}
	return nil
}

func (r *repo) Setup(ctx context.Context) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.Setup(ctx, d); err != nil {
		err = kerrors.WithMsg(err, "Failed to setup chat msg model")
		if !errors.Is(err, db.ErrAuthz) {
			return err
		}
	}
	return nil
}
