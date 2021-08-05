package model

import (
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/uid"
)

//go:generate forge model -m ChatModel -t chats -p chat -o modelchat_gen.go ChatModel
//go:generate forge model -m MemberModel -t chatmembers -p member -o modelmember_gen.go MemberModel chatLastUpdated
//go:generate forge model -m MsgModel -t chatmessages -p msg -o modelmsg_gen.go MsgModel

const (
	chatUIDSize = 16
)

type (
	Repo interface {
		NewChat(kind string, name string, theme string) (*ChatModel, error)
		Setup() error
	}

	repo struct {
		db db.Database
	}

	// ChatModel is the db chat model
	ChatModel struct {
		Chatid       string `model:"chatid,VARCHAR(31) PRIMARY KEY" query:"chatid;getgroupeq,chatid|arr;updeq,chatid;deleq,chatid"`
		Kind         string `model:"kind,VARCHAR(31) NOT NULL" query:"kind"`
		Name         string `model:"name,VARCHAR(255) NOT NULL" query:"name"`
		Theme        string `model:"theme,VARCHAR(4095) NOT NULL" query:"theme"`
		LastUpdated  int64  `model:"last_updated,BIGINT NOT NULL" query:"last_updated"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL" query:"creation_time"`
	}

	// MemberModel is the db chat member model
	MemberModel struct {
		Chatid      string `model:"chatid,VARCHAR(31)" query:"chatid;deleq,chatid"`
		Userid      string `model:"userid,VARCHAR(31), PRIMARY KEY (chatid, userid)" query:"userid;getgroupeq,chatid;deleq,chatid,userid|arr"`
		Kind        string `model:"kind,VARCHAR(31) NOT NULL" query:"kind"`
		LastUpdated int64  `model:"last_updated,BIGINT NOT NULL;index,userid;index,userid,kind" query:"last_updated;getgroupeq,userid;getgroupeq,userid,kind"`
	}

	chatLastUpdated struct {
		LastUpdated int64 `query:"last_updated;updeq,chatid"`
	}

	// MsgModel is the db message model
	MsgModel struct {
		Chatid string `model:"chatid,VARCHAR(31)" query:"chatid"`
		Msgid  string `model:"msgid,VARCHAR(31), PRIMARY KEY (chatid, msgid);index,chatid,kind" query:"msgid;getgroupeq,chatid;getgroupeq,chatid,msgid|lt;getgroupeq,chatid,kind;getgroupeq,chatid,kind,msgid|lt;deleq,chatid,msgid;deleq,chatid"`
		Userid string `model:"userid,VARCHAR(31) NOT NULL" query:"userid"`
		Timems int64  `model:"time_ms,BIGINT NOT NULL" query:"time_ms"`
		Kind   int    `model:"kind,INTEGER NOT NULL" query:"kind"`
		Value  string `model:"value,VARCHAR(4095) NOT NULL" query:"value"`
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
func NewInCtx(inj governor.Injector) {
	SetCtxRepo(inj, NewCtx(inj))
}

// NewCtx creates a new chat repo from a context
func NewCtx(inj governor.Injector) Repo {
	dbService := db.GetCtxDB(inj)
	return New(dbService)
}

// New creates a new user repository
func New(database db.Database) Repo {
	return &repo{
		db: database,
	}
}

// NewChat creates new chat
func (r *repo) NewChat(kind string, name string, theme string) (*ChatModel, error) {
	u, err := uid.New(chatUIDSize)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to create new uid")
	}
	now := time.Now().Round(0)
	return &ChatModel{
		Chatid:       u.Base64(),
		Kind:         kind,
		Name:         name,
		Theme:        theme,
		LastUpdated:  now.UnixNano() / int64(time.Millisecond),
		CreationTime: now.Unix(),
	}, nil
}

// Setup creates new chat, member, and msg tables
func (r *repo) Setup() error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if code, err := chatModelSetup(d); err != nil {
		if code != 5 {
			return governor.ErrWithMsg(err, "Failed to setup chat model")
		}
	}
	if code, err := memberModelSetup(d); err != nil {
		if code != 5 {
			return governor.ErrWithMsg(err, "Failed to setup chat member model")
		}
	}
	if code, err := msgModelSetup(d); err != nil {
		if code != 5 {
			return governor.ErrWithMsg(err, "Failed to setup chat message model")
		}
	}
	return nil
}
