package model

import (
	"errors"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/uid"
)

//go:generate forge model -m ChatModel -p chat -o modelchat_gen.go ChatModel chatLastUpdated
//go:generate forge model -m MemberModel -p member -o modelmember_gen.go MemberModel chatLastUpdated
//go:generate forge model -m MsgModel -p msg -o modelmsg_gen.go MsgModel
//go:generate forge model -m AssocModel -p assoc -o modelassoc_gen.go AssocModel chatLastUpdated
//go:generate forge model -m NameModel -p name -o modelname_gen.go NameModel

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
		GetLatestChatsByKind(kind string, userid string, limit, offset int) ([]MemberModel, error)
		GetLatestChatsBeforeByKind(kind string, userid string, before int64, limit int) ([]MemberModel, error)
		GetLatestChatsPrefix(kind string, userid string, prefix string, limit int) ([]string, error)
		AddMembers(m *ChatModel, userids []string) ([]*MemberModel, int64)
		InsertChat(m *ChatModel) error
		UpdateChat(m *ChatModel) error
		UpdateChatLastUpdated(chatid string, t int64) error
		DeleteChat(m *ChatModel) error
		InsertMembers(m []*MemberModel) error
		DeleteMembers(chatid string, userids []string) error
		DeleteChatMembers(chatid string) error
		DeleteUser(userid string) error
		GetMsgs(chatid string, limit, offset int) ([]MsgModel, error)
		GetMsgsBefore(chatid string, msgid string, limit int) ([]MsgModel, error)
		GetMsgsByKind(chatid string, kind string, limit, offset int) ([]MsgModel, error)
		GetMsgsBeforeByKind(chatid string, kind string, msgid string, limit int) ([]MsgModel, error)
		AddMsg(chatid string, userid string, kind string, value string) (*MsgModel, error)
		InsertMsg(m *MsgModel) error
		DeleteMsgs(chatid string, msgids []string) error
		DeleteChatMsgs(chatid string) error
		UpdateUsername(userid string, username string) error
		Setup() error
	}

	repo struct {
		tableChats   string
		tableMembers string
		tableMsgs    string
		tableAssoc   string
		tableName    string
		db           db.Database
	}

	// ChatModel is the db chat model
	ChatModel struct {
		Chatid       string `model:"chatid,VARCHAR(31) PRIMARY KEY" query:"chatid;getoneeq,chatid;getgroupeq,chatid|arr;updeq,chatid;deleq,chatid"`
		Kind         string `model:"kind,VARCHAR(31) NOT NULL;index" query:"kind"`
		Name         string `model:"name,VARCHAR(255) NOT NULL" query:"name"`
		Theme        string `model:"theme,VARCHAR(4095) NOT NULL" query:"theme"`
		LastUpdated  int64  `model:"last_updated,BIGINT NOT NULL" query:"last_updated"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL" query:"creation_time"`
	}

	// MemberModel is the db chat member model
	MemberModel struct {
		Chatid      string `model:"chatid,VARCHAR(31)" query:"chatid;deleq,chatid;getgroupeq,userid,chatid|arr;getgroupeq,chatid|arr"`
		Userid      string `model:"userid,VARCHAR(31), PRIMARY KEY (chatid, userid)" query:"userid;deleq,userid;getgroupeq,chatid;getgroupeq,chatid,userid|arr;deleq,chatid,userid|arr"`
		Kind        string `model:"kind,VARCHAR(31) NOT NULL" query:"kind"`
		LastUpdated int64  `model:"last_updated,BIGINT NOT NULL;index,userid,kind" query:"last_updated;getgroupeq,userid,kind;getgroupeq,userid,kind,last_updated|lt"`
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

	// AssocModel is the db chat association model
	AssocModel struct {
		Chatid      string `model:"chatid,VARCHAR(31)" query:"chatid;deleq,chatid"`
		Userid1     string `model:"userid_1,VARCHAR(31)" query:"userid_1;deleq,userid_1;deleq,chatid,userid_1|arr"`
		Userid2     string `model:"userid_2,VARCHAR(31), PRIMARY KEY (chatid, userid_1, userid_2);index;index,chatid" query:"userid_2;deleq,userid_2;deleq,chatid,userid_2|arr"`
		Kind        string `model:"kind,VARCHAR(31) NOT NULL" query:"kind"`
		LastUpdated int64  `model:"last_updated,BIGINT NOT NULL;index,userid_1,kind" query:"last_updated"`
	}

	// NameModel is the db user name model mirror
	NameModel struct {
		Userid   string `model:"userid,VARCHAR(31) PRIMARY KEY" query:"userid;deleq,userid"`
		Username string `model:"username,VARCHAR(255) NOT NULL" query:"username"`
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
func NewInCtx(inj governor.Injector, tableChats, tableMembers, tableMsgs, tableAssoc, tableName string) {
	SetCtxRepo(inj, NewCtx(inj, tableChats, tableMembers, tableMsgs, tableAssoc, tableName))
}

// NewCtx creates a new chat repo from a context
func NewCtx(inj governor.Injector, tableChats, tableMembers, tableMsgs, tableAssoc, tableName string) Repo {
	dbService := db.GetCtxDB(inj)
	return New(dbService, tableChats, tableMembers, tableMsgs, tableAssoc, tableName)
}

// New creates a new user repository
func New(database db.Database, tableChats, tableMembers, tableMsgs, tableAssoc, tableName string) Repo {
	return &repo{
		tableChats:   tableChats,
		tableMembers: tableMembers,
		tableMsgs:    tableMsgs,
		tableAssoc:   tableAssoc,
		tableName:    tableName,
		db:           database,
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
	m, err := chatModelGetChatModelEqChatid(d, r.tableChats, chatid)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get chat")
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
	m, err := chatModelGetChatModelHasChatidOrdChatid(d, r.tableChats, chatids, true, len(chatids), 0)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get chats")
	}
	return m, nil
}

// GetMembers returns chat members
func (r *repo) GetMembers(chatid string, limit, offset int) ([]MemberModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := memberModelGetMemberModelEqChatidOrdUserid(d, r.tableMembers, chatid, true, limit, offset)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get chat members")
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
	m, err := memberModelGetMemberModelHasChatidOrdChatid(d, r.tableMembers, chatids, true, limit, 0)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get chat members")
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
	m, err := memberModelGetMemberModelEqChatidHasUseridOrdUserid(d, r.tableMembers, chatid, userids, true, len(userids), 0)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get chat members")
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
	m, err := memberModelGetMemberModelEqUseridHasChatidOrdChatid(d, r.tableMembers, userid, chatids, true, len(chatids), 0)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get user chats")
	}
	return m, nil
}

// GetMembersCount returns the count of chat members
func (r *repo) GetMembersCount(chatid string) (int, error) {
	var count int
	d, err := r.db.DB()
	if err != nil {
		return 0, err
	}
	if err := d.QueryRow("SELECT COUNT(*) FROM "+r.tableMembers+" WHERE chatid = $1;", chatid).Scan(&count); err != nil {
		return 0, db.WrapErr(err, "Failed to get chat members count")
	}
	return count, nil
}

// GetLatestChatsByKind returns latest chats for a user by kind
func (r *repo) GetLatestChatsByKind(kind string, userid string, limit, offset int) ([]MemberModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := memberModelGetMemberModelEqUseridEqKindOrdLastUpdated(d, r.tableMembers, userid, kind, false, limit, offset)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get latest chats of kind")
	}
	return m, nil
}

