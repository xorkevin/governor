package gdmmodel

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/kerrors"
)

//go:generate forge model

const (
	chatUIDSize = 16
)

type (
	Repo interface {
		New(name string, theme string) (*Model, error)
		GetByID(ctx context.Context, chatid string) (*Model, error)
		GetLatest(ctx context.Context, userid string, before int64, limit int) ([]string, error)
		GetChats(ctx context.Context, chatids []string) ([]Model, error)
		GetMembers(ctx context.Context, chatid string, userids []string) ([]string, error)
		GetChatsMembers(ctx context.Context, chatids []string, limit int) ([]MemberModel, error)
		GetUserChats(ctx context.Context, userid string, chatids []string) ([]string, error)
		GetMembersCount(ctx context.Context, chatid string) (int, error)
		GetAssocs(ctx context.Context, userid1, userid2 string, limit, offset int) ([]string, error)
		Insert(ctx context.Context, m *Model) error
		UpdateProps(ctx context.Context, m *Model) error
		UpdateLastUpdated(ctx context.Context, chatid string, t int64) error
		AddMembers(ctx context.Context, chatid string, userids []string) (int64, error)
		RmMembers(ctx context.Context, chatid string, userids []string) error
		Delete(ctx context.Context, chatid string) error
		Setup(ctx context.Context) error
	}

	repo struct {
		table        *gdmModelTable
		tableMembers *memberModelTable
		tableAssoc   *assocModelTable
		db           db.Database
	}

	// Model is the db dm chat model
	//forge:model gdm
	//forge:model:query gdm
	Model struct {
		Chatid       string `model:"chatid,VARCHAR(31) PRIMARY KEY" query:"chatid;getoneeq,chatid;getgroupeq,chatid|in;updeq,chatid;deleq,chatid"`
		Name         string `model:"name,VARCHAR(255) NOT NULL" query:"name"`
		Theme        string `model:"theme,VARCHAR(4095) NOT NULL" query:"theme"`
		LastUpdated  int64  `model:"last_updated,BIGINT NOT NULL" query:"last_updated"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL" query:"creation_time"`
	}

	//forge:model:query gdm
	gdmProps struct {
		Name  string `query:"name;updeq,chatid"`
		Theme string `query:"theme"`
	}

	// MemberModel is the db chat member model
	//forge:model member
	//forge:model:query member
	MemberModel struct {
		Chatid      string `model:"chatid,VARCHAR(31);index,userid" query:"chatid;deleq,chatid;getgroupeq,userid,chatid|in;getgroupeq,chatid|in"`
		Userid      string `model:"userid,VARCHAR(31), PRIMARY KEY (chatid, userid)" query:"userid;getgroupeq,chatid,userid|in;deleq,chatid,userid|in"`
		LastUpdated int64  `model:"last_updated,BIGINT NOT NULL;index,userid" query:"last_updated;getgroupeq,userid;getgroupeq,userid,last_updated|lt"`
	}

	// AssocModel is the db chat association model
	//forge:model assoc
	//forge:model:query assoc
	AssocModel struct {
		Chatid      string `model:"chatid,VARCHAR(31)" query:"chatid;deleq,chatid"`
		Userid1     string `model:"userid_1,VARCHAR(31)" query:"userid_1;deleq,chatid,userid_1|in"`
		Userid2     string `model:"userid_2,VARCHAR(31), PRIMARY KEY (chatid, userid_1, userid_2);index;index,chatid" query:"userid_2;deleq,chatid,userid_2|in"`
		LastUpdated int64  `model:"last_updated,BIGINT NOT NULL;index,userid_1,userid_2" query:"last_updated;getgroupeq,userid_1,userid_2"`
	}

	//forge:model:query gdm
	//forge:model:query member
	//forge:model:query assoc
	modelLastUpdated struct {
		LastUpdated int64 `query:"last_updated;updeq,chatid"`
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
		table: &gdmModelTable{
			TableName: table,
		},
		tableMembers: &memberModelTable{
			TableName: tableMembers,
		},
		tableAssoc: &assocModelTable{
			TableName: tableAssoc,
		},
		db: database,
	}
}

// New creates new group chat
func (r *repo) New(name string, theme string) (*Model, error) {
	u, err := uid.New(chatUIDSize)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to create new uid")
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
func (r *repo) GetByID(ctx context.Context, chatid string) (*Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelEqChatid(ctx, d, chatid)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get group chat")
	}
	return m, nil
}

