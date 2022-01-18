package model

import (
	"errors"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/uid"
)

const (
	msgUIDRandSize = 8
)

//go:generate forge model -m Model -p msg -o model_gen.go Model msgValue

type (
	Repo interface {
		New(chatid string, userid string, kind string, value string) (*Model, error)
		GetMsgs(chatid string, kind string, msgid string, limit int) ([]Model, error)
		Insert(m *Model) error
		DeleteMsgs(chatid string, msgids []string) error
		DeleteChatMsgs(chatid string) error
		Setup() error
	}

	repo struct {
		table string
		db    db.Database
	}

	// Model is the db chat msg model
	Model struct {
		Chatid string `model:"chatid,VARCHAR(31)" query:"chatid"`
		Msgid  string `model:"msgid,VARCHAR(31), PRIMARY KEY (chatid, msgid);index,chatid,kind" query:"msgid;getgroupeq,chatid;getgroupeq,chatid,msgid|lt;getgroupeq,chatid,kind;getgroupeq,chatid,kind,msgid|lt;deleq,chatid"`
		Userid string `model:"userid,VARCHAR(31) NOT NULL" query:"userid"`
		Timems int64  `model:"time_ms,BIGINT NOT NULL" query:"time_ms"`
		Kind   string `model:"kind,VARCHAR(31) NOT NULL" query:"kind"`
		Value  string `model:"value,VARCHAR(4095) NOT NULL" query:"value"`
	}

	msgValue struct {
		Value string `query:"value;updeq,chatid,msgid|arr"`
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

func (r *repo) New(chatid string, userid string, kind string, value string) (*Model, error) {
	u, err := uid.NewSnowflake(msgUIDRandSize)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to create new uid")
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

func (r *repo) GetMsgs(chatid string, kind string, msgid string, limit int) ([]Model, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	var m []Model
	if kind == "" {
		if msgid == "" {
			m, err = msgModelGetModelEqChatidOrdMsgid(d, r.table, chatid, false, limit, 0)
		} else {
			m, err = msgModelGetModelEqChatidLtMsgidOrdMsgid(d, r.table, chatid, msgid, false, limit, 0)
		}
	} else {
		if msgid == "" {
			m, err = msgModelGetModelEqChatidEqKindOrdMsgid(d, r.table, chatid, kind, false, limit, 0)
		} else {
			m, err = msgModelGetModelEqChatidEqKindLtMsgidOrdMsgid(d, r.table, chatid, kind, msgid, false, limit, 0)
		}
	}
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get chat msgs")
	}
	return m, nil
}

func (r *repo) Insert(m *Model) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := msgModelInsert(d, r.table, m); err != nil {
		return db.WrapErr(err, "Failed to insert chat msg")
	}
	return nil
}

func (r *repo) DeleteMsgs(chatid string, msgids []string) error {
	if len(msgids) == 0 {
		return nil
	}
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := msgModelUpdmsgValueEqChatidHasMsgid(d, r.table, &msgValue{
		Value: "",
	}, chatid, msgids); err != nil {
		return db.WrapErr(err, "Failed to delete chat msgs")
	}
	return nil
}

func (r *repo) DeleteChatMsgs(chatid string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := msgModelDelEqChatid(d, r.table, chatid); err != nil {
		return db.WrapErr(err, "Failed to delete chat msgs")
	}
	return nil
}

func (r *repo) Setup() error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := msgModelSetup(d, r.table); err != nil {
		err = db.WrapErr(err, "Failed to setup chat msg model")
		if !errors.Is(err, db.ErrAuthz{}) {
			return err
		}
	}
	return nil
}
