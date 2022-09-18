package model

import (
	"context"
	"errors"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/kerrors"
)

//go:generate forge model -m ListModel -p list -o modellist_gen.go ListModel listProps listLastUpdated
//go:generate forge model -m MemberModel -p member -o modelmember_gen.go MemberModel listLastUpdated
//go:generate forge model -m MsgModel -p msg -o modelmsg_gen.go MsgModel msgProcessed msgSent msgDeleted msgParent msgChildren
//go:generate forge model -m SentMsgModel -p sentmsg -o modelsentmsg_gen.go SentMsgModel
//go:generate forge model -m TreeModel -p tree -o modeltree_gen.go TreeModel

const (
	keySeparator = "."
)

type (
	Repo interface {
		NewList(creatorid, listname string, name, desc string, senderPolicy, memberPolicy string) *ListModel
		GetList(ctx context.Context, creatorid, listname string) (*ListModel, error)
		GetListByID(ctx context.Context, listid string) (*ListModel, error)
		GetLists(ctx context.Context, listids []string) ([]ListModel, error)
		GetCreatorLists(ctx context.Context, creatorid string, limit, offset int) ([]ListModel, error)
		InsertList(ctx context.Context, m *ListModel) error
		UpdateList(ctx context.Context, m *ListModel) error
		UpdateListLastUpdated(ctx context.Context, listid string, t int64) error
		DeleteList(ctx context.Context, m *ListModel) error
		DeleteCreatorLists(ctx context.Context, creatorid string) error
		GetMember(ctx context.Context, listid, userid string) (*MemberModel, error)
		GetMembers(ctx context.Context, listid string, limit, offset int) ([]MemberModel, error)
		GetListsMembers(ctx context.Context, listids []string, limit int) ([]MemberModel, error)
		GetListMembers(ctx context.Context, listid string, userids []string) ([]MemberModel, error)
		GetLatestLists(ctx context.Context, userid string, limit, offset int) ([]MemberModel, error)
		AddMembers(m *ListModel, userids []string) []*MemberModel
		InsertMembers(ctx context.Context, m []*MemberModel) error
		DeleteMembers(ctx context.Context, listid string, userids []string) error
		DeleteListMembers(ctx context.Context, listid string) error
		DeleteUserMembers(ctx context.Context, userid string) error
		NewMsg(listid, msgid, userid string) *MsgModel
		GetMsg(ctx context.Context, listid, msgid string) (*MsgModel, error)
		GetListMsgs(ctx context.Context, listid string, limit, offset int) ([]MsgModel, error)
		GetListThreads(ctx context.Context, listid string, limit, offset int) ([]MsgModel, error)
		GetListThread(ctx context.Context, listid, threadid string, limit, offset int) ([]MsgModel, error)
		InsertMsg(ctx context.Context, m *MsgModel) error
		UpdateMsgParent(ctx context.Context, listid, msgid string, parentid, threadid string) error
		UpdateMsgChildren(ctx context.Context, listid, parentid, threadid string) error
		UpdateMsgThread(ctx context.Context, listid, parentid, threadid string) error
		MarkMsgProcessed(ctx context.Context, listid, msgid string) error
		MarkMsgSent(ctx context.Context, listid, msgid string) error
		DeleteMsgs(ctx context.Context, listid string, msgids []string) error
		GetUnsentMsgs(ctx context.Context, listid, msgid string, limit int) ([]string, error)
		LogSentMsg(ctx context.Context, listid, msgid string, userids []string) error
		DeleteSentMsgLogs(ctx context.Context, listid string, msgid []string) error
		NewTree(listid, msgid string, t int64) *TreeModel
		GetTreeEdge(ctx context.Context, listid, msgid, parentid string) (*TreeModel, error)
		GetTreeChildren(ctx context.Context, listid, parentid string, depth int, limit, offset int) ([]TreeModel, error)
		GetTreeParents(ctx context.Context, listid, msgid string, limit, offset int) ([]TreeModel, error)
		InsertTree(ctx context.Context, m *TreeModel) error
		InsertTreeEdge(ctx context.Context, listid, msgid, parentid string) error
		InsertTreeChildren(ctx context.Context, listid, msgid string) error
		DeleteListTrees(ctx context.Context, listid string) error
		Setup(ctx context.Context) error
	}

	repo struct {
		tableLists   *listModelTable
		tableMembers *memberModelTable
		tableMsgs    *msgModelTable
		tableSent    *sentmsgModelTable
		tableTree    *treeModelTable
		db           db.Database
	}

	// ListModel is the db mailing list model
	ListModel struct {
		ListID       string `model:"listid,VARCHAR(255) PRIMARY KEY" query:"listid;getoneeq,listid;getgroupeq,listid|arr;deleq,listid"`
		CreatorID    string `model:"creatorid,VARCHAR(31) NOT NULL" query:"creatorid;deleq,creatorid"`
		Listname     string `model:"listname,VARCHAR(127) NOT NULL" query:"listname"`
		Name         string `model:"name,VARCHAR(255) NOT NULL" query:"name"`
		Description  string `model:"description,VARCHAR(255)" query:"description"`
		Archive      bool   `model:"archive,BOOLEAN NOT NULL" query:"archive"`
		SenderPolicy string `model:"sender_policy,VARCHAR(255) NOT NULL" query:"sender_policy"`
		MemberPolicy string `model:"member_policy,VARCHAR(255) NOT NULL" query:"member_policy"`
		LastUpdated  int64  `model:"last_updated,BIGINT NOT NULL;index,creatorid" query:"last_updated;getgroupeq,creatorid"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL" query:"creation_time"`
	}

	listProps struct {
		Name         string `query:"name;updeq,listid"`
		Description  string `query:"description"`
		Archive      bool   `query:"archive"`
		SenderPolicy string `query:"sender_policy"`
		MemberPolicy string `query:"member_policy"`
	}

	// MemberModel is the db mailing list member model
	MemberModel struct {
		ListID      string `model:"listid,VARCHAR(255)" query:"listid;deleq,listid;getgroupeq,listid|arr"`
		Userid      string `model:"userid,VARCHAR(31), PRIMARY KEY (listid, userid)" query:"userid;getoneeq,listid,userid;getgroupeq,listid;getgroupeq,listid,userid|arr;deleq,listid,userid|arr;deleq,userid"`
		LastUpdated int64  `model:"last_updated,BIGINT NOT NULL;index,userid" query:"last_updated;getgroupeq,userid"`
	}

	listLastUpdated struct {
		LastUpdated int64 `query:"last_updated;updeq,listid"`
	}

	// MsgModel is the db mailing list message model
	MsgModel struct {
		ListID       string `model:"listid,VARCHAR(255)" query:"listid"`
		Msgid        string `model:"msgid,VARCHAR(1023), PRIMARY KEY (listid, msgid)" query:"msgid;getoneeq,listid,msgid"`
		Userid       string `model:"userid,VARCHAR(31) NOT NULL" query:"userid"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL;index,listid;index,listid,thread_id" query:"creation_time;getgroupeq,listid;getgroupeq,listid,thread_id"`
		SPFPass      string `model:"spf_pass,VARCHAR(255) NOT NULL" query:"spf_pass"`
		DKIMPass     string `model:"dkim_pass,VARCHAR(255) NOT NULL" query:"dkim_pass"`
		Subject      string `model:"subject,VARCHAR(255) NOT NULL" query:"subject"`
		InReplyTo    string `model:"in_reply_to,VARCHAR(1023) NOT NULL;index,listid;index,listid,thread_id" query:"in_reply_to"`
		ParentID     string `model:"parent_id,VARCHAR(1023) NOT NULL" query:"parent_id"`
		ThreadID     string `model:"thread_id,VARCHAR(1023) NOT NULL" query:"thread_id"`
		Processed    bool   `model:"processed,BOOL NOT NULL" query:"processed"`
		Sent         bool   `model:"sent,BOOL NOT NULL" query:"sent"`
		Deleted      bool   `model:"deleted,BOOL NOT NULL" query:"deleted"`
	}

	msgProcessed struct {
		Processed bool `query:"processed;updeq,listid,msgid"`
	}

	msgSent struct {
		Sent bool `query:"sent;updeq,listid,msgid"`
	}

	msgDeleted struct {
		Userid   string `query:"userid"`
		SPFPass  string `query:"spf_pass"`
		DKIMPass string `query:"dkim_pass"`
		Subject  string `query:"subject"`
		Deleted  bool   `query:"deleted;updeq,listid,msgid|arr"`
	}

	msgParent struct {
		ParentID string `query:"parent_id"`
		ThreadID string `query:"thread_id;updeq,listid,msgid,thread_id"`
	}

	msgChildren struct {
		ParentID string `query:"parent_id"`
		ThreadID string `query:"thread_id;updeq,listid,thread_id,in_reply_to"`
	}

	// SentMsgModel is the db mailing list sent message log
	SentMsgModel struct {
		ListID   string `model:"listid,VARCHAR(255)" query:"listid"`
		Msgid    string `model:"msgid,VARCHAR(1023);index,listid,userid" query:"msgid;deleq,listid,msgid|arr"`
		Userid   string `model:"userid,VARCHAR(31), PRIMARY KEY (listid, msgid, userid)" query:"userid"`
		SentTime int64  `model:"sent_time,BIGINT NOT NULL" query:"sent_time"`
	}

	// TreeModel is the db mailing list message tree model
	TreeModel struct {
		ListID       string `model:"listid,VARCHAR(255)" query:"listid;deleq,listid"`
		Msgid        string `model:"msgid,VARCHAR(1023)" query:"msgid"`
		ParentID     string `model:"parent_id,VARCHAR(1023), PRIMARY KEY (listid, msgid, parent_id)" query:"parent_id;getoneeq,listid,msgid,parent_id"`
		Depth        int    `model:"depth,INT NOT NULL;index,listid,msgid" query:"depth;getgroupeq,listid,msgid"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL;index,listid,parent_id,depth" query:"creation_time;getgroupeq,listid,parent_id,depth"`
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
func NewInCtx(inj governor.Injector, tableLists, tableMembers, tableMsgs, tableSent, tableTree string) {
	SetCtxRepo(inj, NewCtx(inj, tableLists, tableMembers, tableMsgs, tableSent, tableTree))
}

// NewCtx creates a new chat repo from a context
func NewCtx(inj governor.Injector, tableLists, tableMembers, tableMsgs, tableSent, tableTree string) Repo {
	dbService := db.GetCtxDB(inj)
	return New(dbService, tableLists, tableMembers, tableMsgs, tableSent, tableTree)
}

// New creates a new user repository
func New(database db.Database, tableLists, tableMembers, tableMsgs, tableSent, tableTree string) Repo {
	return &repo{
		tableLists: &listModelTable{
			TableName: tableLists,
		},
		tableMembers: &memberModelTable{
			TableName: tableMembers,
		},
		tableMsgs: &msgModelTable{
			TableName: tableMsgs,
		},
		tableSent: &sentmsgModelTable{
			TableName: tableSent,
		},
		tableTree: &treeModelTable{
			TableName: tableTree,
		},
		db: database,
	}
}

func toListID(creatorid, listname string) string {
	return creatorid + keySeparator + listname
}

func (r *repo) NewList(creatorid, listname string, name, desc string, senderPolicy, memberPolicy string) *ListModel {
	now := time.Now().Round(0)
	return &ListModel{
		ListID:       toListID(creatorid, listname),
		CreatorID:    creatorid,
		Listname:     listname,
		Name:         name,
		Description:  desc,
		SenderPolicy: senderPolicy,
		MemberPolicy: memberPolicy,
		LastUpdated:  now.UnixMilli(),
		CreationTime: now.Unix(),
	}
}

func (r *repo) GetList(ctx context.Context, creatorid, listname string) (*ListModel, error) {
	return r.GetListByID(ctx, toListID(creatorid, listname))
}

func (r *repo) GetListByID(ctx context.Context, listid string) (*ListModel, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.tableLists.GetListModelEqListID(ctx, d, listid)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get list")
	}
	return m, nil
}

func (r *repo) GetLists(ctx context.Context, listids []string) ([]ListModel, error) {
	if len(listids) == 0 {
		return nil, nil
	}

	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.tableLists.GetListModelHasListIDOrdListID(ctx, d, listids, true, len(listids), 0)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get lists")
	}
	return m, nil
}