// GetLatest returns latest group chats for a user
func (r *repo) GetLatest(ctx context.Context, userid string, before int64, limit int) ([]string, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	var m []MemberModel
	if before == 0 {
		var err error
		m, err = r.tableMembers.GetMemberModelEqUseridOrdLastUpdated(ctx, d, userid, false, limit, 0)
		if err != nil {
			return nil, kerrors.WithMsg(err, "Failed to get latest group chats")
		}
	} else {
		var err error
		m, err = r.tableMembers.GetMemberModelEqUseridLtLastUpdatedOrdLastUpdated(ctx, d, userid, before, false, limit, 0)
		if err != nil {
			return nil, kerrors.WithMsg(err, "Failed to get latest group chats")
		}
	}
	res := make([]string, 0, len(m))
	for _, i := range m {
		res = append(res, i.Chatid)
	}
	return res, nil
}

// GetChats returns gets group chats by id
func (r *repo) GetChats(ctx context.Context, chatids []string) ([]Model, error) {
	if len(chatids) == 0 {
		return nil, nil
	}

	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelHasChatidOrdChatid(ctx, d, chatids, true, len(chatids), 0)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get group chats")
	}
	return m, nil
}

// GetMembers returns gets group chat members
func (r *repo) GetMembers(ctx context.Context, chatid string, userids []string) ([]string, error) {
	if len(userids) == 0 {
		return nil, nil
	}

	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.tableMembers.GetMemberModelEqChatidHasUseridOrdUserid(ctx, d, chatid, userids, true, len(userids), 0)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get group chat members")
	}
	res := make([]string, 0, len(m))
	for _, i := range m {
		res = append(res, i.Userid)
	}
	return res, nil
}

// GetChatsMembers returns gets group chats members
func (r *repo) GetChatsMembers(ctx context.Context, chatids []string, limit int) ([]MemberModel, error) {
	if len(chatids) == 0 {
		return nil, nil
	}

	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.tableMembers.GetMemberModelHasChatidOrdChatid(ctx, d, chatids, true, limit, 0)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get group chat members")
	}
	return m, nil
}

// GetUserChats returns a user's chats
func (r *repo) GetUserChats(ctx context.Context, userid string, chatids []string) ([]string, error) {
	if len(chatids) == 0 {
		return nil, nil
	}

	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.tableMembers.GetMemberModelEqUseridHasChatidOrdChatid(ctx, d, userid, chatids, true, len(chatids), 0)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get group chats")
	}
	res := make([]string, 0, len(m))
	for _, i := range m {
		res = append(res, i.Chatid)
	}
	return res, nil
}

func (t *memberModelTable) CountMembersEqChatid(ctx context.Context, d db.SQLExecutor, chatid string) (int, error) {
	var count int
	if err := d.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+t.TableName+" WHERE chatid = $1;", chatid).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// GetMembersCount returns the count of group chat members
func (r *repo) GetMembersCount(ctx context.Context, chatid string) (int, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return 0, err
	}
	count, err := r.tableMembers.CountMembersEqChatid(ctx, d, chatid)
	if err != nil {
		return 0, kerrors.WithMsg(err, "Failed to get group chat members count")
	}
	return count, nil
}

// GetAssocs returns user associated chats
func (r *repo) GetAssocs(ctx context.Context, userid1, userid2 string, limit, offset int) ([]string, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.tableAssoc.GetAssocModelEqUserid1EqUserid2OrdLastUpdated(ctx, d, userid1, userid2, false, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get group chats")
	}
	res := make([]string, 0, len(m))
	for _, i := range m {
		res = append(res, i.Chatid)
	}
	return res, nil
}

// Insert inserts a group chat
func (r *repo) Insert(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.Insert(ctx, d, m); err != nil {
		return kerrors.WithMsg(err, "Failed to insert group chat")
	}
	return nil
}

// UpdateProps updates a group chat
func (r *repo) UpdateProps(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.UpdgdmPropsEqChatid(ctx, d, &gdmProps{
		Name:  m.Name,
		Theme: m.Theme,
	}, m.Chatid); err != nil {
		return kerrors.WithMsg(err, "Failed to update group chat")
	}
	return nil
}

