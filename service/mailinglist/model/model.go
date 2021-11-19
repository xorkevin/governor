package model

import (
	"errors"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
)

//go:generate forge model -m ListModel -t mailinglists -p list -o modellist_gen.go ListModel listLastUpdated
//go:generate forge model -m MemberModel -t mailinglistmembers -p member -o modelmember_gen.go MemberModel listLastUpdated
//go:generate forge model -m MsgModel -t mailinglistmsgs -p msg -o modelmsg_gen.go MsgModel msgProcessed msgDeleted msgParent msgChildren
//go:generate forge model -m TreeModel -t mailinglisttree -p tree -o modeltree_gen.go TreeModel

const (
	keySeparator = "."
)

type (
	Repo interface {
		NewList(creatorid, listname string, name, desc string, senderPolicy, memberPolicy string) *ListModel
		GetList(creatorid, listname string) (*ListModel, error)
		GetListByID(listid string) (*ListModel, error)
		GetLists(listids []string) ([]ListModel, error)
		GetCreatorLists(creatorid string, limit, offset int) ([]ListModel, error)
		InsertList(m *ListModel) error
		UpdateList(m *ListModel) error
		UpdateListLastUpdated(listid string, t int64) error
		DeleteList(m *ListModel) error
		DeleteCreatorLists(creatorid string) error
		GetMember(listid, userid string) (*MemberModel, error)
		GetMembers(listid string, limit, offset int) ([]MemberModel, error)
		GetListsMembers(listids []string, limit int) ([]MemberModel, error)
		GetListMembers(listid string, userids []string) ([]MemberModel, error)
		GetMembersCount(listid string) (int, error)
		GetLatestLists(userid string, limit, offset int) ([]MemberModel, error)
		AddMembers(m *ListModel, userids []string) []*MemberModel
		InsertMembers(m []*MemberModel) error
		DeleteMembers(listid string, userids []string) error
		DeleteListMembers(listid string) error
		DeleteUserMembers(userid string) error
		NewMsg(listid, msgid, userid string) *MsgModel
		GetMsg(listid, msgid string) (*MsgModel, error)
		GetListMsgs(listid string, limit, offset int) ([]MsgModel, error)
		GetListThreads(listid string, limit, offset int) ([]MsgModel, error)
		GetListThread(listid, threadid string, limit, offset int) ([]MsgModel, error)
		InsertMsg(m *MsgModel) error
		UpdateMsgParent(listid, msgid string, parentid, threadid string) error
		UpdateMsgChildren(listid, parentid, threadid string) error
		UpdateMsgThread(listid, parentid, threadid string) error
		MarkMsgProcessed(listid, msgid string) error
		DeleteMsgs(listid string, msgids []string) error
		DeleteListMsgs(listid string) error
		NewTree(listid, msgid string, t int64) *TreeModel
		GetTreeEdge(listid, msgid, parentid string) (*TreeModel, error)
		GetTreeChildren(listid, parentid string, depth int, limit, offset int) ([]TreeModel, error)
		GetTreeParents(listid, msgid string, limit, offset int) ([]TreeModel, error)
		InsertTree(m *TreeModel) error
		InsertTreeEdge(listid, msgid, parentid string) error
		InsertTreeChildren(listid, msgid string) error
		DeleteListTrees(listid string) error
		Setup() error
	}

	repo struct {
		db db.Database
	}

	// ListModel is the db mailing list model
	ListModel struct {
		ListID       string `model:"listid,VARCHAR(255) PRIMARY KEY" query:"listid;getoneeq,listid;getgroupeq,listid|arr;updeq,listid;deleq,listid"`
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
		ListID       string `model:"listid,VARCHAR(255)" query:"listid;deleq,listid"`
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
		Deleted      bool   `model:"deleted,BOOL NOT NULL" query:"deleted"`
	}

	msgProcessed struct {
		Processed bool `query:"processed;updeq,listid,msgid"`
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

func (r *repo) GetList(creatorid, listname string) (*ListModel, error) {
	return r.GetListByID(toListID(creatorid, listname))
}

func (r *repo) GetListByID(listid string) (*ListModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := listModelGetListModelEqListID(d, listid)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get list")
	}
	return m, nil
}

func (r *repo) GetLists(listids []string) ([]ListModel, error) {
	if len(listids) == 0 {
		return nil, nil
	}
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := listModelGetListModelHasListIDOrdListID(d, listids, true, len(listids), 0)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get lists")
	}
	return m, nil
}