func (r *repo) GetCreatorLists(ctx context.Context, creatorid string, limit, offset int) ([]ListModel, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.tableLists.GetListModelEqCreatorIDOrdLastUpdated(ctx, d, creatorid, false, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get latest lists")
	}
	return m, nil
}

func (r *repo) InsertList(ctx context.Context, m *ListModel) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableLists.Insert(ctx, d, m); err != nil {
		return kerrors.WithMsg(err, "Failed to insert list")
	}
	return nil
}

func (r *repo) UpdateList(ctx context.Context, m *ListModel) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableLists.UpdlistPropsEqListID(ctx, d, &listProps{
		Name:         m.Name,
		Description:  m.Description,
		Archive:      m.Archive,
		SenderPolicy: m.SenderPolicy,
		MemberPolicy: m.MemberPolicy,
	}, m.ListID); err != nil {
		return kerrors.WithMsg(err, "Failed to update list")
	}
	return nil
}

func (r *repo) UpdateListLastUpdated(ctx context.Context, listid string, t int64) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableLists.UpdlistLastUpdatedEqListID(ctx, d, &listLastUpdated{
		LastUpdated: t,
	}, listid); err != nil {
		return kerrors.WithMsg(err, "Failed to update list last updated")
	}
	if err := r.tableMembers.UpdlistLastUpdatedEqListID(ctx, d, &listLastUpdated{
		LastUpdated: t,
	}, listid); err != nil {
		return kerrors.WithMsg(err, "Failed to update list last updated")
	}
	return nil
}

