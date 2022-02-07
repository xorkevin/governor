package model

import (
	"errors"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/uid"
)

//go:generate forge model -m Model -p gdm -o model_gen.go Model modelLastUpdated
//go:generate forge model -m MemberModel -p member -o modelmember_gen.go MemberModel modelLastUpdated
//go:generate forge model -m AssocModel -p assoc -o modelassoc_gen.go AssocModel modelLastUpdated

const (
	chatUIDSize = 16
)

type (
	Repo interface {
		New(name string, theme string) (*Model, error)
		GetByID(chatid string) (*Model, error)
		GetLatest(userid string, before int64, limit int) ([]string, error)
		GetChats(chatids []string) ([]Model, error)
		GetMembers(chatid string, userids []string) ([]string, error)
		GetChatsMembers(chatids []string, limit int) ([]MemberModel, error)
		GetUserChats(userid string, chatids []string) ([]string, error)
		GetMembersCount(chatid string) (int, error)
		GetAssocs(userid1, userid2 string, limit, offset int) ([]string, error)
		Insert(m *Model) error
		Update(m *Model) error
		UpdateLastUpdated(chatid string, t int64) error
		AddMembers(chatid string, userids []string) (int64, error)
		RmMembers(chatid string, userids []string) error
		Delete(chatid string) error
		Setup() error
	}

	repo struct {
		table        string
		tableMembers string
		tableAssoc   string
		db           db.Database
	}

	// Model is the db dm chat model
	Model struct {
		Chatid       string `model:"chatid,VARCHAR(31) PRIMARY KEY" query:"chatid;getoneeq,chatid;getgroupeq,chatid|arr;updeq,chatid;deleq,chatid"`
		Name         string `model:"name,VARCHAR(255) NOT NULL" query:"name"`
		Theme        string `model:"theme,VARCHAR(4095) NOT NULL" query:"theme"`
		LastUpdated  int64  `model:"last_updated,BIGINT NOT NULL" query:"last_updated"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL" query:"creation_time"`
	}

	modelLastUpdated struct {
		LastUpdated int64 `query:"last_updated;updeq,chatid"`
	}

	// MemberModel is the db chat member model
	MemberModel struct {
		Chatid      string `model:"chatid,VARCHAR(31);index,userid" query:"chatid;deleq,chatid;getgroupeq,userid,chatid|arr;getgroupeq,chatid|arr"`
		Userid      string `model:"userid,VARCHAR(31), PRIMARY KEY (chatid, userid)" query:"userid;getgroupeq,chatid,userid|arr;deleq,chatid,userid|arr"`
		LastUpdated int64  `model:"last_updated,BIGINT NOT NULL;index,userid" query:"last_updated;getgroupeq,userid;getgroupeq,userid,last_updated|lt"`
	}

	// AssocModel is the db chat association model
	AssocModel struct {
		Chatid      string `model:"chatid,VARCHAR(31)" query:"chatid;deleq,chatid"`
		Userid1     string `model:"userid_1,VARCHAR(31)" query:"userid_1;deleq,chatid,userid_1|arr"`
		Userid2     string `model:"userid_2,VARCHAR(31), PRIMARY KEY (chatid, userid_1, userid_2);index;index,chatid" query:"userid_2;deleq,chatid,userid_2|arr"`
		LastUpdated int64  `model:"last_updated,BIGINT NOT NULL;index,userid_1,userid_2" query:"last_updated;getgroupeq,userid_1,userid_2"`
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

func NewInCtx(inj governor.Injector, table, tableMembers, tableAssoc string) {
	SetCtxRepo(inj, NewCtx(inj, table, tableMembers, tableAssoc))
}

func NewCtx(inj governor.Injector, table, tableMembers, tableAssoc string) Repo {
	dbService := db.GetCtxDB(inj)
	return New(dbService, table, tableMembers, tableAssoc)
}

func New(database db.Database, table, tableMembers, tableAssoc string) Repo {
	return &repo{
		table:        table,
		tableMembers: tableMembers,
		tableAssoc:   tableAssoc,
		db:           database,
	}
}

// New creates new group chat
func (r *repo) New(name string, theme string) (*Model, error) {
	u, err := uid.New(chatUIDSize)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to create new uid")
	}
	now := time.Now().Round(0)
	return &Model{
		Chatid:       u.Base64(),
		Name:         name,
		Theme:        theme,
		LastUpdated:  now.UnixMilli(),
		CreationTime: now.Unix(),
	}, nil
}

// GetByID returns a group chat by id
func (r *repo) GetByID(chatid string) (*Model, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := gdmModelGetModelEqChatid(d, r.table, chatid)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get group chat")
	}
	return m, nil
}

