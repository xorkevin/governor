package model

import (
	"context"
	"errors"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/kerrors"
)

//go:generate forge model -m Model -p inv -o model_gen.go Model

type (
	// Repo is a role invitation repository
	Repo interface {
		GetByID(ctx context.Context, userid, invitedBy string, after int64) (*Model, error)
		GetByUser(ctx context.Context, userid string, after int64, limit, offset int) ([]Model, error)
		GetByInviter(ctx context.Context, invitedBy string, after int64, limit, offset int) ([]Model, error)
		Insert(ctx context.Context, userid string, invitedBy string, at int64) error
		DeleteByID(ctx context.Context, userid, invitedBy string) error
		DeleteByInviters(ctx context.Context, userid string, inviters []string) error
		DeleteBefore(ctx context.Context, t int64) error
		DeleteByUser(ctx context.Context, userid string) error
		Setup(ctx context.Context) error
	}

	repo struct {
		table *invModelTable
		db    db.Database
	}

	// Model is the db friend invitation model
	Model struct {
		Userid       string `model:"userid,VARCHAR(31)" query:"userid;deleq,userid"`
		InvitedBy    string `model:"invited_by,VARCHAR(31), PRIMARY KEY (userid, invited_by)" query:"invited_by;getoneeq,userid,invited_by,creation_time|gt;deleq,userid,invited_by;deleq,userid,invited_by|arr"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL;index;index,userid;index,invited_by" query:"creation_time;getgroupeq,userid,creation_time|gt;getgroupeq,invited_by,creation_time|gt;deleq,creation_time|leq"`
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

// NewInCtx creates a new role invitation repo from a context and sets it in the context
func NewInCtx(inj governor.Injector, table string) {
	SetCtxRepo(inj, NewCtx(inj, table))
}

// NewCtx creates a new role invitation repo from a context
func NewCtx(inj governor.Injector, table string) Repo {
	dbService := db.GetCtxDB(inj)
	return New(dbService, table)
}

// New creates a new role invitation repo
func New(database db.Database, table string) Repo {
	return &repo{
		table: &invModelTable{
			TableName: table,
		},
		db: database,
	}
}

func (r *repo) GetByID(ctx context.Context, userid, invitedBy string, after int64) (*Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelEqUseridEqInvitedByGtCreationTime(ctx, d, userid, invitedBy, after)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get invitation")
	}
	return m, nil
}

// GetByUser returns a user's invitations
func (r *repo) GetByUser(ctx context.Context, userid string, after int64, limit, offset int) ([]Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelEqUseridGtCreationTimeOrdCreationTime(ctx, d, userid, after, false, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get invitations")
	}
	return m, nil
}

// GetByInviter returns a inviter's invitations
func (r *repo) GetByInviter(ctx context.Context, invitedBy string, after int64, limit, offset int) ([]Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelEqInvitedByGtCreationTimeOrdCreationTime(ctx, d, invitedBy, after, false, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get invitations")
	}
	return m, nil
}

// Insert inserts an invitation into the db
func (r *repo) Insert(ctx context.Context, userid, invitedBy string, at int64) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.Insert(ctx, d, &Model{
		Userid:       userid,
		InvitedBy:    invitedBy,
		CreationTime: at,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to insert invitation")
	}
	return nil
}

// DeleteByID deletes an invitation by userid and inviter
func (r *repo) DeleteByID(ctx context.Context, userid, invitedBy string) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelEqUseridEqInvitedBy(ctx, d, userid, invitedBy); err != nil {
		return kerrors.WithMsg(err, "Failed to delete invitation")
	}
	return nil
}

// DeleteByInviters deletes an invitation by userid and inviters
func (r *repo) DeleteByInviters(ctx context.Context, userid string, inviters []string) error {
	if len(inviters) == 0 {
		return nil
	}
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelEqUseridHasInvitedBy(ctx, d, userid, inviters); err != nil {
		return kerrors.WithMsg(err, "Failed to delete invitations")
	}
	return nil
}

func (r *repo) DeleteBefore(ctx context.Context, t int64) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelLeqCreationTime(ctx, d, t); err != nil {
		return kerrors.WithMsg(err, "Failed to delete invitations")
	}
	return nil
}

func (r *repo) DeleteByUser(ctx context.Context, userid string) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelEqUserid(ctx, d, userid); err != nil {
		return kerrors.WithMsg(err, "Failed to delete user invitations")
	}
	return nil
}

// Setup creates a new friend invitation table
func (r *repo) Setup(ctx context.Context) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.Setup(ctx, d); err != nil {
		err = kerrors.WithMsg(err, "Failed to setup friend invitation model")
		if !errors.Is(err, db.ErrAuthz{}) {
			return err
		}
	}
	return nil
}
