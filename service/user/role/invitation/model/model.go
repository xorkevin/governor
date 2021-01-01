package invitationmodel

import (
	"context"
	"net/http"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/rank"
)

//go:generate forge model -m Model -t userroleinvitations -p inv -o model_gen.go Model

type (
	Repo interface {
		GetByID(userid, role string, after int64) (*Model, error)
		GetByUser(userid string, after int64, limit, offset int) ([]Model, error)
		GetByRole(role string, after int64, limit, offset int) ([]Model, error)
		GetByRolePrefix(prefix string, after int64, limit, offset int) ([]Model, error)
		Insert(userid string, roles rank.Rank, by string, at int64) error
		DeleteByID(userid, role string) error
		DeleteByRoles(userid string, roles rank.Rank) error
		DeleteBefore(before int64) error
	}

	repo struct {
		db db.Database
	}

	// Model is the db role invitation model
	Model struct {
		Userid       string `model:"userid,VARCHAR(31);index" query:"userid"`
		Role         string `model:"role,VARCHAR(255), PRIMARY KEY (userid, role);index" query:"role,getoneeq,userid,role,creation_time|gt;deleq,userid,role;deleq,userid,role|arr"`
		InvitedBy    string `model:"invited_by,VARCHAR(31) NOT NULL" query:"invited_by"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL;index" query:"creation_time,getgroupeq,userid,creation_time|gt;getgroupeq,role,creation_time|gt;getgroupeq,role|like,creation_time|gt;deleq,creation_time|leq"`
	}

	ctxKeyRepo struct{}
)

// GetCtxRepo returns a Repo from the context
func GetCtxRepo(ctx context.Context, r Repo) Repo {
	v := ctx.Value(ctxKeyRepo{})
	if v == nil {
		return nil
	}
	return v.(Repo)
}

// SetCtxRepo sets a Repo in the context
func SetCtxRepo(ctx context.Context, r Repo) context.Context {
	return context.WithValue(ctx, ctxKeyRepo{}, r)
}

func New(database db.Database) Repo {
	return &repo{
		db: database,
	}
}

func (r *repo) GetByID(userid, role string, after int64) (*Model, error) {
	db, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, code, err := invModelGetModelEqUseridEqRoleGtCreationTime(db, userid, role, after)
	if err != nil {
		if code == 2 {
			return nil, governor.NewError("Invitation not found", http.StatusNotFound, err)
		}
		return nil, governor.NewError("Failed to get invitation", http.StatusInternalServerError, err)
	}
	return m, nil
}

// GetByUser returns a user's invitations
func (r *repo) GetByUser(userid string, after int64, limit, offset int) ([]Model, error) {
	db, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := invModelGetModelEqUseridGtCreationTimeOrdCreationTime(db, userid, after, false, limit, offset)
	if err != nil {
		return nil, governor.NewError("Failed to get invitations", http.StatusInternalServerError, err)
	}
	return m, nil
}

// GetByRole returns a role's invitations
func (r *repo) GetByRole(role string, after int64, limit, offset int) ([]Model, error) {
	db, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := invModelGetModelEqRoleGtCreationTimeOrdCreationTime(db, role, after, false, limit, offset)
	if err != nil {
		return nil, governor.NewError("Failed to get invitations", http.StatusInternalServerError, err)
	}
	return m, nil
}

// GetByRolePrefix returns invitations matching a role prefix
func (r *repo) GetByRolePrefix(prefix string, after int64, limit, offset int) ([]Model, error) {
	db, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := invModelGetModelLikeRoleGtCreationTimeOrdCreationTime(db, prefix+"%", after, false, limit, offset)
	if err != nil {
		return nil, governor.NewError("Failed to get invitations", http.StatusInternalServerError, err)
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
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if _, err := invModelInsertBulk(db, m, true); err != nil {
		return governor.NewError("Failed to insert invitations", http.StatusInternalServerError, err)
	}
	return nil
}

// DeleteByID deletes an invitation by userid and role
func (r *repo) DeleteByID(userid, role string) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := invModelDelEqUseridEqRole(db, userid, role); err != nil {
		return governor.NewError("Failed to delete invitation", http.StatusInternalServerError, err)
	}
	return nil
}

// DeleteByRoles deletes invitations by userid and roles
func (r *repo) DeleteByRoles(userid string, roles rank.Rank) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := invModelDelEqUseridHasRole(db, userid, roles.ToSlice()); err != nil {
		return governor.NewError("Failed to delete invitations", http.StatusInternalServerError, err)
	}
	return nil
}

func (r *repo) DeleteBefore(before int64) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := invModelDelLeqCreationTime(db, before); err != nil {
		return governor.NewError("Failed to delete invitations", http.StatusInternalServerError, err)
	}
	return nil
}

// Setup creates a new role invitation table
func (r *repo) Setup() error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := invModelSetup(db); err != nil {
		return governor.NewError("Failed to setup role invitation model", http.StatusInternalServerError, err)
	}
	return nil
}