// GetLatest returns latest group chats for a user
func (r *repo) GetLatest(userid string, before int64, limit int) ([]string, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	var m []MemberModel
	if before == 0 {
		var err error
		m, err = memberModelGetMemberModelEqUseridOrdLastUpdated(d, r.tableMembers, userid, false, limit, 0)
		if err != nil {
			return nil, db.WrapErr(err, "Failed to get latest group chats")
		}
	} else {
		var err error
		m, err = memberModelGetMemberModelEqUseridLtLastUpdatedOrdLastUpdated(d, r.tableMembers, userid, before, false, limit, 0)
		if err != nil {
			return nil, db.WrapErr(err, "Failed to get latest group chats")
		}
	}
	res := make([]string, 0, len(m))
	for _, i := range m {
		res = append(res, i.Chatid)
	}
	return res, nil
}

// GetChats returns gets group chats by id
func (r *repo) GetChats(chatids []string) ([]Model, error) {
	if len(chatids) == 0 {
		return nil, nil
	}

	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := gdmModelGetModelHasChatidOrdChatid(d, r.table, chatids, true, len(chatids), 0)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get group chats")
	}
	return m, nil
}

// GetMembers returns gets group chat members
func (r *repo) GetMembers(chatid string, userids []string) ([]string, error) {
	if len(userids) == 0 {
		return nil, nil
	}

	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := memberModelGetMemberModelEqChatidHasUseridOrdUserid(d, r.tableMembers, chatid, userids, true, len(userids), 0)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get group chat members")
	}
	res := make([]string, 0, len(m))
	for _, i := range m {
		res = append(res, i.Userid)
	}
	return res, nil
}

// GetChatsMembers returns gets group chats members
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
		return nil, db.WrapErr(err, "Failed to get group chat members")
	}
	return m, nil
}

func (r *repo) GetUserChats(userid string, chatids []string) ([]string, error) {
	if len(chatids) == 0 {
		return nil, nil
	}

	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := memberModelGetMemberModelEqUseridHasChatidOrdChatid(d, r.tableMembers, userid, chatids, true, len(chatids), 0)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get group chats")
	}
	res := make([]string, 0, len(m))
	for _, i := range m {
		res = append(res, i.Chatid)
	}
	return res, nil
}

// GetMembersCount returns the count of group chat members
func (r *repo) GetMembersCount(chatid string) (int, error) {
	var count int
	d, err := r.db.DB()
	if err != nil {
		return 0, err
	}
	if err := d.QueryRow("SELECT COUNT(*) FROM "+r.tableMembers+" WHERE chatid = $1;", chatid).Scan(&count); err != nil {
		return 0, db.WrapErr(err, "Failed to get group chat members count")
	}
	return count, nil
}

// GetAssocs returns user associated chats
func (r *repo) GetAssocs(userid1, userid2 string, limit, offset int) ([]string, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := assocModelGetAssocModelEqUserid1EqUserid2OrdLastUpdated(d, r.tableAssoc, userid1, userid2, false, limit, offset)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get group chats")
	}
	res := make([]string, 0, len(m))
	for _, i := range m {
		res = append(res, i.Chatid)
	}
	return res, nil
}

func (r *repo) Insert(m *Model) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := gdmModelInsert(d, r.table, m); err != nil {
		return db.WrapErr(err, "Failed to insert group chat")
	}
	return nil
}

func (r *repo) Update(m *Model) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := gdmModelUpdModelEqChatid(d, r.table, m, m.Chatid); err != nil {
		return db.WrapErr(err, "Failed to update group chat")
	}
	return nil
}