func (r *repo) GetCreatorLists(creatorid string, limit, offset int) ([]ListModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := listModelGetListModelEqCreatorIDOrdLastUpdated(d, creatorid, false, limit, offset)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get latest lists")
	}
	return m, nil
}

func (r *repo) InsertList(m *ListModel) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := listModelInsert(d, m); err != nil {
		return db.WrapErr(err, "Failed to insert list")
	}
	return nil
}

func (r *repo) UpdateList(m *ListModel) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := listModelUpdListModelEqListID(d, m, m.ListID); err != nil {
		return db.WrapErr(err, "Failed to update list")
	}
	return nil
}

func (r *repo) UpdateListLastUpdated(listid string, t int64) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := listModelUpdlistLastUpdatedEqListID(d, &listLastUpdated{
		LastUpdated: t,
	}, listid); err != nil {
		return db.WrapErr(err, "Failed to update list last updated")
	}
	if err := memberModelUpdlistLastUpdatedEqListID(d, &listLastUpdated{
		LastUpdated: t,
	}, listid); err != nil {
		return db.WrapErr(err, "Failed to update list last updated")
	}
	return nil
}

func (r *repo) DeleteList(m *ListModel) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := listModelDelEqListID(d, m.ListID); err != nil {
		return db.WrapErr(err, "Failed to delete list")
	}
	return nil
}

func (r *repo) DeleteCreatorLists(creatorid string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := listModelDelEqCreatorID(d, creatorid); err != nil {
		return db.WrapErr(err, "Failed to delete lists")
	}
	return nil
}

func (r *repo) GetMember(listid, userid string) (*MemberModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := memberModelGetMemberModelEqListIDEqUserid(d, listid, userid)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get list member")
	}
	return m, nil
}

func (r *repo) GetMembers(listid string, limit, offset int) ([]MemberModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := memberModelGetMemberModelEqListIDOrdUserid(d, listid, true, limit, offset)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get list members")
	}
	return m, nil
}

func (r *repo) GetListsMembers(listids []string, limit int) ([]MemberModel, error) {
	if len(listids) == 0 {
		return nil, nil
	}
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := memberModelGetMemberModelHasListIDOrdListID(d, listids, true, limit, 0)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get list members")
	}
	return m, nil
}

func (r *repo) GetListMembers(listid string, userids []string) ([]MemberModel, error) {
	if len(userids) == 0 {
		return nil, nil
	}
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := memberModelGetMemberModelEqListIDHasUseridOrdUserid(d, listid, userids, true, len(userids), 0)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get list members")
	}
	return m, nil
}

const (
	sqlMemberCount = "SELECT COUNT(*) FROM " + memberModelTableName + " WHERE listid = $1;"
)

func (r *repo) GetMembersCount(listid string) (int, error) {
	var count int
	d, err := r.db.DB()
	if err != nil {
		return 0, err
	}
	if err := d.QueryRow(sqlMemberCount, listid).Scan(&count); err != nil {
		return 0, db.WrapErr(err, "Failed to get list members count")
	}
	return count, nil
}

