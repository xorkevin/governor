package model

import (
	"context"
	"errors"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/kerrors"
)

//go:generate forge model -m Model -p org -o model_gen.go Model
//go:generate forge model -m MemberModel -p member -o modelmember_gen.go MemberModel orgName memberUsername

const (
	uidSize = 16
)

type (
	// Repo is a user org repository
	Repo interface {
		New(displayName, desc string) (*Model, error)
		GetByID(ctx context.Context, orgid string) (*Model, error)
		GetByName(ctx context.Context, orgname string) (*Model, error)
		GetAllOrgs(ctx context.Context, limit, offset int) ([]Model, error)
		GetOrgs(ctx context.Context, orgids []string) ([]Model, error)
		GetOrgMembers(ctx context.Context, orgid string, prefix string, limit, offset int) ([]MemberModel, error)
		GetOrgMods(ctx context.Context, orgid string, prefix string, limit, offset int) ([]MemberModel, error)
		GetUserOrgs(ctx context.Context, userid string, prefix string, limit, offset int) ([]string, error)
		GetUserMods(ctx context.Context, userid string, prefix string, limit, offset int) ([]string, error)
		Insert(ctx context.Context, m *Model) error
		Update(ctx context.Context, m *Model) error
		UpdateName(ctx context.Context, orgid string, name string) error
		AddMembers(ctx context.Context, m []*MemberModel) error
		AddMods(ctx context.Context, m []*MemberModel) error
		RmMembers(ctx context.Context, userid string, roles []string) error
		RmMods(ctx context.Context, userid string, roles []string) error
		UpdateUsername(ctx context.Context, userid string, username string) error
		Delete(ctx context.Context, m *Model) error
		Setup(ctx context.Context) error
	}

	repo struct {
		table        *orgModelTable
		tableMembers *memberModelTable
		tableMods    *memberModelTable
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
		OrgID    string `model:"orgid,VARCHAR(31)" query:"orgid;deleq,orgid"`
		Userid   string `model:"userid,VARCHAR(31), PRIMARY KEY (orgid, userid)" query:"userid;deleq,userid,orgid|arr"`
		Name     string `model:"name,VARCHAR(255) NOT NULL;index,userid" query:"name;getgroupeq,userid;getgroupeq,userid,name|like"`
		Username string `model:"username,VARCHAR(255) NOT NULL;index,orgid" query:"username;getgroupeq,orgid;getgroupeq,orgid,username|like"`
	}

	orgName struct {
		Name string `query:"name;updeq,orgid"`
	}

	memberUsername struct {
		Username string `query:"username;updeq,userid"`
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
func NewInCtx(inj governor.Injector, table, tableMembers, tableMods string) {
	SetCtxRepo(inj, NewCtx(inj, table, tableMembers, tableMods))
}

// NewCtx creates a new org repo from a context
func NewCtx(inj governor.Injector, table, tableMembers, tableMods string) Repo {
	dbService := db.GetCtxDB(inj)
	return New(dbService, table, tableMembers, tableMods)
}

// New creates a new OAuth app repository
func New(database db.Database, table, tableMembers, tableMods string) Repo {
	return &repo{
		table: &orgModelTable{
			TableName: table,
		},
		tableMembers: &memberModelTable{
			TableName: tableMembers,
		},
		tableMods: &memberModelTable{
			TableName: tableMods,
		},
		db: database,
	}
}

func (r *repo) New(displayName, desc string) (*Model, error) {
	mUID, err := uid.New(uidSize)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to create new uid")
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

func (r *repo) GetByID(ctx context.Context, orgid string) (*Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelEqOrgID(ctx, d, orgid)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get org")
	}
	return m, nil
}

func (r *repo) GetByName(ctx context.Context, orgname string) (*Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelEqName(ctx, d, orgname)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get org")
	}
	return m, nil
}
func (r *repo) GetAllOrgs(ctx context.Context, limit, offset int) ([]Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelOrdCreationTime(ctx, d, false, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get orgs")
	}
	return m, nil
}

func (r *repo) GetOrgs(ctx context.Context, orgids []string) ([]Model, error) {
	if len(orgids) == 0 {
		return nil, nil
	}

	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelHasOrgIDOrdOrgID(ctx, d, orgids, true, len(orgids), 0)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get orgs")
	}
	return m, nil
}

func (r *repo) GetOrgMembers(ctx context.Context, orgid string, prefix string, limit, offset int) ([]MemberModel, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	if prefix == "" {
		var err error
		m, err := r.tableMembers.GetMemberModelEqOrgIDOrdUsername(ctx, d, orgid, true, limit, offset)
		if err != nil {
			return nil, kerrors.WithMsg(err, "Failed to get org members")
		}
		return m, nil
	}
	m, err := r.tableMembers.GetMemberModelEqOrgIDLikeUsernameOrdUsername(ctx, d, orgid, prefix+"%", true, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get org members")
	}
	return m, nil
}

func (r *repo) GetOrgMods(ctx context.Context, orgid string, prefix string, limit, offset int) ([]MemberModel, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	if prefix == "" {
		var err error
		m, err := r.tableMods.GetMemberModelEqOrgIDOrdUsername(ctx, d, orgid, true, limit, offset)
		if err != nil {
			return nil, kerrors.WithMsg(err, "Failed to get org mods")
		}
		return m, nil
	}
	m, err := r.tableMods.GetMemberModelEqOrgIDLikeUsernameOrdUsername(ctx, d, orgid, prefix+"%", true, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get org mods")
	}
	return m, nil
}

