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
		GetChat(chatid string) (*ChatModel, error)
		GetChats(chatids []string) ([]ChatModel, error)
		GetMembers(chatid string, limit, offset int) ([]MemberModel, error)
		GetRecentChats(userid string, limit, offset int) ([]MemberModel, error)
		GetRecentChatsByKind(userid string, kind string, limit, offset int) ([]MemberModel, error)
		AddMember(m *ChatModel, userid string) *MemberModel
		InsertChat(m *ChatModel) error
		UpdateChat(m *ChatModel) error
		UpdateChatLastUpdated(chatid string, t int64) error
		DeleteChat(m *ChatModel) error
		InsertMember(m *MemberModel) error
		DeleteChatMembers(chatid string) error
		DeleteMembers(chatid string, userids []string) error
		Setup() error
	}

	repo struct {
		db db.Database
	}

	// ChatModel is the db chat model
	ChatModel struct {
		Chatid       string `model:"chatid,VARCHAR(31) PRIMARY KEY" query:"chatid;getoneeq,chatid;getgroupeq,chatid|arr;updeq,chatid;deleq,chatid"`
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
		Msgid  string `model:"msgid,VARCHAR(31), PRIMARY KEY (chatid, msgid);index,chatid,kind" query:"msgid;getgroupeq,chatid;getgroupeq,chatid,msgid|lt;getgroupeq,chatid,kind;getgroupeq,chatid,kind,msgid|lt;deleq,chatid,msgid|arr;deleq,chatid"`
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

// GetChat returns a chat by id
func (r *repo) GetChat(chatid string) (*ChatModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, code, err := chatModelGetChatModelEqChatid(d, chatid)
	if err != nil {
		if code == 2 {
			return nil, governor.ErrWithKind(err, db.ErrNotFound{}, "No chat found with that id")
		}
		return nil, governor.ErrWithMsg(err, "Failed to get chat")
	}
	return m, nil
}

// GetChats returns a chat by id
func (r *repo) GetChats(chatids []string) ([]ChatModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := chatModelGetChatModelHasChatidOrdChatid(d, chatids, true, len(chatids), 0)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get chats")
	}
	return m, nil
}

// GetMembers returns chat members
func (r *repo) GetMembers(chatid string, limit, offset int) ([]MemberModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := memberModelGetMemberModelEqChatidOrdUserid(d, chatid, true, limit, offset)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get chat members")
	}
	return m, nil
}

// GetRecentChats returns most recent chats for a user
func (r *repo) GetRecentChats(userid string, limit, offset int) ([]MemberModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := memberModelGetMemberModelEqUseridOrdLastUpdated(d, userid, false, limit, offset)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get recent chats")
	}
	return m, nil
}

// GetRecentChatsByKind returns most recent chats for a user by kind
func (r *repo) GetRecentChatsByKind(userid string, kind string, limit, offset int) ([]MemberModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := memberModelGetMemberModelEqUseridEqKindOrdLastUpdated(d, userid, kind, false, limit, offset)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get recent chats of kind")
	}
	return m, nil
}

// AddMember adds a new chat member
func (r *repo) AddMember(m *ChatModel, userid string) *MemberModel {
	return &MemberModel{
		Chatid:      m.Chatid,
		Userid:      userid,
		Kind:        m.Kind,
		LastUpdated: time.Now().Round(0).UnixNano() / int64(time.Millisecond),
	}
}

// InsertChat inserts a new chat into the db
func (r *repo) InsertChat(m *ChatModel) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if code, err := chatModelInsert(d, m); err != nil {
		if code == 3 {
			return governor.ErrWithKind(err, db.ErrUnique{}, "Chat id must be unique")
		}
		return governor.ErrWithMsg(err, "Failed to insert chat")
	}
	return nil
}

// UpdateChat updates a chat in the db
func (r *repo) UpdateChat(m *ChatModel) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if code, err := chatModelUpdChatModelEqChatid(d, m, m.Chatid); err != nil {
		if code == 3 {
			return governor.ErrWithKind(err, db.ErrUnique{}, "Chat id must be unique")
		}
		return governor.ErrWithMsg(err, "Failed to update chat")
	}
	return nil
}

// UpdateChatLastUpdated updates a chat last updated for users
func (r *repo) UpdateChatLastUpdated(chatid string, t int64) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if code, err := memberModelUpdchatLastUpdatedEqChatid(d, &chatLastUpdated{
		LastUpdated: t,
	}, chatid); err != nil {
		if code == 3 {
			return governor.ErrWithKind(err, db.ErrUnique{}, "Chat id must be unique")
		}
		return governor.ErrWithMsg(err, "Failed to update chat last updated")
	}
	return nil
}

// DeleteChat deletes a chat in the db
func (r *repo) DeleteChat(m *ChatModel) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := chatModelDelEqChatid(d, m.Chatid); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete chat")
	}
	return nil
}

// InsertMember inserts a new chat member into the db
func (r *repo) InsertMember(m *MemberModel) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if code, err := memberModelInsert(d, m); err != nil {
		if code == 3 {
			return governor.ErrWithKind(err, db.ErrUnique{}, "User already added to chat")
		}
		return governor.ErrWithMsg(err, "Failed to insert chat member")
	}
	return nil
}

// DeleteChatMembers deletes all chat members
func (r *repo) DeleteChatMembers(chatid string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := memberModelDelEqChatid(d, chatid); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete chat members")
	}
	return nil
}

// DeleteMembers deletes chat members
func (r *repo) DeleteMembers(chatid string, userids []string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := memberModelDelEqChatidHasUserid(d, chatid, userids); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete chat members")
	}
	return nil
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
