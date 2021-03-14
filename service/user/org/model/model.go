package model

import (
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/uid"
)

//go:generate forge model -m Model -t userorgs -p org -o model_gen.go Model

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
		Insert(m *Model) error
		Update(m *Model) error
		Delete(m *Model) error
		Setup() error
	}

	repo struct {
		db db.Database
	}

	// Model is the user org model
	Model struct {
		OrgID        string `model:"orgid,VARCHAR(31) PRIMARY KEY" query:"orgid,getoneeq,orgid;getgroupeq,orgid|arr;updeq,orgid;deleq,orgid"`
		Name         string `model:"name,VARCHAR(255) NOT NULL UNIQUE" query:"name,getoneeq,name"`
		DisplayName  string `model:"display_name,VARCHAR(255) NOT NULL" query:"display_name"`
		Desc         string `model:"description,VARCHAR(255) NOT NULL" query:"description"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL;index" query:"creation_time,getgroup"`
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
func NewInCtx(inj governor.Injector) {
	SetCtxRepo(inj, NewCtx(inj))
}

// NewCtx creates a new org repo from a context
func NewCtx(inj governor.Injector) Repo {
	dbService := db.GetCtxDB(inj)
	return New(dbService)
}

// New creates a new OAuth app repository
func New(database db.Database) Repo {
	return &repo{
		db: database,
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
	m, code, err := orgModelGetModelEqOrgID(d, orgid)
	if err != nil {
		if code == 2 {
			return nil, governor.ErrWithKind(err, db.ErrNotFound{}, "No org found with that id")
		}
		return nil, governor.ErrWithMsg(err, "Failed to get org")
	}
	return m, nil
}

func (r *repo) GetByName(orgname string) (*Model, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, code, err := orgModelGetModelEqName(d, orgname)
	if err != nil {
		if code == 2 {
			return nil, governor.ErrWithKind(err, db.ErrNotFound{}, "No org found with that name")
		}
		return nil, governor.ErrWithMsg(err, "Failed to get org")
	}
	return m, nil
}
func (r *repo) GetAllOrgs(limit, offset int) ([]Model, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := orgModelGetModelOrdCreationTime(d, false, limit, offset)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get orgs")
	}
	return m, nil
}

func (r *repo) GetOrgs(orgids []string) ([]Model, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := orgModelGetModelHasOrgIDOrdOrgID(d, orgids, true, len(orgids), 0)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get orgs")
	}
	return m, nil
}

func (r *repo) Insert(m *Model) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if code, err := orgModelInsert(d, m); err != nil {
		if code == 3 {
			return governor.ErrWithKind(err, db.ErrUnique{}, "Org name must be unique")
		}
		return governor.ErrWithMsg(err, "Failed to insert org")
	}
	return nil
}

func (r *repo) Update(m *Model) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if code, err := orgModelUpdModelEqOrgID(d, m, m.OrgID); err != nil {
		if code == 3 {
			return governor.ErrWithKind(err, db.ErrUnique{}, "Org name must be unique")
		}
		return governor.ErrWithMsg(err, "Failed to update org")
	}
	return nil
}

func (r *repo) Delete(m *Model) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := orgModelDelEqOrgID(d, m.OrgID); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete org")
	}
	return nil
}

func (r *repo) Setup() error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := orgModelSetup(d); err != nil {
		return governor.ErrWithMsg(err, "Failed to setup org model")
	}
	return nil
}