func (r *repo) UpdateLastUpdated(chatid string, t int64) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := gdmModelUpdmodelLastUpdatedEqChatid(d, r.table, &modelLastUpdated{
		LastUpdated: t,
	}, chatid); err != nil {
		return db.WrapErr(err, "Failed to update group chat last updated")
	}
	if err := memberModelUpdmodelLastUpdatedEqChatid(d, r.tableMembers, &modelLastUpdated{
		LastUpdated: t,
	}, chatid); err != nil {
		return db.WrapErr(err, "Failed to update group chat last updated")
	}
	if err := assocModelUpdmodelLastUpdatedEqChatid(d, r.tableAssoc, &modelLastUpdated{
		LastUpdated: t,
	}, chatid); err != nil {
		return db.WrapErr(err, "Failed to update group chat last updated")
	}
	return nil
}

func (r *repo) AddMembers(chatid string, userids []string) (int64, error) {
	if len(userids) == 0 {
		return 0, nil
	}

	d, err := r.db.DB()
	if err != nil {
		return 0, err
	}
	members := make([]*MemberModel, 0, len(userids))
	now := time.Now().Round(0).UnixMilli()
	for _, i := range userids {
		members = append(members, &MemberModel{
			Chatid:      chatid,
			Userid:      i,
			LastUpdated: now,
		})
	}
	if err := memberModelInsertBulk(d, r.tableMembers, members, false); err != nil {
		return 0, db.WrapErr(err, "Failed to add group chat members")
	}
	for _, i := range userids {
		if _, err := d.Exec("INSERT INTO "+r.tableAssoc+" (chatid, userid_1, userid_2, last_updated) SELECT chatid, $2::VARCHAR, userid, last_updated FROM "+r.tableMembers+" WHERE chatid = $1 AND userid <> $2 UNION ALL SELECT chatid, userid, $2::VARCHAR, last_updated FROM "+r.tableMembers+" WHERE chatid = $1 AND userid <> $2 ON CONFLICT DO NOTHING;", chatid, i); err != nil {
			return 0, db.WrapErr(err, "Failed to add group chat associations")
		}
	}
	return now, nil
}

func (r *repo) RmMembers(chatid string, userids []string) error {
	if len(userids) == 0 {
		return nil
	}

	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := assocModelDelEqChatidHasUserid1(d, r.tableAssoc, chatid, userids); err != nil {
		return db.WrapErr(err, "Failed to delete group chat associations")
	}
	if err := assocModelDelEqChatidHasUserid2(d, r.tableAssoc, chatid, userids); err != nil {
		return db.WrapErr(err, "Failed to delete group chat associations")
	}
	if err := memberModelDelEqChatidHasUserid(d, r.tableMembers, chatid, userids); err != nil {
		return db.WrapErr(err, "Failed to delete group chat members")
	}
	return nil
}

func (r *repo) Delete(chatid string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := assocModelDelEqChatid(d, r.tableAssoc, chatid); err != nil {
		return db.WrapErr(err, "Failed to delete group chat members")
	}
	if err := memberModelDelEqChatid(d, r.tableMembers, chatid); err != nil {
		return db.WrapErr(err, "Failed to delete group chat members")
	}
	if err := gdmModelDelEqChatid(d, r.table, chatid); err != nil {
		return db.WrapErr(err, "Failed to delete group chat")
	}
	return nil
}

// Setup creates new chat, member, and msg tables
func (r *repo) Setup() error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := gdmModelSetup(d, r.table); err != nil {
		err = db.WrapErr(err, "Failed to setup gdm model")
		if !errors.Is(err, db.ErrAuthz{}) {
			return err
		}
	}
	if err := memberModelSetup(d, r.tableMembers); err != nil {
		err = db.WrapErr(err, "Failed to setup gdm member model")
		if !errors.Is(err, db.ErrAuthz{}) {
			return err
		}
	}
	if err := assocModelSetup(d, r.tableAssoc); err != nil {
		err = db.WrapErr(err, "Failed to setup gdm assoc model")
		if !errors.Is(err, db.ErrAuthz{}) {
			return err
		}
	}
	return nil
}
