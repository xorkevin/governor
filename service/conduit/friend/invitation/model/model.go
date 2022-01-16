package model

import (
	"errors"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
)

//go:generate forge model -m Model -p inv -o model_gen.go Model

type (
	// Repo is a role invitation repository
	Repo interface {
		GetByID(userid, invitedBy string, after int64) (*Model, error)
		GetByUser(userid string, after int64, limit, offset int) ([]Model, error)
		GetByInviter(invitedBy string, after int64, limit, offset int) ([]Model, error)
		Insert(userid string, invitedBy string, at int64) error
		DeleteByID(userid, invitedBy string) error
		DeleteByInviters(userid string, inviters []string) error
		DeleteBefore(t int64) error
		Setup() error
	}

	repo struct {
		table string
		db    db.Database
	}

	// Model is the db friend invitation model
	Model struct {
		Userid       string `model:"userid,VARCHAR(31)" query:"userid"`
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
		table: table,
		db:    database,
	}
}

func (r *repo) GetByID(userid, invitedBy string, after int64) (*Model, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := invModelGetModelEqUseridEqInvitedByGtCreationTime(d, r.table, userid, invitedBy, after)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get invitation")
	}
	return m, nil
}

// GetByUser returns a user's invitations
func (r *repo) GetByUser(userid string, after int64, limit, offset int) ([]Model, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := invModelGetModelEqUseridGtCreationTimeOrdCreationTime(d, r.table, userid, after, false, limit, offset)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get invitations")
	}
	return m, nil
}

// GetByInviter returns a inviter's invitations
func (r *repo) GetByInviter(invitedBy string, after int64, limit, offset int) ([]Model, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := invModelGetModelEqInvitedByGtCreationTimeOrdCreationTime(d, r.table, invitedBy, after, false, limit, offset)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get invitations")
	}
	return m, nil
}

// Insert inserts an invitation into the db
func (r *repo) Insert(userid, invitedBy string, at int64) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := invModelInsert(d, r.table, &Model{
		Userid:       userid,
		InvitedBy:    invitedBy,
		CreationTime: at,
	}); err != nil {
		return db.WrapErr(err, "Failed to insert invitation")
	}
	return nil
}

// DeleteByID deletes an invitation by userid and inviter
func (r *repo) DeleteByID(userid, invitedBy string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := invModelDelEqUseridEqInvitedBy(d, r.table, userid, invitedBy); err != nil {
		return db.WrapErr(err, "Failed to delete invitation")
	}
	return nil
}

// DeleteByInviters deletes an invitation by userid and inviters
func (r *repo) DeleteByInviters(userid string, inviters []string) error {
	if len(inviters) == 0 {
		return nil
	}
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := invModelDelEqUseridHasInvitedBy(d, r.table, userid, inviters); err != nil {
		return db.WrapErr(err, "Failed to delete invitations")
	}
	return nil
}

func (r *repo) DeleteBefore(t int64) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := invModelDelLeqCreationTime(d, r.table, t); err != nil {
		return db.WrapErr(err, "Failed to delete invitations")
	}
	return nil
}

// Setup creates a new friend invitation table
func (r *repo) Setup() error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := invModelSetup(d, r.table); err != nil {
		err = db.WrapErr(err, "Failed to setup friend invitation model")
		if !errors.Is(err, db.ErrAuthz{}) {
			return err
		}
	}
	return nil
}