func (r *repo) DeleteList(ctx context.Context, m *ListModel) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableLists.DelEqListID(ctx, d, m.ListID); err != nil {
		return kerrors.WithMsg(err, "Failed to delete list")
	}
	return nil
}

func (r *repo) DeleteCreatorLists(ctx context.Context, creatorid string) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableLists.DelEqCreatorID(ctx, d, creatorid); err != nil {
		return kerrors.WithMsg(err, "Failed to delete lists")
	}
	return nil
}

func (r *repo) GetMember(ctx context.Context, listid, userid string) (*MemberModel, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.tableMembers.GetMemberModelEqListIDEqUserid(ctx, d, listid, userid)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get list member")
	}
	return m, nil
}

func (r *repo) GetMembers(ctx context.Context, listid string, limit, offset int) ([]MemberModel, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.tableMembers.GetMemberModelEqListIDOrdUserid(ctx, d, listid, true, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get list members")
	}
	return m, nil
}

func (r *repo) GetListsMembers(ctx context.Context, listids []string, limit int) ([]MemberModel, error) {
	if len(listids) == 0 {
		return nil, nil
	}

	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.tableMembers.GetMemberModelHasListIDOrdListID(ctx, d, listids, true, limit, 0)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get list members")
	}
	return m, nil
}

