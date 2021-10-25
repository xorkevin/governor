package model

import (
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/uid"
)

//go:generate forge model -m ChatModel -t chats -p chat -o modelchat_gen.go ChatModel chatLastUpdated
//go:generate forge model -m MemberModel -t chatmembers -p member -o modelmember_gen.go MemberModel chatLastUpdated
//go:generate forge model -m MsgModel -t chatmessages -p msg -o modelmsg_gen.go MsgModel

const (
	chatUIDSize    = 16
	msgUIDRandSize = 8
)

type (
	Repo interface {
		NewChat(kind string, name string, theme string) (*ChatModel, error)
		GetChat(chatid string) (*ChatModel, error)
		GetChats(chatids []string) ([]ChatModel, error)
		GetMembers(chatid string, limit, offset int) ([]MemberModel, error)
		GetChatsMembers(chatids []string, limit int) ([]MemberModel, error)
		GetChatMembers(chatid string, userid []string) ([]MemberModel, error)
		GetUserChats(userid string, chatids []string) ([]MemberModel, error)
		GetMembersCount(chatid string) (int, error)
		GetLatestChats(userid string, limit, offset int) ([]MemberModel, error)
		GetLatestChatsByKind(kind string, userid string, limit, offset int) ([]MemberModel, error)
		GetLatestChatsBefore(userid string, before int64, limit int) ([]MemberModel, error)
		GetLatestChatsBeforeByKind(kind string, userid string, before int64, limit int) ([]MemberModel, error)
		AddMembers(m *ChatModel, userids []string) ([]*MemberModel, int64)
		InsertChat(m *ChatModel) error
		UpdateChat(m *ChatModel) error
		UpdateChatLastUpdated(chatid string, t int64) error
		DeleteChat(m *ChatModel) error
		InsertMembers(m []*MemberModel) error
		DeleteMembers(chatid string, userids []string) error
		DeleteChatMembers(chatid string) error
		GetMsgs(chatid string, limit, offset int) ([]MsgModel, error)
		GetMsgsBefore(chatid string, msgid string, limit int) ([]MsgModel, error)
		GetMsgsByKind(chatid string, kind string, limit, offset int) ([]MsgModel, error)
		GetMsgsBeforeByKind(chatid string, kind string, msgid string, limit int) ([]MsgModel, error)
		AddMsg(chatid string, userid string, kind string, value string) (*MsgModel, error)
		InsertMsg(m *MsgModel) error
		DeleteMsgs(chatid string, msgids []string) error
		DeleteChatMsgs(chatid string) error
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
		Chatid      string `model:"chatid,VARCHAR(31)" query:"chatid;deleq,chatid;getgroupeq,userid,chatid|arr;getgroupeq,chatid|arr"`
		Userid      string `model:"userid,VARCHAR(31), PRIMARY KEY (chatid, userid)" query:"userid;getgroupeq,chatid;getgroupeq,chatid,userid|arr;deleq,chatid,userid|arr"`
		Kind        string `model:"kind,VARCHAR(31) NOT NULL" query:"kind"`
		LastUpdated int64  `model:"last_updated,BIGINT NOT NULL;index,userid;index,userid,kind" query:"last_updated;getgroupeq,userid;getgroupeq,userid,kind;getgroupeq,userid,last_updated|lt;getgroupeq,userid,kind,last_updated|lt"`
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
		Kind   string `model:"kind,VARCHAR(31) NOT NULL" query:"kind"`
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
		LastUpdated:  now.UnixMilli(),
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
	if len(chatids) == 0 {
		return nil, nil
	}
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

// GetChatsMembers returns chat members
func (r *repo) GetChatsMembers(chatids []string, limit int) ([]MemberModel, error) {
	if len(chatids) == 0 {
		return nil, nil
	}
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := memberModelGetMemberModelHasChatidOrdChatid(d, chatids, true, limit, 0)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get chat members")
	}
	return m, nil
}

// GetChatMembers returns chat members
func (r *repo) GetChatMembers(chatid string, userids []string) ([]MemberModel, error) {
	if len(userids) == 0 {
		return nil, nil
	}
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := memberModelGetMemberModelEqChatidHasUseridOrdUserid(d, chatid, userids, true, len(userids), 0)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get chat members")
	}
	return m, nil
}

// GetUserChats returns a users chats
func (r *repo) GetUserChats(userid string, chatids []string) ([]MemberModel, error) {
	if len(chatids) == 0 {
		return nil, nil
	}
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := memberModelGetMemberModelEqUseridHasChatidOrdChatid(d, userid, chatids, true, len(chatids), 0)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get user chats")
	}
	return m, nil
}

const (
	sqlMemberCount = "SELECT COUNT(*) FROM " + memberModelTableName + " WHERE chatid = $1;"
)

// GetMembersCount returns the count of chat members
func (r *repo) GetMembersCount(chatid string) (int, error) {
	var count int
	d, err := r.db.DB()
	if err != nil {
		return 0, err
	}
	if err := d.QueryRow(sqlMemberCount, chatid).Scan(&count); err != nil {
		return 0, governor.ErrWithMsg(err, "Failed to get chat members count")
	}
	return count, nil
}

// GetLatestChats returns latest chats for a user
func (r *repo) GetLatestChats(userid string, limit, offset int) ([]MemberModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := memberModelGetMemberModelEqUseridOrdLastUpdated(d, userid, false, limit, offset)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get latest chats")
	}
	return m, nil
}

