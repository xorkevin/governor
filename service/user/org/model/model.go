package orgmodel

import (
	"net/http"
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
	// Repo is an user org repository
	Repo interface {
		New(name, displayName, desc string) (*Model, error)
		GetByID(orgid string) (*Model, error)
		GetOrgs(limit, offset int) ([]Model, error)
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
		OrgID        string `model:"orgid,VARCHAR(31) PRIMARY KEY" query:"orgid,getoneeq,orgid;updeq,orgid;deleq,orgid"`
		Name         string `model:"name,VARCHAR(255) NOT NULL UNIQUE" query:"name,getoneeq,name"`
		DisplayName  string `model:"display_name,VARCHAR(255) NOT NULL" query:"display_name"`
		Desc         string `model:"description,VARCHAR(255) NOT NULL" query:"description"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL;index" query:"creation_time,getgroup"`
	}
)

// New creates a new OAuth app repository
func New(database db.Database) Repo {
	return &repo{
		db: database,
	}
}

func (r *repo) New(name, displayName, desc string) (*Model, error) {
	mUID, err := uid.New(uidSize)
	if err != nil {
		return nil, governor.NewError("Failed to create new uid", http.StatusInternalServerError, err)
	}
	orgid := mUID.Base64()

	now := time.Now().Round(0).Unix()
	return &Model{
		OrgID:        orgid,
		Name:         name,
		DisplayName:  displayName,
		Desc:         desc,
		CreationTime: now,
	}, nil
}

func (r *repo) GetByID(orgid string) (*Model, error) {
	db, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, code, err := orgModelGetModelEqOrgID(db, orgid)
	if err != nil {
		if code == 2 {
			return nil, governor.NewError("No org found with that id", http.StatusNotFound, err)
		}
		return nil, governor.NewError("Failed to get org", http.StatusInternalServerError, err)
	}
	return m, nil
}

func (r *repo) GetOrgs(limit, offset int) ([]Model, error) {
	db, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := orgModelGetModelOrdCreationTime(db, false, limit, offset)
	if err != nil {
		return nil, governor.NewError("Failed to get orgs", http.StatusInternalServerError, err)
	}
	return m, nil
}

func (r *repo) Insert(m *Model) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if code, err := orgModelInsert(db, m); err != nil {
		if code == 3 {
			return governor.NewError("org name must be unique", http.StatusBadRequest, err)
		}
		return governor.NewError("Failed to insert org", http.StatusInternalServerError, err)
	}
	return nil
}

func (r *repo) Update(m *Model) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if _, err := orgModelUpdModelEqOrgID(db, m, m.OrgID); err != nil {
		return governor.NewError("Failed to update org", http.StatusInternalServerError, err)
	}
	return nil
}

func (r *repo) Delete(m *Model) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := orgModelDelEqOrgID(db, m.OrgID); err != nil {
		return governor.NewError("Failed to delete org", http.StatusInternalServerError, err)
	}
	return nil
}

func (r *repo) Setup() error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := orgModelSetup(db); err != nil {
		return governor.NewError("Failed to setup org model", http.StatusInternalServerError, err)
	}
	return nil
}