func (r *repo) GetListMembers(ctx context.Context, listid string, userids []string) ([]MemberModel, error) {
	if len(userids) == 0 {
		return nil, nil
	}

	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.tableMembers.GetMemberModelEqListIDHasUseridOrdUserid(ctx, d, listid, userids, true, len(userids), 0)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get list members")
	}
	return m, nil
}

func (r *repo) GetLatestLists(ctx context.Context, userid string, limit, offset int) ([]MemberModel, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.tableMembers.GetMemberModelEqUseridOrdLastUpdated(ctx, d, userid, false, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get latest user lists")
	}
	return m, nil
}

func (r *repo) AddMembers(m *ListModel, userids []string) []*MemberModel {
	if len(userids) == 0 {
		return nil
	}

	members := make([]*MemberModel, 0, len(userids))
	for _, i := range userids {
		members = append(members, &MemberModel{
			ListID:      m.ListID,
			Userid:      i,
			LastUpdated: m.LastUpdated,
		})
	}
	return members
}

func (r *repo) InsertMembers(ctx context.Context, m []*MemberModel) error {
	if len(m) == 0 {
		return nil
	}

	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableMembers.InsertBulk(ctx, d, m, false); err != nil {
		return kerrors.WithMsg(err, "Failed to insert list members")
	}
	return nil
}

