package couriermodel

import (
	"net/http"
	"time"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/uid"
)

//go:generate forge model -m LinkModel -t courierlinks -p link -o modellink_gen.go LinkModel
//go:generate forge model -m BrandModel -t courierbrands -p brand -o modelbrand_gen.go BrandModel

const (
	defaultUIDSize = 8
)

type (
	// Repo is a courier repository
	Repo interface {
		NewLink(linkid, url, creatorid string) *LinkModel
		NewLinkAuto(url, creatorid string) (*LinkModel, error)
		NewLinkEmpty() LinkModel
		NewLinkEmptyPtr() *LinkModel
		GetLinkGroup(limit, offset int, creatorid string) ([]LinkModel, error)
		GetLink(linkid string) (*LinkModel, error)
		InsertLink(m *LinkModel) error
		DeleteLink(m *LinkModel) error
		NewBrand(brandid, creatorid string) *BrandModel
		GetBrandGroup(limit, offset int, creatorid string) ([]BrandModel, error)
		GetBrand(brandid string) (*BrandModel, error)
		InsertBrand(m *BrandModel) error
		DeleteBrand(m *BrandModel) error
		Setup() error
	}

	repo struct {
		db db.Database
	}

	// LinkModel is the db link model
	LinkModel struct {
		LinkID       string `model:"linkid,VARCHAR(63) PRIMARY KEY" query:"linkid,getoneeq,linkid;deleq,linkid"`
		URL          string `model:"url,VARCHAR(2047) NOT NULL" query:"url"`
		CreatorID    string `model:"creatorid,VARCHAR(31) NOT NULL;index" query:"creatorid"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL;index" query:"creation_time,getgroup;getgroupeq,creatorid"`
	}

	// BrandModel is the db brand model
	BrandModel struct {
		BrandID      string `model:"brandid,VARCHAR(63)" query:"brandid,getoneeq,brandid;deleq,brandid"`
		CreatorID    string `model:"creatorid,VARCHAR(31), PRIMARY KEY (brandid, creatorid)" query:"creatorid"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL;index" query:"creation_time,getgroup;getgroupeq,creatorid"`
	}
)

// New creates a new courier repo
func New(database db.Database) Repo {
	return &repo{
		db: database,
	}
}

// NewLink creates a new link model
func (r *repo) NewLink(linkid, url, creatorid string) *LinkModel {
	return &LinkModel{
		LinkID:       linkid,
		URL:          url,
		CreatorID:    creatorid,
		CreationTime: time.Now().Round(0).Unix(),
	}
}

// NewLinkAuto creates a new courier model with the link id randomly generated
func (r *repo) NewLinkAuto(url, creatorid string) (*LinkModel, error) {
	mUID, err := uid.New(defaultUIDSize)
	if err != nil {
		return nil, governor.NewError("Failed to create new uid", http.StatusInternalServerError, err)
	}
	return r.NewLink(mUID.Base64(), url, creatorid), nil
}

// NewLinkEmpty creates an empty link model
func (r *repo) NewLinkEmpty() LinkModel {
	return LinkModel{}
}

// NewLinkEmptyPtr creates an empty link model reference
func (r *repo) NewLinkEmptyPtr() *LinkModel {
	return &LinkModel{}
}

// GetLinkGroup gets a list of links ordered by creation time
func (r *repo) GetLinkGroup(limit, offset int, creatorid string) ([]LinkModel, error) {
	db, err := r.db.DB()
	if err != nil {
		return nil, err
	}

	if creatorid != "" {
		m, err := linkModelGetLinkModelEqCreatorIDOrdCreationTime(db, creatorid, false, limit, offset)
		if err != nil {
			return nil, governor.NewError("Failed to get links", http.StatusInternalServerError, err)
		}
		return m, nil
	}

	m, err := linkModelGetLinkModelOrdCreationTime(db, false, limit, offset)
	if err != nil {
		return nil, governor.NewError("Failed to get links", http.StatusInternalServerError, err)
	}
	return m, nil
}

// GetLink returns a link model with the given id
func (r *repo) GetLink(linkid string) (*LinkModel, error) {
	db, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, code, err := linkModelGetLinkModelEqLinkID(db, linkid)
	if err != nil {
		if code == 2 {
			return nil, governor.NewError("No link found with that id", http.StatusNotFound, err)
		}
		return nil, governor.NewError("Failed to get link", http.StatusInternalServerError, err)
	}
	return m, nil
}

// InsertLink inserts the link model into the db
func (r *repo) InsertLink(m *LinkModel) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if code, err := linkModelInsert(db, m); err != nil {
		if code == 3 {
			return governor.NewError("Link id must be unique", http.StatusBadRequest, err)
		}
		return governor.NewError("Failed to insert link", http.StatusInternalServerError, err)
	}
	return nil
}

// DeleteLink deletes the link model in the db
func (r *repo) DeleteLink(m *LinkModel) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := linkModelDelEqLinkID(db, m.LinkID); err != nil {
		return governor.NewError("Failed to delete link", http.StatusInternalServerError, err)
	}
	return nil
}

// NewBrand creates a new brand model
func (r *repo) NewBrand(brandid, creatorid string) *BrandModel {
	return &BrandModel{
		BrandID:      brandid,
		CreatorID:    creatorid,
		CreationTime: time.Now().Round(0).Unix(),
	}
}

// GetBrandGroup gets a list of brands ordered by creation time
func (r *repo) GetBrandGroup(limit, offset int, creatorid string) ([]BrandModel, error) {
	db, err := r.db.DB()
	if err != nil {
		return nil, err
	}

	if creatorid != "" {
		m, err := brandModelGetBrandModelEqCreatorIDOrdCreationTime(db, creatorid, false, limit, offset)
		if err != nil {
			return nil, governor.NewError("Failed to get brands", http.StatusInternalServerError, err)
		}
		return m, nil
	}

	m, err := brandModelGetBrandModelOrdCreationTime(db, false, limit, offset)
	if err != nil {
		return nil, governor.NewError("Failed to get brands", http.StatusInternalServerError, err)
	}
	return m, nil
}

// GetBrand returns a brand model with the given id
func (r *repo) GetBrand(brandid string) (*BrandModel, error) {
	db, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, code, err := brandModelGetBrandModelEqBrandID(db, brandid)
	if err != nil {
		if code == 2 {
			return nil, governor.NewError("No brand found with that id", http.StatusNotFound, err)
		}
		return nil, governor.NewError("Failed to get brand", http.StatusInternalServerError, err)
	}
	return m, nil
}

// InsertBrand adds a brand to the db
func (r *repo) InsertBrand(m *BrandModel) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if code, err := brandModelInsert(db, m); err != nil {
		if code == 3 {
			return governor.NewError("Brand id must be unique", http.StatusBadRequest, err)
		}
		return governor.NewError("Failed to insert brand", http.StatusInternalServerError, err)
	}
	return nil
}

// DeleteBrand removes a brand from the db
func (r *repo) DeleteBrand(m *BrandModel) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := brandModelDelEqBrandID(db, m.BrandID); err != nil {
		return governor.NewError("Failed to delete brand", http.StatusInternalServerError, err)
	}
	return nil
}

// Setup creates new Courier tables
func (r *repo) Setup() error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := linkModelSetup(db); err != nil {
		return governor.NewError("Failed to setup link model", http.StatusInternalServerError, err)
	}
	if err := brandModelSetup(db); err != nil {
		return governor.NewError("Failed to setup brand model", http.StatusInternalServerError, err)
	}
	return nil
}
