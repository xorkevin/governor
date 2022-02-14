package model

import (
	"errors"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/uid"
)

//go:generate forge model -m Model -p org -o model_gen.go Model
//go:generate forge model -m MemberModel -p member -o modelmember_gen.go MemberModel orgName

const (
	uidSize = 16
)

type (
	// Repo is a user org repository
	Repo interface {
		New(displayName, desc string) (*Model, error)
		GetByID(orgid string) (*Model, error)
		GetByName(orgname string) (*Model, error)
		GetAllOrgs(limit, offset int) ([]Model, error)
		GetOrgs(orgids []string) ([]Model, error)
		GetUserOrgs(userid string, prefix string, limit, offset int) ([]string, error)
		Insert(m *Model) error
		Update(m *Model) error
		AddMembers(m []*MemberModel) error
		RmMembers(userid string, roles []string) error
		Delete(m *Model) error
		Setup() error
	}

	repo struct {
		table        string
		tableMembers string
		db           db.Database
	}

	// Model is the user org model
	Model struct {
		OrgID        string `model:"orgid,VARCHAR(31) PRIMARY KEY" query:"orgid;getoneeq,orgid;getgroupeq,orgid|arr;updeq,orgid;deleq,orgid"`
		Name         string `model:"name,VARCHAR(255) NOT NULL UNIQUE" query:"name;getoneeq,name"`
		DisplayName  string `model:"display_name,VARCHAR(255) NOT NULL" query:"display_name"`
		Desc         string `model:"description,VARCHAR(255) NOT NULL" query:"description"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL;index" query:"creation_time;getgroup"`
	}

	// MemberModel is the user org member model
	MemberModel struct {
		OrgID  string `model:"orgid,VARCHAR(31)" query:"orgid;deleq,orgid"`
		Userid string `model:"userid,VARCHAR(31), PRIMARY KEY (orgid, userid)" query:"userid;deleq,userid,orgid|arr"`
		Name   string `model:"name,VARCHAR(255) NOT NULL;index,userid" query:"name;getgroupeq,userid;getgroupeq,userid,name|like"`
	}

	orgName struct {
		Name string `query:"name;updeq,orgid"`
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

// NewInCtx creates a new org repo from a context and sets it in the context
func NewInCtx(inj governor.Injector, table, tableMembers string) {
	SetCtxRepo(inj, NewCtx(inj, table, tableMembers))
}

// NewCtx creates a new org repo from a context
func NewCtx(inj governor.Injector, table, tableMembers string) Repo {
	dbService := db.GetCtxDB(inj)
	return New(dbService, table, tableMembers)
}

// New creates a new OAuth app repository
func New(database db.Database, table, tableMembers string) Repo {
	return &repo{
		table:        table,
		tableMembers: tableMembers,
		db:           database,
	}
}

func (r *repo) New(displayName, desc string) (*Model, error) {
	mUID, err := uid.New(uidSize)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to create new uid")
	}
	orgid := mUID.Base64()

	now := time.Now().Round(0).Unix()
	return &Model{
		OrgID:        orgid,
		Name:         orgid,
		DisplayName:  displayName,
		Desc:         desc,
		CreationTime: now,
	}, nil
}

func (r *repo) GetByID(orgid string) (*Model, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := orgModelGetModelEqOrgID(d, r.table, orgid)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get org")
	}
	return m, nil
}

func (r *repo) GetByName(orgname string) (*Model, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := orgModelGetModelEqName(d, r.table, orgname)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get org")
	}
	return m, nil
}
func (r *repo) GetAllOrgs(limit, offset int) ([]Model, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := orgModelGetModelOrdCreationTime(d, r.table, false, limit, offset)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get orgs")
	}
	return m, nil
}

func (r *repo) GetOrgs(orgids []string) ([]Model, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := orgModelGetModelHasOrgIDOrdOrgID(d, r.table, orgids, true, len(orgids), 0)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get orgs")
	}
	return m, nil
}

func (r *repo) GetUserOrgs(userid string, prefix string, limit, offset int) ([]string, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	var m []MemberModel
	if prefix == "" {
		var err error
		m, err = memberModelGetMemberModelEqUseridOrdName(d, r.tableMembers, userid, true, limit, offset)
		if err != nil {
			return nil, db.WrapErr(err, "Failed to get user orgs")
		}
	} else {
		var err error
		m, err = memberModelGetMemberModelEqUseridLikeNameOrdName(d, r.tableMembers, userid, prefix, true, limit, offset)
		if err != nil {
			return nil, db.WrapErr(err, "Failed to get user orgs")
		}
	}
	res := make([]string, 0, len(m))
	for _, i := range m {
		res = append(res, i.OrgID)
	}
	return res, nil
}

func (r *repo) Insert(m *Model) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := orgModelInsert(d, r.table, m); err != nil {
		return db.WrapErr(err, "Failed to insert org")
	}
	return nil
}

func (r *repo) Update(m *Model) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := orgModelUpdModelEqOrgID(d, r.table, m, m.OrgID); err != nil {
		return db.WrapErr(err, "Failed to update org")
	}
	return nil
}

func (r *repo) AddMembers(m []*MemberModel) error {
	if len(m) == 0 {
		return nil
	}

	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := memberModelInsertBulk(d, r.tableMembers, m, true); err != nil {
		return db.WrapErr(err, "Failed to add org members")
	}
	return nil
}

func (r *repo) RmMembers(userid string, orgids []string) error {
	if len(orgids) == 0 {
		return nil
	}

	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := memberModelDelEqUseridHasOrgID(d, r.tableMembers, userid, orgids); err != nil {
		return db.WrapErr(err, "Failed to remove org members")
	}
	return nil
}

func (r *repo) Delete(m *Model) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := memberModelDelEqOrgID(d, r.tableMembers, m.OrgID); err != nil {
		return db.WrapErr(err, "Failed to delete org members")
	}
	if err := orgModelDelEqOrgID(d, r.table, m.OrgID); err != nil {
		return db.WrapErr(err, "Failed to delete org")
	}
	return nil
}

func (r *repo) Setup() error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := orgModelSetup(d, r.table); err != nil {
		err = db.WrapErr(err, "Failed to setup org model")
		if !errors.Is(err, db.ErrAuthz{}) {
			return err
		}
	}
	if err := memberModelSetup(d, r.tableMembers); err != nil {
		err = db.WrapErr(err, "Failed to setup org member model")
		if !errors.Is(err, db.ErrAuthz{}) {
			return err
		}
	}
	return nil
}
