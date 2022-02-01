package model

import (
	"errors"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/uid"
)

//go:generate forge model -m Model -p gdm -o model_gen.go Model
//go:generate forge model -m MemberModel -p member -o modelmember_gen.go MemberModel

const (
	chatUIDSize = 16
)

type (
	Repo interface {
		New(name string, theme string) (*Model, error)
		GetByID(chatid string) (*Model, error)
		GetLatest(userid string, before int64, limit int) ([]string, error)
		GetChats(chatids []string) ([]Model, error)
		Insert(m *Model) error
		Update(m *Model) error
		Delete(chatid string) error
		Setup() error
	}

	repo struct {
		table        string
		tableMembers string
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

	// MemberModel is the db chat member model
	MemberModel struct {
		Chatid      string `model:"chatid,VARCHAR(31);index,userid" query:"chatid;deleq,chatid;getgroupeq,userid,chatid|arr;getgroupeq,chatid|arr"`
		Userid      string `model:"userid,VARCHAR(31), PRIMARY KEY (chatid, userid)" query:"userid;deleq,userid;getgroupeq,chatid,userid|arr;deleq,chatid,userid|arr"`
		LastUpdated int64  `model:"last_updated,BIGINT NOT NULL;index,userid" query:"last_updated;getgroupeq,userid;getgroupeq,userid,last_updated|lt"`
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

func NewInCtx(inj governor.Injector, table string) {
	SetCtxRepo(inj, NewCtx(inj, table))
}

func NewCtx(inj governor.Injector, table string) Repo {
	dbService := db.GetCtxDB(inj)
	return New(dbService, table)
}

func New(database db.Database, table string) Repo {
	return &repo{
		table: table,
		db:    database,
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
		return nil, db.WrapErr(err, "Failed to get group chat")
	}
	return m, nil
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

func (r *repo) Delete(chatid string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
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
	return nil
}
