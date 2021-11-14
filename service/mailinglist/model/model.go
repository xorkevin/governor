package model

import (
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
)

//go:generate forge model -m ListModel -t mailinglists -p list -o modellist_gen.go ListModel listLastUpdated
//go:generate forge model -m MemberModel -t mailinglistmembers -p member -o modelmember_gen.go MemberModel listLastUpdated
//go:generate forge model -m MsgModel -t mailinglistmsgs -p msg -o modelmsg_gen.go MsgModel
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
		InsertMsg(m *MsgModel) error
		DeleteMsgs(listid string, msgids []string) error
		DeleteListMsgs(listid string) error
		NewTree(listid, msgid string, t int64) *TreeModel
		GetTreeEdge(listid, msgid, parentid string) (*TreeModel, error)
		GetTreeChildren(listid, parentid string, depth int, limit, offset int) ([]TreeModel, error)
		GetTreeParents(listid, msgid string, limit, offset int) ([]TreeModel, error)
		InsertTree(m *TreeModel) error
		DeleteListTree(listid string) error
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
		Msgid        string `model:"msgid,VARCHAR(1023), PRIMARY KEY (listid, msgid)" query:"msgid;getoneeq,listid,msgid;deleq,listid,msgid|arr"`
		Userid       string `model:"userid,VARCHAR(31) NOT NULL" query:"userid"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL;index,listid" query:"creation_time;getgroupeq,listid"`
		SPFPass      string `model:"spf_pass,VARCHAR(255) NOT NULL" query:"spf_pass"`
		DKIMPass     string `model:"dkim_pass,VARCHAR(255) NOT NULL" query:"dkim_pass"`
		Subject      string `model:"subject,VARCHAR(255) NOT NULL" query:"subject"`
		InReplyTo    string `model:"in_reply_to,VARCHAR(1023) NOT NULL" query:"in_reply_to"`
		ParentID     string `model:"parent_id,VARCHAR(1023) NOT NULL" query:"parent_id"`
		ThreadID     string `model:"thread_id,VARCHAR(1023) NOT NULL" query:"thread_id"`
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
	m, code, err := listModelGetListModelEqListID(d, listid)
	if err != nil {
		if code == 2 {
			return nil, governor.ErrWithKind(err, db.ErrNotFound{}, "No list found with that id")
		}
		return nil, governor.ErrWithMsg(err, "Failed to get list")
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
		return nil, governor.ErrWithMsg(err, "Failed to get lists")
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
		return nil, governor.ErrWithMsg(err, "Failed to get latest lists")
	}
	return m, nil
}

func (r *repo) InsertList(m *ListModel) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if code, err := listModelInsert(d, m); err != nil {
		if code == 3 {
			return governor.ErrWithKind(err, db.ErrUnique{}, "List id must be unique")
		}
		return governor.ErrWithMsg(err, "Failed to insert list")
	}
	return nil
}

func (r *repo) UpdateList(m *ListModel) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if code, err := listModelUpdListModelEqListID(d, m, m.ListID); err != nil {
		if code == 3 {
			return governor.ErrWithKind(err, db.ErrUnique{}, "List id must be unique")
		}
		return governor.ErrWithMsg(err, "Failed to update list")
	}
	return nil
}

func (r *repo) UpdateListLastUpdated(listid string, t int64) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if _, err := listModelUpdlistLastUpdatedEqListID(d, &listLastUpdated{
		LastUpdated: t,
	}, listid); err != nil {
		return governor.ErrWithMsg(err, "Failed to update list last updated")
	}
	if _, err := memberModelUpdlistLastUpdatedEqListID(d, &listLastUpdated{
		LastUpdated: t,
	}, listid); err != nil {
		return governor.ErrWithMsg(err, "Failed to update list last updated")
	}
	return nil
}

func (r *repo) DeleteList(m *ListModel) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := listModelDelEqListID(d, m.ListID); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete list")
	}
	return nil
}

func (r *repo) DeleteCreatorLists(creatorid string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := listModelDelEqCreatorID(d, creatorid); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete lists")
	}
	return nil
}

