package roleinvmodel

import (
	"context"
	"errors"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/rank"
	"xorkevin.dev/kerrors"
)

//go:generate forge model

type (
	// Repo is a role invitation repository
	Repo interface {
		GetByID(ctx context.Context, userid, role string, after int64) (*Model, error)
		GetByUser(ctx context.Context, userid string, after int64, limit, offset int) ([]Model, error)
		GetByRole(ctx context.Context, role string, after int64, limit, offset int) ([]Model, error)
		Insert(ctx context.Context, userid string, roles rank.Rank, by string, at int64) error
		DeleteByID(ctx context.Context, userid, role string) error
		DeleteByRoles(ctx context.Context, userid string, roles rank.Rank) error
		DeleteRole(ctx context.Context, role string) error
		DeleteBefore(ctx context.Context, t int64) error
		Setup(ctx context.Context) error
	}

	repo struct {
		table *invModelTable
		db    db.Database
	}

	// Model is the db role invitation model
	//forge:model inv
	//forge:model:query inv
	Model struct {
		Userid       string `model:"userid,VARCHAR(31)" query:"userid"`
		Role         string `model:"role,VARCHAR(255), PRIMARY KEY (userid, role)" query:"role;getoneeq,userid,role,creation_time|gt;deleq,userid,role;deleq,userid,role|in;deleq,role"`
		InvitedBy    string `model:"invited_by,VARCHAR(31) NOT NULL" query:"invited_by"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL;index;index,userid;index,role" query:"creation_time;getgroupeq,userid,creation_time|gt;getgroupeq,role,creation_time|gt;deleq,creation_time|leq"`
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

func (r *repo) GetByID(ctx context.Context, userid, role string, after int64) (*Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelEqUseridEqRoleGtCreationTime(ctx, d, userid, role, after)
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

// GetByRole returns a role's invitations
func (r *repo) GetByRole(ctx context.Context, role string, after int64, limit, offset int) ([]Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelEqRoleGtCreationTimeOrdCreationTime(ctx, d, role, after, false, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get invitations")
	}
	return m, nil
}

// Insert inserts invitations into the db
func (r *repo) Insert(ctx context.Context, userid string, roles rank.Rank, by string, at int64) error {
	if len(roles) == 0 {
		return nil
	}

	m := make([]*Model, 0, len(roles))
	for _, i := range roles.ToSlice() {
		m = append(m, &Model{
			Userid:       userid,
			Role:         i,
			InvitedBy:    by,
			CreationTime: at,
		})
	}
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.InsertBulk(ctx, d, m, true); err != nil {
		return kerrors.WithMsg(err, "Failed to insert invitations")
	}
	return nil
}

// DeleteByID deletes an invitation by userid and role
func (r *repo) DeleteByID(ctx context.Context, userid, role string) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelEqUseridEqRole(ctx, d, userid, role); err != nil {
		return kerrors.WithMsg(err, "Failed to delete invitation")
	}
	return nil
}

// DeleteByRoles deletes invitations by userid and roles
func (r *repo) DeleteByRoles(ctx context.Context, userid string, roles rank.Rank) error {
	if len(roles) == 0 {
		return nil
	}

	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelEqUseridHasRole(ctx, d, userid, roles.ToSlice()); err != nil {
		return kerrors.WithMsg(err, "Failed to delete invitations")
	}
	return nil
}

func (r *repo) DeleteRole(ctx context.Context, role string) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelEqRole(ctx, d, role); err != nil {
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

// Setup creates a new role invitation table
func (r *repo) Setup(ctx context.Context) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.Setup(ctx, d); err != nil {
		err = kerrors.WithMsg(err, "Failed to setup role invitation model")
		if !errors.Is(err, db.ErrorAuthz) {
			return err
		}
	}
	return nil
}
