package model

import (
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/rank"
)

//go:generate forge model -m Model -t userroleinvitations -p inv -o model_gen.go Model

type (
	// Repo is a role invitation repository
	Repo interface {
		GetByID(userid, role string, after int64) (*Model, error)
		GetByUser(userid string, after int64, limit, offset int) ([]Model, error)
		GetByRole(role string, after int64, limit, offset int) ([]Model, error)
		Insert(userid string, roles rank.Rank, by string, at int64) error
		DeleteByID(userid, role string) error
		DeleteByRoles(userid string, roles rank.Rank) error
		DeleteBefore(before int64) error
		Setup() error
	}

	repo struct {
		db db.Database
	}

	// Model is the db role invitation model
	Model struct {
		Userid       string `model:"userid,VARCHAR(31);index" query:"userid"`
		Role         string `model:"role,VARCHAR(255), PRIMARY KEY (userid, role);index" query:"role;getoneeq,userid,role,creation_time|gt;deleq,userid,role;deleq,userid,role|arr"`
		InvitedBy    string `model:"invited_by,VARCHAR(31) NOT NULL" query:"invited_by"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL;index" query:"creation_time;getgroupeq,userid,creation_time|gt;getgroupeq,role,creation_time|gt;deleq,creation_time|leq"`
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
func NewInCtx(inj governor.Injector) {
	SetCtxRepo(inj, NewCtx(inj))
}

// NewCtx creates a new role invitation repo from a context
func NewCtx(inj governor.Injector) Repo {
	dbService := db.GetCtxDB(inj)
	return New(dbService)
}

// New creates a new role invitation repo
func New(database db.Database) Repo {
	return &repo{
		db: database,
	}
}

func (r *repo) GetByID(userid, role string, after int64) (*Model, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, code, err := invModelGetModelEqUseridEqRoleGtCreationTime(d, userid, role, after)
	if err != nil {
		if code == 2 {
			return nil, governor.ErrWithKind(err, db.ErrNotFound{}, "Invitation not found")
		}
		return nil, governor.ErrWithMsg(err, "Failed to get invitation")
	}
	return m, nil
}

// GetByUser returns a user's invitations
func (r *repo) GetByUser(userid string, after int64, limit, offset int) ([]Model, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := invModelGetModelEqUseridGtCreationTimeOrdCreationTime(d, userid, after, false, limit, offset)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get invitations")
	}
	return m, nil
}

// GetByRole returns a role's invitations
func (r *repo) GetByRole(role string, after int64, limit, offset int) ([]Model, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := invModelGetModelEqRoleGtCreationTimeOrdCreationTime(d, role, after, false, limit, offset)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get invitations")
	}
	return m, nil
}

// Insert inserts invitations into the db
func (r *repo) Insert(userid string, roles rank.Rank, by string, at int64) error {
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
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if _, err := invModelInsertBulk(d, m, true); err != nil {
		return governor.ErrWithMsg(err, "Failed to insert invitations")
	}
	return nil
}

// DeleteByID deletes an invitation by userid and role
func (r *repo) DeleteByID(userid, role string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := invModelDelEqUseridEqRole(d, userid, role); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete invitation")
	}
	return nil
}

// DeleteByRoles deletes invitations by userid and roles
func (r *repo) DeleteByRoles(userid string, roles rank.Rank) error {
	if len(roles) == 0 {
		return nil
	}

	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := invModelDelEqUseridHasRole(d, userid, roles.ToSlice()); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete invitations")
	}
	return nil
}

func (r *repo) DeleteBefore(before int64) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := invModelDelLeqCreationTime(d, before); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete invitations")
	}
	return nil
}

// Setup creates a new role invitation table
func (r *repo) Setup() error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if code, err := invModelSetup(d); err != nil {
		if code != 5 {
			return governor.ErrWithMsg(err, "Failed to setup role invitation model")
		}
	}
	return nil
}
