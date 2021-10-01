package model

import (
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
)

//go:generate forge model -m ListModel -t mailinglists -p list -o modellist_gen.go ListModel
//go:generate forge model -m MemberModel -t mailinglistmembers -p member -o modelmember_gen.go MemberModel
//go:generate forge model -m MsgModel -t mailinglistmsgs -p msg -o modelmsg_gen.go MsgModel

type (
	Repo interface {
		NewList(listid string, creatorid string, name, desc string) *ListModel
		GetList(listid string) (*ListModel, error)
		GetLists(listids []string) ([]ListModel, error)
		GetCreatorLists(creatorid string, limit, offset int) ([]ListModel, error)
		GetCreatorListsBefore(creatorid string, before int64, limit int) ([]ListModel, error)
		InsertList(m *ListModel) error
		UpdateList(m *ListModel) error
		DeleteList(m *ListModel) error
		DeleteCreatorLists(creatorid string) error
		GetListMembers(listid string, limit, offset int) ([]MemberModel, error)
		GetUserLists(userid string, limit, offset int) ([]MemberModel, error)
		GetUserListsBefore(userid string, before int64, limit int) ([]MemberModel, error)
		AddMembers(m *ListModel, userids []string) []*MemberModel
		InsertMembers(m []*MemberModel) error
		DeleteMembers(listid string, userids []string) error
		DeleteListMembers(listid string) error
		DeleteUserMembers(userid string) error
		NewMsg(listid, msgid, userid string) *MsgModel
		GetMsg(listid, msgid string) (*MsgModel, error)
		GetListMsgs(listid string, limit, offset int) ([]MsgModel, error)
		GetListMsgsBefore(listid string, before int64, limit int) ([]MsgModel, error)
		InsertMsg(m *MsgModel) error
		DeleteMsgs(listid string, msgids []string) error
		DeleteListMsgs(listid string) error
	}

	repo struct {
		db db.Database
	}

	// ListModel is the db mailing list model
	ListModel struct {
		ListID       string `model:"listid,VARCHAR(255) PRIMARY KEY" query:"listid;getoneeq,listid;getgroupeq,listid|arr;updeq,listid;deleq,listid"`
		CreatorID    string `model:"creatorid,VARCHAR(31) NOT NULL" query:"creatorid;deleq,creatorid"`
		Name         string `model:"name,VARCHAR(255) NOT NULL" query:"name"`
		Description  string `model:"description,VARCHAR(255)" query:"description"`
		LastUpdated  int64  `model:"last_updated,BIGINT NOT NULL;index,creatorid" query:"last_updated;getgroupeq,creatorid;getgroupeq,creatorid,last_updated|lt"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL" query:"creation_time"`
	}

	// MemberModel is the db mailing list member model
	MemberModel struct {
		ListID      string `model:"listid,VARCHAR(255)" query:"listid;deleq,listid"`
		Userid      string `model:"userid,VARCHAR(31), PRIMARY KEY (listid, userid)" query:"userid;getgroupeq,listid;deleq,listid,userid|arr;deleq,userid"`
		LastUpdated int64  `model:"last_updated,BIGINT NOT NULL;index,userid" query:"last_updated;getgroupeq,userid;getgroupeq,userid,last_updated|lt"`
	}

	// MsgModel is the db mailing list message model
	MsgModel struct {
		ListID       string `model:"listid,VARCHAR(255)" query:"listid;deleq,listid"`
		Msgid        string `model:"msgid,VARCHAR(1023), PRIMARY KEY (listid, msgid)" query:"msgid;getoneeq,listid,msgid;deleq,listid,msgid|arr"`
		Userid       string `model:"userid,VARCHAR(31) NOT NULL" query:"userid"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL;index,listid" query:"creation_time;getgroupeq,listid;getgroupeq,listid,creation_time|lt"`
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

func (r *repo) NewList(listid string, creatorid string, name, desc string) *ListModel {
	now := time.Now().Round(0)
	return &ListModel{
		ListID:       listid,
		CreatorID:    creatorid,
		Name:         name,
		Description:  desc,
		LastUpdated:  now.UnixMilli(),
		CreationTime: now.Unix(),
	}
}

func (r *repo) GetList(listid string) (*ListModel, error) {
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

func (r *repo) GetCreatorListsBefore(creatorid string, before int64, limit int) ([]ListModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := listModelGetListModelEqCreatorIDLtLastUpdatedOrdLastUpdated(d, creatorid, before, false, limit, 0)
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

func (r *repo) GetListMembers(listid string, limit, offset int) ([]MemberModel, error) {
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

func (r *repo) GetUserLists(userid string, limit, offset int) ([]MemberModel, error) {
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

func (r *repo) GetUserListsBefore(userid string, before int64, limit int) ([]MemberModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := memberModelGetMemberModelEqUseridLtLastUpdatedOrdLastUpdated(d, userid, before, false, limit, 0)
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
	now := time.Now().Round(0).UnixMilli()
	m.LastUpdated = now
	for _, i := range userids {
		members = append(members, &MemberModel{
			ListID:      m.ListID,
			Userid:      i,
			LastUpdated: now,
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

func (r *repo) GetListMsgsBefore(listid string, before int64, limit int) ([]MsgModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := msgModelGetMsgModelEqListIDLtCreationTimeOrdCreationTime(d, listid, before, false, limit, 0)
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