// GetLatestChatsBeforeByKind returns latest chats for a user by kind before a time
func (r *repo) GetLatestChatsBeforeByKind(kind string, userid string, before int64, limit int) ([]MemberModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := memberModelGetMemberModelEqUseridEqKindLtLastUpdatedOrdLastUpdated(d, r.tableMembers, userid, kind, before, false, limit, 0)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get latest chats of kind")
	}
	return m, nil
}

// GetLatestChatsPrefix returns latest chats for a user by kind before a time
func (r *repo) GetLatestChatsPrefix(kind string, userid string, prefix string, limit int) ([]string, error) {
	if limit == 0 {
		return nil, nil
	}
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	res := make([]string, 0, limit)
	rows, err := d.Query("SELECT DISTINCT chatid FROM "+r.tableAssoc+" a INNER JOIN "+r.tableName+" n ON a.userid_2 = n.userid INNER JOIN "+r.tableChats+" c ON a.chatid = c.chatid WHERE a.userid_1 = $2 AND a.kind = $3 AND (n.username LIKE $4 OR c.name LIKE $4) ORDER BY a.last_updated DESC LIMIT $1;", limit, userid, kind, prefix+"%")
	if err != nil {
		return nil, db.WrapErr(err, "Failed to search chats of kind")
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, db.WrapErr(err, "Failed to search chats of kind")
		}
		res = append(res, s)
	}
	if err := rows.Err(); err != nil {
		return nil, db.WrapErr(err, "Failed to chats of kind")
	}
	return res, nil
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
	if err := chatModelInsert(d, r.tableChats, m); err != nil {
		return db.WrapErr(err, "Failed to insert chat")
	}
	return nil
}