// GetLatestChatsByKind returns latest chats for a user by kind
func (r *repo) GetLatestChatsByKind(kind string, userid string, limit, offset int) ([]MemberModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := memberModelGetMemberModelEqUseridEqKindOrdLastUpdated(d, userid, kind, false, limit, offset)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get latest chats of kind")
	}
	return m, nil
}

// GetLatestChatsBefore returns latest chats for a user before a time
func (r *repo) GetLatestChatsBefore(userid string, before int64, limit int) ([]MemberModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := memberModelGetMemberModelEqUseridLtLastUpdatedOrdLastUpdated(d, userid, before, false, limit, 0)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get latest chats")
	}
	return m, nil
}

// GetLatestChatsBeforeByKind returns latest chats for a user by kind before a time
func (r *repo) GetLatestChatsBeforeByKind(kind string, userid string, before int64, limit int) ([]MemberModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := memberModelGetMemberModelEqUseridEqKindLtLastUpdatedOrdLastUpdated(d, userid, kind, before, false, limit, 0)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get latest chats of kind")
	}
	return m, nil
}

// AddMembers adds new chat members
func (r *repo) AddMembers(m *ChatModel, userids []string) ([]*MemberModel, int64) {
	if len(userids) == 0 {
		return nil, m.LastUpdated
	}
	members := make([]*MemberModel, 0, len(userids))
	now := time.Now().Round(0).UnixMilli()
	for _, i := range userids {
		members = append(members, &MemberModel{
			Chatid:      m.Chatid,
			Userid:      i,
			Kind:        m.Kind,
			LastUpdated: now,
		})
	}
	return members, now
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
	if _, err := chatModelUpdchatLastUpdatedEqChatid(d, &chatLastUpdated{
		LastUpdated: t,
	}, chatid); err != nil {
		return governor.ErrWithMsg(err, "Failed to update chat last updated")
	}
	if _, err := memberModelUpdchatLastUpdatedEqChatid(d, &chatLastUpdated{
		LastUpdated: t,
	}, chatid); err != nil {
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

// InsertMembers inserts a new chat member into the db
func (r *repo) InsertMembers(m []*MemberModel) error {
	if len(m) == 0 {
		return nil
	}
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if code, err := memberModelInsertBulk(d, m, false); err != nil {
		if code == 3 {
			return governor.ErrWithKind(err, db.ErrUnique{}, "User already added to chat")
		}
		return governor.ErrWithMsg(err, "Failed to insert chat members")
	}
	return nil
}

// DeleteMembers deletes chat members
func (r *repo) DeleteMembers(chatid string, userids []string) error {
	if len(userids) == 0 {
		return nil
	}
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := memberModelDelEqChatidHasUserid(d, chatid, userids); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete chat members")
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

// GetMsgs returns chat msgs
func (r *repo) GetMsgs(chatid string, limit, offset int) ([]MsgModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := msgModelGetMsgModelEqChatidOrdMsgid(d, chatid, false, limit, offset)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get chat messages")
	}
	return m, nil
}

// GetMsgsBefore returns chat msgs before a time
func (r *repo) GetMsgsBefore(chatid string, msgid string, limit int) ([]MsgModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := msgModelGetMsgModelEqChatidLtMsgidOrdMsgid(d, chatid, msgid, false, limit, 0)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get chat messages")
	}
	return m, nil
}

// GetMsgsByKind returns chat msgs of a kind
func (r *repo) GetMsgsByKind(chatid string, kind string, limit, offset int) ([]MsgModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := msgModelGetMsgModelEqChatidEqKindOrdMsgid(d, chatid, kind, false, limit, offset)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get chat messages")
	}
	return m, nil
}

// GetMsgsByKind returns chat msgs of a kind
func (r *repo) GetMsgsBeforeByKind(chatid string, kind string, msgid string, limit int) ([]MsgModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := msgModelGetMsgModelEqChatidEqKindLtMsgidOrdMsgid(d, chatid, kind, msgid, false, limit, 0)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get chat messages")
	}
	return m, nil
}

// AddMsg adds a new chat msg
func (r *repo) AddMsg(chatid string, userid string, kind string, value string) (*MsgModel, error) {
	u, err := uid.NewSnowflake(msgUIDRandSize)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to create new uid")
	}
	now := time.Now().Round(0).UnixMilli()
	return &MsgModel{
		Chatid: chatid,
		Msgid:  u.Base32(),
		Userid: userid,
		Timems: now,
		Kind:   kind,
		Value:  value,
	}, nil
}

// InsertMsg inserts a new chat msg into the db
func (r *repo) InsertMsg(m *MsgModel) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if code, err := msgModelInsert(d, m); err != nil {
		if code == 3 {
			return governor.ErrWithKind(err, db.ErrUnique{}, "Message id must be unique")
		}
		return governor.ErrWithMsg(err, "Failed to insert chat message")
	}
	return nil
}

// DeleteMsgs deletes chat messages
func (r *repo) DeleteMsgs(chatid string, msgids []string) error {
	if len(msgids) == 0 {
		return nil
	}
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := msgModelDelEqChatidHasMsgid(d, chatid, msgids); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete chat messages")
	}
	return nil
}

// DeleteChatMsgs deletes all chat messages
func (r *repo) DeleteChatMsgs(chatid string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := msgModelDelEqChatid(d, chatid); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete chat messages")
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