// UpdateLastUpdated updates a group chat last updated time
func (r *repo) UpdateLastUpdated(ctx context.Context, chatid string, t int64) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.UpdmodelLastUpdatedEqChatid(ctx, d, &modelLastUpdated{
		LastUpdated: t,
	}, chatid); err != nil {
		return kerrors.WithMsg(err, "Failed to update group chat last updated")
	}
	if err := r.tableMembers.UpdmodelLastUpdatedEqChatid(ctx, d, &modelLastUpdated{
		LastUpdated: t,
	}, chatid); err != nil {
		return kerrors.WithMsg(err, "Failed to update group chat last updated")
	}
	if err := r.tableAssoc.UpdmodelLastUpdatedEqChatid(ctx, d, &modelLastUpdated{
		LastUpdated: t,
	}, chatid); err != nil {
		return kerrors.WithMsg(err, "Failed to update group chat last updated")
	}
	return nil
}

// AddMembers adds members to a chat
func (r *repo) AddMembers(ctx context.Context, chatid string, userids []string) (int64, error) {
	if len(userids) == 0 {
		return 0, nil
	}

	d, err := r.db.DB(ctx)
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
	if err := r.tableMembers.InsertBulk(ctx, d, members, false); err != nil {
		return 0, kerrors.WithMsg(err, "Failed to add group chat members")
	}

	args := make([]interface{}, 0, len(userids)+1)
	args = append(args, chatid)
	var placeholdersid string
	{
		paramCount := 1
		placeholders := make([]string, 0, len(userids))
		for _, i := range userids {
			paramCount++
			placeholders = append(placeholders, fmt.Sprintf("($%d)", paramCount))
			args = append(args, i)
		}
		placeholdersid = strings.Join(placeholders, ", ")
	}

	if _, err := d.ExecContext(ctx, "INSERT INTO "+r.tableAssoc.TableName+" (chatid, userid_1, userid_2, last_updated) SELECT a.chatid, a.userid, b.userid, a.last_updated FROM "+r.tableMembers.TableName+" a INNER JOIN "+r.tableMembers.TableName+" b ON a.chatid = b.chatid WHERE a.chatid = $1 AND a.userid <> b.userid AND a.userid IN (VALUES "+placeholdersid+") UNION ALL SELECT a.chatid, a.userid, b.userid, a.last_updated FROM "+r.tableMembers.TableName+" a INNER JOIN "+r.tableMembers.TableName+" b ON a.chatid = b.chatid WHERE a.chatid = $1 AND a.userid <> b.userid AND b.userid IN (VALUES "+placeholdersid+") ON CONFLICT DO NOTHING;", args...); err != nil {
		return 0, kerrors.WithMsg(err, "Failed to add group chat associations")
	}
	return now, nil
}

func (r *repo) RmMembers(ctx context.Context, chatid string, userids []string) error {
	if len(userids) == 0 {
		return nil
	}

	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableAssoc.DelEqChatidHasUserid1(ctx, d, chatid, userids); err != nil {
		return kerrors.WithMsg(err, "Failed to delete group chat associations")
	}
	if err := r.tableAssoc.DelEqChatidHasUserid2(ctx, d, chatid, userids); err != nil {
		return kerrors.WithMsg(err, "Failed to delete group chat associations")
	}
	if err := r.tableMembers.DelEqChatidHasUserid(ctx, d, chatid, userids); err != nil {
		return kerrors.WithMsg(err, "Failed to delete group chat members")
	}
	return nil
}

func (r *repo) Delete(ctx context.Context, chatid string) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableAssoc.DelEqChatid(ctx, d, chatid); err != nil {
		return kerrors.WithMsg(err, "Failed to delete group chat members")
	}
	if err := r.tableMembers.DelEqChatid(ctx, d, chatid); err != nil {
		return kerrors.WithMsg(err, "Failed to delete group chat members")
	}
	if err := r.table.DelEqChatid(ctx, d, chatid); err != nil {
		return kerrors.WithMsg(err, "Failed to delete group chat")
	}
	return nil
}

// Setup creates new chat, member, and msg tables
func (r *repo) Setup(ctx context.Context) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.Setup(ctx, d); err != nil {
		err = kerrors.WithMsg(err, "Failed to setup gdm model")
		if !errors.Is(err, db.ErrAuthz) {
			return err
		}
	}
	if err := r.tableMembers.Setup(ctx, d); err != nil {
		err = kerrors.WithMsg(err, "Failed to setup gdm member model")
		if !errors.Is(err, db.ErrAuthz) {
			return err
		}
	}
	if err := r.tableAssoc.Setup(ctx, d); err != nil {
		err = kerrors.WithMsg(err, "Failed to setup gdm assoc model")
		if !errors.Is(err, db.ErrAuthz) {
			return err
		}
	}
	return nil
}