func (r *repo) GetLatestLists(userid string, limit, offset int) ([]MemberModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := memberModelGetMemberModelEqUseridOrdLastUpdated(d, userid, false, limit, offset)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get latest user lists")
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

func (r *repo) InsertMembers(m []*MemberModel) error {
	if len(m) == 0 {
		return nil
	}
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := memberModelInsertBulk(d, m, false); err != nil {
		return db.WrapErr(err, "Failed to insert list members")
	}
	return nil
}

func (r *repo) DeleteMembers(listid string, userids []string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := memberModelDelEqListIDHasUserid(d, listid, userids); err != nil {
		return db.WrapErr(err, "Failed to delete list members")
	}
	return nil
}

func (r *repo) DeleteListMembers(listid string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := memberModelDelEqListID(d, listid); err != nil {
		return db.WrapErr(err, "Failed to delete list members")
	}
	return nil
}

func (r *repo) DeleteUserMembers(userid string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := memberModelDelEqUserid(d, userid); err != nil {
		return db.WrapErr(err, "Failed to delete user list memberships")
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
func (r *repo) GetMsg(listid, msgid string) (*MsgModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := msgModelGetMsgModelEqListIDEqMsgid(d, listid, msgid)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get list")
	}
	return m, nil
}

func (r *repo) GetListMsgs(listid string, limit, offset int) ([]MsgModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := msgModelGetMsgModelEqListIDOrdCreationTime(d, listid, false, limit, offset)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get latest list messages")
	}
	return m, nil
}

func (r *repo) GetListThreads(listid string, limit, offset int) ([]MsgModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := msgModelGetMsgModelEqListIDEqThreadIDOrdCreationTime(d, listid, "", false, limit, offset)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get latest list threads")
	}
	return m, nil
}

func (r *repo) GetListThread(listid, threadid string, limit, offset int) ([]MsgModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := msgModelGetMsgModelEqListIDEqThreadIDOrdCreationTime(d, listid, threadid, true, limit, offset)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get list thread")
	}
	return m, nil
}

func (r *repo) InsertMsg(m *MsgModel) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := msgModelInsert(d, m); err != nil {
		return db.WrapErr(err, "Failed to insert list message")
	}
	return nil
}

func (r *repo) UpdateMsgParent(listid, msgid string, parentid, threadid string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := msgModelUpdmsgParentEqListIDEqMsgidEqThreadID(d, &msgParent{
		ParentID: parentid,
		ThreadID: threadid,
	}, listid, msgid, ""); err != nil {
		return db.WrapErr(err, "Failed to update list message parent")
	}
	return nil
}

func (r *repo) UpdateMsgChildren(listid, parentid, threadid string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := msgModelUpdmsgChildrenEqListIDEqThreadIDEqInReplyTo(d, &msgChildren{
		ParentID: parentid,
		ThreadID: threadid,
	}, listid, "", parentid); err != nil {
		return db.WrapErr(err, "Failed to update list message children")
	}
	return nil
}

const (
	sqlMsgUpdateThread = "UPDATE " + msgModelTableName + " SET (thread_id) = ROW($3) WHERE listid = $1 AND thread_id IN (SELECT msgid FROM " + msgModelTableName + " WHERE listid = $1 AND thread_id = '' AND in_reply_to = $2);"
)

func (r *repo) UpdateMsgThread(listid, parentid, threadid string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if _, err := d.Exec(sqlMsgUpdateThread, listid, parentid, threadid); err != nil {
		return db.WrapErr(err, "Failed to update list message thread")
	}
	return nil
}

func (r *repo) MarkMsgProcessed(listid, msgid string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := msgModelUpdmsgProcessedEqListIDEqMsgid(d, &msgProcessed{
		Processed: true,
	}, listid, msgid); err != nil {
		return db.WrapErr(err, "Failed to update list message")
	}
	return nil
}

func (r *repo) DeleteMsgs(listid string, msgids []string) error {
	if len(msgids) == 0 {
		return nil
	}
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := msgModelUpdmsgDeletedEqListIDHasMsgid(d, &msgDeleted{
		Userid:   "",
		SPFPass:  "",
		DKIMPass: "",
		Subject:  "",
		Deleted:  true,
	}, listid, msgids); err != nil {
		return db.WrapErr(err, "Failed to mark list messages as deleted")
	}
	return nil
}