// UpdateChat updates a chat in the db
func (r *repo) UpdateChat(m *ChatModel) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := chatModelUpdChatModelEqChatid(d, r.tableChats, m, m.Chatid); err != nil {
		return db.WrapErr(err, "Failed to update chat")
	}
	return nil
}

// UpdateChatLastUpdated updates a chat last updated for users
func (r *repo) UpdateChatLastUpdated(chatid string, t int64) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := chatModelUpdchatLastUpdatedEqChatid(d, r.tableChats, &chatLastUpdated{
		LastUpdated: t,
	}, chatid); err != nil {
		return db.WrapErr(err, "Failed to update chat last updated")
	}
	if err := memberModelUpdchatLastUpdatedEqChatid(d, r.tableMembers, &chatLastUpdated{
		LastUpdated: t,
	}, chatid); err != nil {
		return db.WrapErr(err, "Failed to update chat last updated")
	}
	if err := assocModelUpdchatLastUpdatedEqChatid(d, r.tableAssoc, &chatLastUpdated{
		LastUpdated: t,
	}, chatid); err != nil {
		return db.WrapErr(err, "Failed to update chat last updated")
	}
	return nil
}

// DeleteChat deletes a chat in the db
func (r *repo) DeleteChat(m *ChatModel) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := chatModelDelEqChatid(d, r.tableChats, m.Chatid); err != nil {
		return db.WrapErr(err, "Failed to delete chat")
	}
	return nil
}

// InsertMembers inserts a new chat member into the db
func (r *repo) InsertMembers(members []*MemberModel) error {
	if len(members) == 0 {
		return nil
	}
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	for _, m := range members {
		if _, err := d.Exec("INSERT INTO "+r.tableAssoc+" (chatid, userid_1, userid_2, kind, last_updated) SELECT chatid, $2, userid, kind, last_updated FROM "+r.tableMembers+" WHERE chatid = $1 AND userid <> $2 UNION ALL SELECT chatid, userid, $2, kind, last_updated FROM "+r.tableMembers+" WHERE chatid = $1 AND userid <> $2 ON CONFLICT DO NOTHING;", m.Chatid, m.Userid); err != nil {
			return db.WrapErr(err, "Failed to insert chat member associations")
		}
		if err := memberModelInsert(d, r.tableMembers, m); err != nil {
			return db.WrapErr(err, "Failed to insert chat member")
		}
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
	if err := assocModelDelEqChatidHasUserid2(d, r.tableAssoc, chatid, userids); err != nil {
		return db.WrapErr(err, "Failed to delete chat member associations")
	}
	if err := assocModelDelEqChatidHasUserid1(d, r.tableAssoc, chatid, userids); err != nil {
		return db.WrapErr(err, "Failed to delete chat member associations")
	}
	if err := memberModelDelEqChatidHasUserid(d, r.tableMembers, chatid, userids); err != nil {
		return db.WrapErr(err, "Failed to delete chat members")
	}
	return nil
}

// DeleteChatMembers deletes all chat members
func (r *repo) DeleteChatMembers(chatid string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := assocModelDelEqChatid(d, r.tableAssoc, chatid); err != nil {
		return db.WrapErr(err, "Failed to delete chat member associations")
	}
	if err := memberModelDelEqChatid(d, r.tableMembers, chatid); err != nil {
		return db.WrapErr(err, "Failed to delete chat members")
	}
	return nil
}

// DeleteUser deletes user memberships
func (r *repo) DeleteUser(userid string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := nameModelDelEqUserid(d, r.tableName, userid); err != nil {
		return db.WrapErr(err, "Failed to delete user member name")
	}
	if err := assocModelDelEqUserid2(d, r.tableAssoc, userid); err != nil {
		return db.WrapErr(err, "Failed to delete user member associations")
	}
	if err := assocModelDelEqUserid1(d, r.tableAssoc, userid); err != nil {
		return db.WrapErr(err, "Failed to delete user member associations")
	}
	if err := memberModelDelEqUserid(d, r.tableMembers, userid); err != nil {
		return db.WrapErr(err, "Failed to delete user memberships")
	}
	return nil
}

// GetMsgs returns chat msgs
func (r *repo) GetMsgs(chatid string, limit, offset int) ([]MsgModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := msgModelGetMsgModelEqChatidOrdMsgid(d, r.tableMsgs, chatid, false, limit, offset)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get chat messages")
	}
	return m, nil
}

