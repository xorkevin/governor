package orgmodel

import (
	"context"
	"errors"
	"time"

	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/kerrors"
)

//go:generate forge model

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
	//forge:model org
	//forge:model:query org
	Model struct {
		OrgID        string `model:"orgid,VARCHAR(31) PRIMARY KEY"`
		Name         string `model:"name,VARCHAR(255) NOT NULL UNIQUE"`
		DisplayName  string `model:"display_name,VARCHAR(255) NOT NULL"`
		Desc         string `model:"description,VARCHAR(255) NOT NULL"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL"`
	}

	// MemberModel is the user org member model
	//forge:model member
	//forge:model:query member
	MemberModel struct {
		OrgID    string `model:"orgid,VARCHAR(31)"`
		Userid   string `model:"userid,VARCHAR(31)"`
		Name     string `model:"name,VARCHAR(255) NOT NULL"`
		Username string `model:"username,VARCHAR(255) NOT NULL"`
	}

	//forge:model:query member
	orgName struct {
		Name string `model:"name"`
	}

	//forge:model:query member
	memberUsername struct {
		Username string `model:"username"`
	}
)

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
	m, err := r.table.GetModelByID(ctx, d, orgid)
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
	m, err := r.table.GetModelByName(ctx, d, orgname)
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
	m, err := r.table.GetModelAll(ctx, d, limit, offset)
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
	m, err := r.table.GetModelByIDs(ctx, d, orgids, len(orgids), 0)
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
		m, err := r.tableMembers.GetMemberModelByOrgid(ctx, d, orgid, limit, offset)
		if err != nil {
			return nil, kerrors.WithMsg(err, "Failed to get org members")
		}
		return m, nil
	}
	m, err := r.tableMembers.GetMemberModelByOrgUsernamePrefix(ctx, d, orgid, prefix+"%", limit, offset)
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
		m, err := r.tableMods.GetMemberModelByOrgid(ctx, d, orgid, limit, offset)
		if err != nil {
			return nil, kerrors.WithMsg(err, "Failed to get org mods")
		}
		return m, nil
	}
	m, err := r.tableMods.GetMemberModelByOrgUsernamePrefix(ctx, d, orgid, prefix+"%", limit, offset)
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
		m, err = r.tableMembers.GetMemberModelByUserid(ctx, d, userid, limit, offset)
		if err != nil {
			return nil, kerrors.WithMsg(err, "Failed to get user orgs")
		}
	} else {
		var err error
		m, err = r.tableMembers.GetMemberModelByUserOrgNamePrefix(ctx, d, userid, prefix+"%", limit, offset)
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
		m, err = r.tableMods.GetMemberModelByUserid(ctx, d, userid, limit, offset)
		if err != nil {
			return nil, kerrors.WithMsg(err, "Failed to get user orgs")
		}
	} else {
		var err error
		m, err = r.tableMods.GetMemberModelByUserOrgNamePrefix(ctx, d, userid, prefix+"%", limit, offset)
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
	if err := r.table.UpdModelByID(ctx, d, m, m.OrgID); err != nil {
		return kerrors.WithMsg(err, "Failed to update org")
	}
	return nil
}

func (r *repo) UpdateName(ctx context.Context, orgid string, name string) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableMembers.UpdorgNameByID(ctx, d, &orgName{
		Name: name,
	}, orgid); err != nil {
		return kerrors.WithMsg(err, "Failed to update org name for members")
	}
	if err := r.tableMods.UpdorgNameByID(ctx, d, &orgName{
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
	if err := r.tableMembers.DelByUserOrgs(ctx, d, userid, orgids); err != nil {
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
	if err := r.tableMods.DelByUserOrgs(ctx, d, userid, orgids); err != nil {
		return kerrors.WithMsg(err, "Failed to remove org mods")
	}
	return nil
}

func (r *repo) UpdateUsername(ctx context.Context, userid string, username string) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableMembers.UpdmemberUsernameByUserid(ctx, d, &memberUsername{
		Username: username,
	}, userid); err != nil {
		return kerrors.WithMsg(err, "Failed to update member username")
	}
	if err := r.tableMods.UpdmemberUsernameByUserid(ctx, d, &memberUsername{
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
	if err := r.table.DelByID(ctx, d, m.OrgID); err != nil {
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
		if !errors.Is(err, db.ErrAuthz) {
			return err
		}
	}
	if err := r.tableMembers.Setup(ctx, d); err != nil {
		err = kerrors.WithMsg(err, "Failed to setup org member model")
		if !errors.Is(err, db.ErrAuthz) {
			return err
		}
	}
	if err := r.tableMods.Setup(ctx, d); err != nil {
		err = kerrors.WithMsg(err, "Failed to setup org mod model")
		if !errors.Is(err, db.ErrAuthz) {
			return err
		}
	}
	return nil
}
