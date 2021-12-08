package model

import (
	"errors"
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
		NewLink(creatorid, linkid, url string) *LinkModel
		NewLinkAuto(creatorid, url string) (*LinkModel, error)
		GetLinkGroup(creatorid string, limit, offset int) ([]LinkModel, error)
		GetLink(linkid string) (*LinkModel, error)
		InsertLink(m *LinkModel) error
		DeleteLink(m *LinkModel) error
		DeleteLinks(linkids []string) error
		NewBrand(creatorid, brandid string) *BrandModel
		GetBrandGroup(creatorid string, limit, offset int) ([]BrandModel, error)
		GetBrand(creatorid, brandid string) (*BrandModel, error)
		InsertBrand(m *BrandModel) error
		DeleteBrand(m *BrandModel) error
		DeleteBrands(creatorid string, brandids []string) error
		Setup() error
	}

	repo struct {
		db db.Database
	}

	// LinkModel is the db link model
	LinkModel struct {
		LinkID       string `model:"linkid,VARCHAR(63) PRIMARY KEY" query:"linkid;getoneeq,linkid;deleq,linkid;deleq,linkid|arr"`
		URL          string `model:"url,VARCHAR(2047) NOT NULL" query:"url"`
		CreatorID    string `model:"creatorid,VARCHAR(31) NOT NULL" query:"creatorid"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL;index;index,creatorid" query:"creation_time;getgroup;getgroupeq,creatorid"`
	}

	// BrandModel is the db brand model
	BrandModel struct {
		CreatorID    string `model:"creatorid,VARCHAR(31)" query:"creatorid"`
		BrandID      string `model:"brandid,VARCHAR(63), PRIMARY KEY (creatorid, brandid)" query:"brandid;getoneeq,creatorid,brandid;deleq,creatorid,brandid;deleq,creatorid,brandid|arr"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL;index;index,creatorid" query:"creation_time;getgroup;getgroupeq,creatorid"`
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

// NewInCtx creates a new courier repo from a context and sets it in the context
func NewInCtx(inj governor.Injector) {
	SetCtxRepo(inj, NewCtx(inj))
}

// NewCtx creates a new courier repo from a context
func NewCtx(inj governor.Injector) Repo {
	dbService := db.GetCtxDB(inj)
	return New(dbService)
}

// New creates a new courier repo
func New(database db.Database) Repo {
	return &repo{
		db: database,
	}
}

// NewLink creates a new link model
func (r *repo) NewLink(creatorid, linkid, url string) *LinkModel {
	return &LinkModel{
		LinkID:       linkid,
		URL:          url,
		CreatorID:    creatorid,
		CreationTime: time.Now().Round(0).Unix(),
	}
}

// NewLinkAuto creates a new courier model with the link id randomly generated
func (r *repo) NewLinkAuto(creatorid, url string) (*LinkModel, error) {
	mUID, err := uid.New(defaultUIDSize)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to create new uid")
	}
	return r.NewLink(mUID.Base64(), url, creatorid), nil
}

// GetLinkGroup gets a list of links ordered by creation time
func (r *repo) GetLinkGroup(creatorid string, limit, offset int) ([]LinkModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}

	if creatorid != "" {
		m, err := linkModelGetLinkModelEqCreatorIDOrdCreationTime(d, creatorid, false, limit, offset)
		if err != nil {
			return nil, db.WrapErr(err, "Failed to get links")
		}
		return m, nil
	}

	m, err := linkModelGetLinkModelOrdCreationTime(d, false, limit, offset)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get links")
	}
	return m, nil
}

// GetLink returns a link model with the given id
func (r *repo) GetLink(linkid string) (*LinkModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := linkModelGetLinkModelEqLinkID(d, linkid)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get link")
	}
	return m, nil
}

// InsertLink inserts the link model into the db
func (r *repo) InsertLink(m *LinkModel) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := linkModelInsert(d, m); err != nil {
		return db.WrapErr(err, "Failed to insert link")
	}
	return nil
}

// DeleteLink deletes the link model in the db
func (r *repo) DeleteLink(m *LinkModel) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := linkModelDelEqLinkID(d, m.LinkID); err != nil {
		return db.WrapErr(err, "Failed to delete link")
	}
	return nil
}

// DeleteLinks deletes the links in the db
func (r *repo) DeleteLinks(linkids []string) error {
	if len(linkids) == 0 {
		return nil
	}
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := linkModelDelHasLinkID(d, linkids); err != nil {
		return db.WrapErr(err, "Failed to delete links")
	}
	return nil
}

// NewBrand creates a new brand model
func (r *repo) NewBrand(creatorid, brandid string) *BrandModel {
	return &BrandModel{
		CreatorID:    creatorid,
		BrandID:      brandid,
		CreationTime: time.Now().Round(0).Unix(),
	}
}

// GetBrandGroup gets a list of brands ordered by creation time
func (r *repo) GetBrandGroup(creatorid string, limit, offset int) ([]BrandModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}

	if creatorid != "" {
		m, err := brandModelGetBrandModelEqCreatorIDOrdCreationTime(d, creatorid, false, limit, offset)
		if err != nil {
			return nil, db.WrapErr(err, "Failed to get brands")
		}
		return m, nil
	}

	m, err := brandModelGetBrandModelOrdCreationTime(d, false, limit, offset)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get brands")
	}
	return m, nil
}

// GetBrand returns a brand model with the given id
func (r *repo) GetBrand(creatorid, brandid string) (*BrandModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := brandModelGetBrandModelEqCreatorIDEqBrandID(d, creatorid, brandid)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get brand")
	}
	return m, nil
}

// InsertBrand adds a brand to the db
func (r *repo) InsertBrand(m *BrandModel) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := brandModelInsert(d, m); err != nil {
		return db.WrapErr(err, "Failed to insert brand")
	}
	return nil
}

// DeleteBrand removes a brand from the db
func (r *repo) DeleteBrand(m *BrandModel) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := brandModelDelEqCreatorIDEqBrandID(d, m.CreatorID, m.BrandID); err != nil {
		return db.WrapErr(err, "Failed to delete brand")
	}
	return nil
}

// DeleteBrands removes brands from the db
func (r *repo) DeleteBrands(creatorid string, brandids []string) error {
	if len(brandids) == 0 {
		return nil
	}
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := brandModelDelEqCreatorIDHasBrandID(d, creatorid, brandids); err != nil {
		return db.WrapErr(err, "Failed to delete brands")
	}
	return nil
}

// Setup creates new Courier tables
func (r *repo) Setup() error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := linkModelSetup(d); err != nil {
		err = db.WrapErr(err, "Failed to setup link model")
		if !errors.Is(err, db.ErrAuthz{}) {
			return err
		}
	}
	if err := brandModelSetup(d); err != nil {
		err = db.WrapErr(err, "Failed to setup brand model")
		if !errors.Is(err, db.ErrAuthz{}) {
			return err
		}
	}
	return nil
}