// GetMsgsBefore returns chat msgs before a time
func (r *repo) GetMsgsBefore(chatid string, msgid string, limit int) ([]MsgModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := msgModelGetMsgModelEqChatidLtMsgidOrdMsgid(d, r.tableMsgs, chatid, msgid, false, limit, 0)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get chat messages")
	}
	return m, nil
}

// GetMsgsByKind returns chat msgs of a kind
func (r *repo) GetMsgsByKind(chatid string, kind string, limit, offset int) ([]MsgModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := msgModelGetMsgModelEqChatidEqKindOrdMsgid(d, r.tableMsgs, chatid, kind, false, limit, offset)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get chat messages")
	}
	return m, nil
}

// GetMsgsByKind returns chat msgs of a kind
func (r *repo) GetMsgsBeforeByKind(chatid string, kind string, msgid string, limit int) ([]MsgModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := msgModelGetMsgModelEqChatidEqKindLtMsgidOrdMsgid(d, r.tableMsgs, chatid, kind, msgid, false, limit, 0)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get chat messages")
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
	if err := msgModelInsert(d, r.tableMsgs, m); err != nil {
		return db.WrapErr(err, "Failed to insert chat message")
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
	if err := msgModelDelEqChatidHasMsgid(d, r.tableMsgs, chatid, msgids); err != nil {
		return db.WrapErr(err, "Failed to delete chat messages")
	}
	return nil
}

// DeleteChatMsgs deletes all chat messages
func (r *repo) DeleteChatMsgs(chatid string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := msgModelDelEqChatid(d, r.tableMsgs, chatid); err != nil {
		return db.WrapErr(err, "Failed to delete chat messages")
	}
	return nil
}

// UpdateUsername updates a user name
func (r *repo) UpdateUsername(userid string, username string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if _, err := d.Exec("INSERT INTO "+r.tableName+" (userid, username) VALUES ($1, $2) ON CONFLICT (userid) DO UPDATE SET username = excluded.username;", userid, username); err != nil {
		return db.WrapErr(err, "Failed to update user name")
	}
	return nil
}

// Setup creates new chat, member, and msg tables
func (r *repo) Setup() error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := chatModelSetup(d, r.tableChats); err != nil {
		err = db.WrapErr(err, "Failed to setup chat model")
		if !errors.Is(err, db.ErrAuthz{}) {
			return err
		}
	}
	if err := memberModelSetup(d, r.tableMembers); err != nil {
		err = db.WrapErr(err, "Failed to setup chat member model")
		if !errors.Is(err, db.ErrAuthz{}) {
			return err
		}
	}
	if err := msgModelSetup(d, r.tableMsgs); err != nil {
		err = db.WrapErr(err, "Failed to setup chat message model")
		if !errors.Is(err, db.ErrAuthz{}) {
			return err
		}
	}
	if err := assocModelSetup(d, r.tableAssoc); err != nil {
		err = db.WrapErr(err, "Failed to setup chat member association model")
		if !errors.Is(err, db.ErrAuthz{}) {
			return err
		}
	}
	if err := nameModelSetup(d, r.tableName); err != nil {
		err = db.WrapErr(err, "Failed to setup chat member name model")
		if !errors.Is(err, db.ErrAuthz{}) {
			return err
		}
	}
	return nil
}
