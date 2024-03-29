package msgmodel

import (
	"context"
	"time"

	"xorkevin.dev/governor/service/dbsql"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/kerrors"
)

//go:generate forge model

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
		db    dbsql.Database
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
)

func New(database dbsql.Database, table string) Repo {
	return &repo{
		table: &msgModelTable{
			TableName: table,
		},
		db: database,
	}
}

func (r *repo) New(chatid string, userid string, kind string, value string) (*Model, error) {
	u, err := uid.New()
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to create new uid")
	}
	return &Model{
		Chatid: chatid,
		Msgid:  u.Base64(),
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
		return kerrors.WithMsg(err, "Failed to setup chat msg model")
	}
	return nil
}