func (r *repo) DeleteMembers(ctx context.Context, listid string, userids []string) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableMembers.DelEqListIDHasUserid(ctx, d, listid, userids); err != nil {
		return kerrors.WithMsg(err, "Failed to delete list members")
	}
	return nil
}

func (r *repo) DeleteListMembers(ctx context.Context, listid string) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableMembers.DelEqListID(ctx, d, listid); err != nil {
		return kerrors.WithMsg(err, "Failed to delete list members")
	}
	return nil
}

func (r *repo) DeleteUserMembers(ctx context.Context, userid string) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableMembers.DelEqUserid(ctx, d, userid); err != nil {
		return kerrors.WithMsg(err, "Failed to delete user list memberships")
	}
	return nil
}

func (r *repo) NewMsg(listid, msgid, userid string) *MsgModel {
	now := time.Now().Round(0).UnixMilli()
	return &MsgModel{
		ListID:       listid,
		Msgid:        msgid,
		Userid:       userid,
		CreationTime: now,
	}
}
func (r *repo) GetMsg(ctx context.Context, listid, msgid string) (*MsgModel, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.tableMsgs.GetMsgModelEqListIDEqMsgid(ctx, d, listid, msgid)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get list")
	}
	return m, nil
}

func (r *repo) GetListMsgs(ctx context.Context, listid string, limit, offset int) ([]MsgModel, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.tableMsgs.GetMsgModelEqListIDOrdCreationTime(ctx, d, listid, false, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get latest list messages")
	}
	return m, nil
}

func (r *repo) GetListThreads(ctx context.Context, listid string, limit, offset int) ([]MsgModel, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.tableMsgs.GetMsgModelEqListIDEqThreadIDOrdCreationTime(ctx, d, listid, "", false, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get latest list threads")
	}
	return m, nil
}

func (r *repo) GetListThread(ctx context.Context, listid, threadid string, limit, offset int) ([]MsgModel, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.tableMsgs.GetMsgModelEqListIDEqThreadIDOrdCreationTime(ctx, d, listid, threadid, true, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get list thread")
	}
	return m, nil
}

func (r *repo) InsertMsg(ctx context.Context, m *MsgModel) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableMsgs.Insert(ctx, d, m); err != nil {
		return kerrors.WithMsg(err, "Failed to insert list message")
	}
	return nil
}

func (r *repo) UpdateMsgParent(ctx context.Context, listid, msgid string, parentid, threadid string) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableMsgs.UpdmsgParentEqListIDEqMsgidEqThreadID(ctx, d, &msgParent{
		ParentID: parentid,
		ThreadID: threadid,
	}, listid, msgid, ""); err != nil {
		return kerrors.WithMsg(err, "Failed to update list message parent")
	}
	return nil
}

func (r *repo) UpdateMsgChildren(ctx context.Context, listid, parentid, threadid string) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableMsgs.UpdmsgChildrenEqListIDEqThreadIDEqInReplyTo(ctx, d, &msgChildren{
		ParentID: parentid,
		ThreadID: threadid,
	}, listid, "", parentid); err != nil {
		return kerrors.WithMsg(err, "Failed to update list message children")
	}
	return nil
}

func (t *msgModelTable) UpdMsgThreadEqListidEqInReplyTo(ctx context.Context, d db.SQLExecutor, listid, parentid, threadid string) error {
	if _, err := d.ExecContext(ctx, "UPDATE "+t.TableName+" SET (thread_id) = ROW($3) WHERE listid = $1 AND thread_id IN (SELECT msgid FROM "+t.TableName+" WHERE listid = $1 AND thread_id = '' AND in_reply_to = $2);", listid, parentid, threadid); err != nil {
		return err
	}
	return nil
}

