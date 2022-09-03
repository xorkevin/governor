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

//go:generate forge model -m LinkModel -p link -o modellink_gen.go LinkModel
//go:generate forge model -m BrandModel -p brand -o modelbrand_gen.go BrandModel

const (
	defaultUIDSize = 8
)

type (
	// Repo is a courier repository
	Repo interface {
		NewLink(creatorid, linkid, url string) *LinkModel
		NewLinkAuto(creatorid, url string) (*LinkModel, error)
		GetLinkGroup(ctx context.Context, creatorid string, limit, offset int) ([]LinkModel, error)
		GetLink(ctx context.Context, linkid string) (*LinkModel, error)
		InsertLink(ctx context.Context, m *LinkModel) error
		DeleteLink(ctx context.Context, m *LinkModel) error
		DeleteLinks(ctx context.Context, linkids []string) error
		NewBrand(creatorid, brandid string) *BrandModel
		GetBrandGroup(ctx context.Context, creatorid string, limit, offset int) ([]BrandModel, error)
		GetBrand(ctx context.Context, creatorid, brandid string) (*BrandModel, error)
		InsertBrand(ctx context.Context, m *BrandModel) error
		DeleteBrand(ctx context.Context, m *BrandModel) error
		DeleteBrands(ctx context.Context, creatorid string, brandids []string) error
		Setup(ctx context.Context) error
	}

	repo struct {
		tableLinks  *linkModelTable
		tableBrands *brandModelTable
		db          db.Database
	}

	// LinkModel is the db link model
	LinkModel struct {
		LinkID       string `model:"linkid,VARCHAR(63) PRIMARY KEY" query:"linkid;getoneeq,linkid;deleq,linkid;deleq,linkid|arr"`
		URL          string `model:"url,VARCHAR(2047) NOT NULL" query:"url"`
		CreatorID    string `model:"creatorid,VARCHAR(31) NOT NULL" query:"creatorid"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL;index;index,creatorid" query:"creation_time;getgroupeq,creatorid"`
	}

	// BrandModel is the db brand model
	BrandModel struct {
		CreatorID    string `model:"creatorid,VARCHAR(31)" query:"creatorid"`
		BrandID      string `model:"brandid,VARCHAR(63), PRIMARY KEY (creatorid, brandid)" query:"brandid;getoneeq,creatorid,brandid;deleq,creatorid,brandid;deleq,creatorid,brandid|arr"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL;index;index,creatorid" query:"creation_time;getgroupeq,creatorid"`
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
func NewInCtx(inj governor.Injector, tableLinks, tableBrands string) {
	SetCtxRepo(inj, NewCtx(inj, tableLinks, tableBrands))
}

// NewCtx creates a new courier repo from a context
func NewCtx(inj governor.Injector, tableLinks, tableBrands string) Repo {
	dbService := db.GetCtxDB(inj)
	return New(dbService, tableLinks, tableBrands)
}

// New creates a new courier repo
func New(database db.Database, tableLinks, tableBrands string) Repo {
	return &repo{
		tableLinks: &linkModelTable{
			TableName: tableLinks,
		},
		tableBrands: &brandModelTable{
			TableName: tableBrands,
		},
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
		return nil, kerrors.WithMsg(err, "Failed to create new uid")
	}
	return r.NewLink(mUID.Base64(), url, creatorid), nil
}

// GetLinkGroup gets a list of links ordered by creation time
func (r *repo) GetLinkGroup(ctx context.Context, creatorid string, limit, offset int) ([]LinkModel, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}

	m, err := r.tableLinks.GetLinkModelEqCreatorIDOrdCreationTime(ctx, d, creatorid, false, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get links")
	}
	return m, nil
}

// GetLink returns a link model with the given id
func (r *repo) GetLink(ctx context.Context, linkid string) (*LinkModel, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.tableLinks.GetLinkModelEqLinkID(ctx, d, linkid)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get link")
	}
	return m, nil
}

// InsertLink inserts the link model into the db
func (r *repo) InsertLink(ctx context.Context, m *LinkModel) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableLinks.Insert(ctx, d, m); err != nil {
		return kerrors.WithMsg(err, "Failed to insert link")
	}
	return nil
}

// DeleteLink deletes the link model in the db
func (r *repo) DeleteLink(ctx context.Context, m *LinkModel) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableLinks.DelEqLinkID(ctx, d, m.LinkID); err != nil {
		return kerrors.WithMsg(err, "Failed to delete link")
	}
	return nil
}

// DeleteLinks deletes the links in the db
func (r *repo) DeleteLinks(ctx context.Context, linkids []string) error {
	if len(linkids) == 0 {
		return nil
	}

	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableLinks.DelHasLinkID(ctx, d, linkids); err != nil {
		return kerrors.WithMsg(err, "Failed to delete links")
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
func (r *repo) GetBrandGroup(ctx context.Context, creatorid string, limit, offset int) ([]BrandModel, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}

	m, err := r.tableBrands.GetBrandModelEqCreatorIDOrdCreationTime(ctx, d, creatorid, false, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get brands")
	}
	return m, nil
}

// GetBrand returns a brand model with the given id
func (r *repo) GetBrand(ctx context.Context, creatorid, brandid string) (*BrandModel, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.tableBrands.GetBrandModelEqCreatorIDEqBrandID(ctx, d, creatorid, brandid)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get brand")
	}
	return m, nil
}

// InsertBrand adds a brand to the db
func (r *repo) InsertBrand(ctx context.Context, m *BrandModel) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableBrands.Insert(ctx, d, m); err != nil {
		return kerrors.WithMsg(err, "Failed to insert brand")
	}
	return nil
}

// DeleteBrand removes a brand from the db
func (r *repo) DeleteBrand(ctx context.Context, m *BrandModel) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableBrands.DelEqCreatorIDEqBrandID(ctx, d, m.CreatorID, m.BrandID); err != nil {
		return kerrors.WithMsg(err, "Failed to delete brand")
	}
	return nil
}

// DeleteBrands removes brands from the db
func (r *repo) DeleteBrands(ctx context.Context, creatorid string, brandids []string) error {
	if len(brandids) == 0 {
		return nil
	}

	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableBrands.DelEqCreatorIDHasBrandID(ctx, d, creatorid, brandids); err != nil {
		return kerrors.WithMsg(err, "Failed to delete brands")
	}
	return nil
}

// Setup creates new Courier tables
func (r *repo) Setup(ctx context.Context) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableLinks.Setup(ctx, d); err != nil {
		err = kerrors.WithMsg(err, "Failed to setup link model")
		if !errors.Is(err, db.ErrAuthz{}) {
			return err
		}
	}
	if err := r.tableBrands.Setup(ctx, d); err != nil {
		err = kerrors.WithMsg(err, "Failed to setup brand model")
		if !errors.Is(err, db.ErrAuthz{}) {
			return err
		}
	}
	return nil
}