func (r *repo) GetMember(listid, userid string) (*MemberModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, code, err := memberModelGetMemberModelEqListIDEqUserid(d, listid, userid)
	if err != nil {
		if code == 2 {
			return nil, governor.ErrWithKind(err, db.ErrNotFound{}, "User is not list member")
		}
		return nil, governor.ErrWithMsg(err, "Failed to get list member")
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
		return nil, governor.ErrWithMsg(err, "Failed to get list members")
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
		return nil, governor.ErrWithMsg(err, "Failed to get list members")
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
		return nil, governor.ErrWithMsg(err, "Failed to get list members")
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
		return 0, governor.ErrWithMsg(err, "Failed to get list members count")
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
		return nil, governor.ErrWithMsg(err, "Failed to get latest user lists")
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
	if code, err := memberModelInsertBulk(d, m, false); err != nil {
		if code == 3 {
			return governor.ErrWithKind(err, db.ErrUnique{}, "User already added to list")
		}
		return governor.ErrWithMsg(err, "Failed to insert list members")
	}
	return nil
}

func (r *repo) DeleteMembers(listid string, userids []string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := memberModelDelEqListIDHasUserid(d, listid, userids); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete list members")
	}
	return nil
}

func (r *repo) DeleteListMembers(listid string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := memberModelDelEqListID(d, listid); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete list members")
	}
	return nil
}

func (r *repo) DeleteUserMembers(userid string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := memberModelDelEqUserid(d, userid); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete user list memberships")
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
	m, code, err := msgModelGetMsgModelEqListIDEqMsgid(d, listid, msgid)
	if err != nil {
		if code == 2 {
			return nil, governor.ErrWithKind(err, db.ErrNotFound{}, "No list found with that id")
		}
		return nil, governor.ErrWithMsg(err, "Failed to get list")
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
		return nil, governor.ErrWithMsg(err, "Failed to get latest list messages")
	}
	return m, nil
}

func (r *repo) InsertMsg(m *MsgModel) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if code, err := msgModelInsert(d, m); err != nil {
		if code == 3 {
			return governor.ErrWithKind(err, db.ErrUnique{}, "Msg id must be unique for list")
		}
		return governor.ErrWithMsg(err, "Failed to insert list message")
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
	if err := msgModelDelEqListIDHasMsgid(d, listid, msgids); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete list messages")
	}
	return nil
}

func (r *repo) DeleteListMsgs(listid string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := msgModelDelEqListID(d, listid); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete list messages")
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
	m, code, err := treeModelGetTreeModelEqListIDEqMsgidEqParentID(d, listid, msgid, parentid)
	if err != nil {
		if code == 2 {
			return nil, governor.ErrWithKind(err, db.ErrNotFound{}, "No tree edge found")
		}
		return nil, governor.ErrWithMsg(err, "Failed to get tree edge")
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
		return nil, governor.ErrWithMsg(err, "Failed to get tree children")
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
		return nil, governor.ErrWithMsg(err, "Failed to get tree parents")
	}
	return m, nil
}

func (r *repo) InsertTree(m *TreeModel) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if code, err := treeModelInsert(d, m); err != nil {
		if code == 3 {
			return governor.ErrWithKind(err, db.ErrUnique{}, "Parent and msg id already added")
		}
		return governor.ErrWithMsg(err, "Failed to insert tree edge")
	}
	return nil
}

func (r *repo) DeleteListTree(listid string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := treeModelDelEqListID(d, listid); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete list trees")
	}
	return nil
}

func (r *repo) Setup() error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if code, err := listModelSetup(d); err != nil {
		if code != 5 {
			return governor.ErrWithMsg(err, "Failed to setup list model")
		}
	}
	if code, err := memberModelSetup(d); err != nil {
		if code != 5 {
			return governor.ErrWithMsg(err, "Failed to setup list member model")
		}
	}
	if code, err := msgModelSetup(d); err != nil {
		if code != 5 {
			return governor.ErrWithMsg(err, "Failed to setup list message model")
		}
	}
	if code, err := treeModelSetup(d); err != nil {
		if code != 5 {
			return governor.ErrWithMsg(err, "Failed to setup list message model")
		}
	}
	return nil
}