func (r *repo) UpdateMsgThread(ctx context.Context, listid, parentid, threadid string) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableMsgs.UpdMsgThreadEqListidEqInReplyTo(ctx, d, listid, parentid, threadid); err != nil {
		return kerrors.WithMsg(err, "Failed to update list message thread")
	}
	return nil
}

func (r *repo) MarkMsgProcessed(ctx context.Context, listid, msgid string) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableMsgs.UpdmsgProcessedEqListIDEqMsgid(ctx, d, &msgProcessed{
		Processed: true,
	}, listid, msgid); err != nil {
		return kerrors.WithMsg(err, "Failed to update list message")
	}
	return nil
}

func (r *repo) MarkMsgSent(ctx context.Context, listid, msgid string) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableMsgs.UpdmsgSentEqListIDEqMsgid(ctx, d, &msgSent{
		Sent: true,
	}, listid, msgid); err != nil {
		return kerrors.WithMsg(err, "Failed to update list message")
	}
	return nil
}

func (r *repo) DeleteMsgs(ctx context.Context, listid string, msgids []string) error {
	if len(msgids) == 0 {
		return nil
	}

	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableMsgs.UpdmsgDeletedEqListIDHasMsgid(ctx, d, &msgDeleted{
		Userid:   "",
		SPFPass:  "",
		DKIMPass: "",
		Subject:  "",
		Deleted:  true,
	}, listid, msgids); err != nil {
		return kerrors.WithMsg(err, "Failed to mark list messages as deleted")
	}
	return nil
}

func (r *repo) GetUnsentMsgs(ctx context.Context, listid, msgid string, limit int) ([]string, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	res := make([]string, 0, limit)
	rows, err := d.QueryContext(ctx, "SELECT m.userid FROM "+r.tableMembers.TableName+" m LEFT JOIN "+r.tableSent.TableName+" s ON m.listid = s.listid AND m.userid = s.userid AND s.msgid = $3 WHERE m.listid = $2 AND s.msgid IS NULL LIMIT $1;", limit, listid, msgid)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get unsent list messages")
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, kerrors.WithMsg(err, "Failed to get unsent list messages")
		}
		res = append(res, s)
	}
	if err := rows.Err(); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get unsent list messages")
	}
	return res, nil
}

func (r *repo) LogSentMsg(ctx context.Context, listid, msgid string, userids []string) error {
	if len(userids) == 0 {
		return nil
	}

	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	now := time.Now().Round(0).Unix()
	m := make([]*SentMsgModel, 0, len(userids))
	for _, i := range userids {
		m = append(m, &SentMsgModel{
			ListID:   listid,
			Msgid:    msgid,
			Userid:   i,
			SentTime: now,
		})
	}
	if err := r.tableSent.InsertBulk(ctx, d, m, true); err != nil {
		return kerrors.WithMsg(err, "Failed to log sent messages")
	}
	return nil
}

func (r *repo) DeleteSentMsgLogs(ctx context.Context, listid string, msgids []string) error {
	if len(msgids) == 0 {
		return nil
	}

	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableSent.DelEqListIDHasMsgid(ctx, d, listid, msgids); err != nil {
		return kerrors.WithMsg(err, "Failed to delete sent message logs")
	}
	return nil
}

func (r *repo) NewTree(listid, msgid string, t int64) *TreeModel {
	return &TreeModel{
		ListID:       listid,
		Msgid:        msgid,
		ParentID:     msgid,
		Depth:        0,
		CreationTime: t,
	}
}

func (r *repo) GetTreeEdge(ctx context.Context, listid, msgid, parentid string) (*TreeModel, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.tableTree.GetTreeModelEqListIDEqMsgidEqParentID(ctx, d, listid, msgid, parentid)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get tree edge")
	}
	return m, nil
}