func (r *repo) GetUserOrgs(ctx context.Context, userid string, prefix string, limit, offset int) ([]string, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	var m []MemberModel
	if prefix == "" {
		var err error
		m, err = r.tableMembers.GetMemberModelEqUseridOrdName(ctx, d, userid, true, limit, offset)
		if err != nil {
			return nil, kerrors.WithMsg(err, "Failed to get user orgs")
		}
	} else {
		var err error
		m, err = r.tableMembers.GetMemberModelEqUseridLikeNameOrdName(ctx, d, userid, prefix+"%", true, limit, offset)
		if err != nil {
			return nil, kerrors.WithMsg(err, "Failed to get user orgs")
		}
	}
	res := make([]string, 0, len(m))
	for _, i := range m {
		res = append(res, i.OrgID)
	}
	return res, nil
}

func (r *repo) GetUserMods(ctx context.Context, userid string, prefix string, limit, offset int) ([]string, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	var m []MemberModel
	if prefix == "" {
		var err error
		m, err = r.tableMods.GetMemberModelEqUseridOrdName(ctx, d, userid, true, limit, offset)
		if err != nil {
			return nil, kerrors.WithMsg(err, "Failed to get user orgs")
		}
	} else {
		var err error
		m, err = r.tableMods.GetMemberModelEqUseridLikeNameOrdName(ctx, d, userid, prefix+"%", true, limit, offset)
		if err != nil {
			return nil, kerrors.WithMsg(err, "Failed to get user orgs")
		}
	}
	res := make([]string, 0, len(m))
	for _, i := range m {
		res = append(res, i.OrgID)
	}
	return res, nil
}

func (r *repo) Insert(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.Insert(ctx, d, m); err != nil {
		return kerrors.WithMsg(err, "Failed to insert org")
	}
	return nil
}

func (r *repo) Update(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.UpdModelEqOrgID(ctx, d, m, m.OrgID); err != nil {
		return kerrors.WithMsg(err, "Failed to update org")
	}
	return nil
}

func (r *repo) UpdateName(ctx context.Context, orgid string, name string) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableMembers.UpdorgNameEqOrgID(ctx, d, &orgName{
		Name: name,
	}, orgid); err != nil {
		return kerrors.WithMsg(err, "Failed to update org name for members")
	}
	if err := r.tableMods.UpdorgNameEqOrgID(ctx, d, &orgName{
		Name: name,
	}, orgid); err != nil {
		return kerrors.WithMsg(err, "Failed to update org name for mods")
	}
	return nil
}

func (r *repo) AddMembers(ctx context.Context, m []*MemberModel) error {
	if len(m) == 0 {
		return nil
	}

	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableMembers.InsertBulk(ctx, d, m, true); err != nil {
		return kerrors.WithMsg(err, "Failed to add org members")
	}
	return nil
}

func (r *repo) AddMods(ctx context.Context, m []*MemberModel) error {
	if len(m) == 0 {
		return nil
	}

	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableMods.InsertBulk(ctx, d, m, true); err != nil {
		return kerrors.WithMsg(err, "Failed to add org mods")
	}
	return nil
}

func (r *repo) RmMembers(ctx context.Context, userid string, orgids []string) error {
	if len(orgids) == 0 {
		return nil
	}

	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableMembers.DelEqUseridHasOrgID(ctx, d, userid, orgids); err != nil {
		return kerrors.WithMsg(err, "Failed to remove org members")
	}
	return nil
}

func (r *repo) RmMods(ctx context.Context, userid string, orgids []string) error {
	if len(orgids) == 0 {
		return nil
	}

	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableMods.DelEqUseridHasOrgID(ctx, d, userid, orgids); err != nil {
		return kerrors.WithMsg(err, "Failed to remove org mods")
	}
	return nil
}

func (r *repo) UpdateUsername(ctx context.Context, userid string, username string) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableMembers.UpdmemberUsernameEqUserid(ctx, d, &memberUsername{
		Username: username,
	}, userid); err != nil {
		return kerrors.WithMsg(err, "Failed to update member username")
	}
	if err := r.tableMods.UpdmemberUsernameEqUserid(ctx, d, &memberUsername{
		Username: username,
	}, userid); err != nil {
		return kerrors.WithMsg(err, "Failed to update mod username")
	}
	return nil
}

func (r *repo) Delete(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableMembers.DelEqOrgID(ctx, d, m.OrgID); err != nil {
		return kerrors.WithMsg(err, "Failed to delete org members")
	}
	if err := r.tableMods.DelEqOrgID(ctx, d, m.OrgID); err != nil {
		return kerrors.WithMsg(err, "Failed to delete org mods")
	}
	if err := r.table.DelEqOrgID(ctx, d, m.OrgID); err != nil {
		return kerrors.WithMsg(err, "Failed to delete org")
	}
	return nil
}

func (r *repo) Setup(ctx context.Context) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.Setup(ctx, d); err != nil {
		err = kerrors.WithMsg(err, "Failed to setup org model")
		if !errors.Is(err, db.ErrorAuthz{}) {
			return err
		}
	}
	if err := r.tableMembers.Setup(ctx, d); err != nil {
		err = kerrors.WithMsg(err, "Failed to setup org member model")
		if !errors.Is(err, db.ErrorAuthz{}) {
			return err
		}
	}
	if err := r.tableMods.Setup(ctx, d); err != nil {
		err = kerrors.WithMsg(err, "Failed to setup org mod model")
		if !errors.Is(err, db.ErrorAuthz{}) {
			return err
		}
	}
	return nil
}