func (r *repo) DeleteListMsgs(listid string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := msgModelDelEqListID(d, listid); err != nil {
		return db.WrapErr(err, "Failed to delete list messages")
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

func (r *repo) GetTreeEdge(listid, msgid, parentid string) (*TreeModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := treeModelGetTreeModelEqListIDEqMsgidEqParentID(d, listid, msgid, parentid)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get tree edge")
	}
	return m, nil
}

func (r *repo) GetTreeChildren(listid, parentid string, depth int, limit, offset int) ([]TreeModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := treeModelGetTreeModelEqListIDEqParentIDEqDepthOrdCreationTime(d, listid, parentid, depth, true, limit, offset)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get tree children")
	}
	return m, nil
}

func (r *repo) GetTreeParents(listid, msgid string, limit, offset int) ([]TreeModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := treeModelGetTreeModelEqListIDEqMsgidOrdDepth(d, listid, msgid, true, limit, offset)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get tree parents")
	}
	return m, nil
}

func (r *repo) InsertTree(m *TreeModel) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := treeModelInsert(d, m); err != nil {
		return db.WrapErr(err, "Failed to insert tree node")
	}
	return nil
}

const (
	sqlTreeEdgeInsert = "INSERT INTO " + treeModelTableName + " (listid, msgid, parent_id, depth, creation_time) SELECT c.listid, c.msgid, p.parent_id, p.depth+c.depth+1, c.creation_time FROM " + treeModelTableName + " p INNER JOIN " + treeModelTableName + " c ON p.listid = $1 AND c.listid = $1 AND p.msgid = $2 AND c.parent_id = $3 ON CONFLICT DO NOTHING;"
)

func (r *repo) InsertTreeEdge(listid, msgid, parentid string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if _, err := d.Exec(sqlTreeEdgeInsert, listid, parentid, msgid); err != nil {
		return db.WrapErr(err, "Failed to insert tree edge")
	}
	return nil
}

const (
	sqlTreeChildrenInsert = "INSERT INTO " + treeModelTableName + " (listid, msgid, parent_id, depth, creation_time) SELECT c.listid, c.msgid, p.parent_id, p.depth+c.depth+1, c.creation_time FROM " + treeModelTableName + " p INNER JOIN " + treeModelTableName + " c ON p.listid = $1 AND c.listid = $1 AND p.msgid = $2 AND c.parent_id IN (SELECT msgid FROM " + msgModelTableName + " WHERE listid = $1 AND thread_id = '' AND in_reply_to = $2) ON CONFLICT DO NOTHING;"
)

func (r *repo) InsertTreeChildren(listid, msgid string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if _, err := d.Exec(sqlTreeChildrenInsert, listid, msgid); err != nil {
		return db.WrapErr(err, "Failed to insert tree children edges")
	}
	return nil
}

func (r *repo) DeleteListTrees(listid string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := treeModelDelEqListID(d, listid); err != nil {
		return db.WrapErr(err, "Failed to delete list trees")
	}
	return nil
}

func (r *repo) Setup() error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := listModelSetup(d); err != nil {
		err = db.WrapErr(err, "Failed to setup list model")
		if !errors.Is(err, db.ErrAuthz{}) {
			return err
		}
	}
	if err := memberModelSetup(d); err != nil {
		err = db.WrapErr(err, "Failed to setup list member model")
		if !errors.Is(err, db.ErrAuthz{}) {
			return err
		}
	}
	if err := msgModelSetup(d); err != nil {
		err = db.WrapErr(err, "Failed to setup list message model")
		if !errors.Is(err, db.ErrAuthz{}) {
			return err
		}
	}
	if err := treeModelSetup(d); err != nil {
		err = db.WrapErr(err, "Failed to setup list message model")
		if !errors.Is(err, db.ErrAuthz{}) {
			return err
		}
	}
	return nil
}