func (r *repo) GetTreeChildren(ctx context.Context, listid, parentid string, depth int, limit, offset int) ([]TreeModel, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.tableTree.GetTreeModelEqListIDEqParentIDEqDepthOrdCreationTime(ctx, d, listid, parentid, depth, true, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get tree children")
	}
	return m, nil
}

func (r *repo) GetTreeParents(ctx context.Context, listid, msgid string, limit, offset int) ([]TreeModel, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.tableTree.GetTreeModelEqListIDEqMsgidOrdDepth(ctx, d, listid, msgid, true, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get tree parents")
	}
	return m, nil
}

func (r *repo) InsertTree(ctx context.Context, m *TreeModel) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableTree.Insert(ctx, d, m); err != nil {
		return kerrors.WithMsg(err, "Failed to insert tree node")
	}
	return nil
}

func (t *treeModelTable) InsertTreeParentClosures(ctx context.Context, d db.SQLExecutor, listid, msgid, parentid string) error {
	if _, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (listid, msgid, parent_id, depth, creation_time) SELECT c.listid, c.msgid, p.parent_id, p.depth+c.depth+1, c.creation_time FROM "+t.TableName+" p INNER JOIN "+t.TableName+" c ON p.listid = c.listid WHERE p.listid = $1 AND p.msgid = $2 AND c.parent_id = $3 ON CONFLICT DO NOTHING;", listid, parentid, msgid); err != nil {
		return err
	}
	return nil
}

func (r *repo) InsertTreeEdge(ctx context.Context, listid, msgid, parentid string) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableTree.InsertTreeParentClosures(ctx, d, listid, msgid, parentid); err != nil {
		return kerrors.WithMsg(err, "Failed to insert tree edge")
	}
	return nil
}

func (r *repo) InsertTreeChildren(ctx context.Context, listid, msgid string) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if _, err := d.ExecContext(ctx, "INSERT INTO "+r.tableTree.TableName+" (listid, msgid, parent_id, depth, creation_time) SELECT c.listid, c.msgid, p.parent_id, p.depth+c.depth+1, c.creation_time FROM "+r.tableTree.TableName+" p INNER JOIN "+r.tableTree.TableName+" c ON p.listid = c.listid WHERE p.listid = $1 AND p.msgid = $2 AND c.parent_id IN (SELECT msgid FROM "+r.tableMsgs.TableName+" WHERE listid = $1 AND thread_id = '' AND in_reply_to = $2) ON CONFLICT DO NOTHING;", listid, msgid); err != nil {
		return kerrors.WithMsg(err, "Failed to insert tree children edges")
	}
	return nil
}

func (r *repo) DeleteListTrees(ctx context.Context, listid string) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableTree.DelEqListID(ctx, d, listid); err != nil {
		return kerrors.WithMsg(err, "Failed to delete list trees")
	}
	return nil
}

func (r *repo) Setup(ctx context.Context) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableLists.Setup(ctx, d); err != nil {
		err = kerrors.WithMsg(err, "Failed to setup list model")
		if !errors.Is(err, db.ErrorAuthz{}) {
			return err
		}
	}
	if err := r.tableMembers.Setup(ctx, d); err != nil {
		err = kerrors.WithMsg(err, "Failed to setup list member model")
		if !errors.Is(err, db.ErrorAuthz{}) {
			return err
		}
	}
	if err := r.tableMsgs.Setup(ctx, d); err != nil {
		err = kerrors.WithMsg(err, "Failed to setup list message model")
		if !errors.Is(err, db.ErrorAuthz{}) {
			return err
		}
	}
	if err := r.tableSent.Setup(ctx, d); err != nil {
		err = kerrors.WithMsg(err, "Failed to setup list sent message model")
		if !errors.Is(err, db.ErrorAuthz{}) {
			return err
		}
	}
	if err := r.tableTree.Setup(ctx, d); err != nil {
		err = kerrors.WithMsg(err, "Failed to setup list message model")
		if !errors.Is(err, db.ErrorAuthz{}) {
			return err
		}
	}
	return nil
}
