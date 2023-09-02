package couriermodel

import (
	"context"
	"time"

	"xorkevin.dev/governor/service/dbsql"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/kerrors"
)

//go:generate forge model

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
		Setup(ctx context.Context) error
	}

	repo struct {
		tableLinks *linkModelTable
		db         dbsql.Database
	}

	// LinkModel is the db link model
	//forge:model link
	//forge:model:query link
	LinkModel struct {
		LinkID       string `model:"linkid,VARCHAR(63) PRIMARY KEY"`
		URL          string `model:"url,VARCHAR(2047) NOT NULL"`
		CreatorID    string `model:"creatorid,VARCHAR(31) NOT NULL"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL"`
	}
)

// New creates a new courier repo
func New(database dbsql.Database, tableLinks, tableBrands string) Repo {
	return &repo{
		tableLinks: &linkModelTable{
			TableName: tableLinks,
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
	mUID, err := uid.New()
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

	m, err := r.tableLinks.GetLinkModelByCreator(ctx, d, creatorid, limit, offset)
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
	m, err := r.tableLinks.GetLinkModelByID(ctx, d, linkid)
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
	if err := r.tableLinks.DelByID(ctx, d, m.LinkID); err != nil {
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
	if err := r.tableLinks.DelByIDs(ctx, d, linkids); err != nil {
		return kerrors.WithMsg(err, "Failed to delete links")
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
		return kerrors.WithMsg(err, "Failed to setup link model")
	}
	return nil
}
