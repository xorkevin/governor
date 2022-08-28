package model

import (
	"context"
	"errors"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/rank"
	"xorkevin.dev/kerrors"
)

//go:generate forge model -m Model -p role -o model_gen.go Model

type (
	// Repo is a user role repository
	Repo interface {
		New(userid, role string) *Model
		GetByID(ctx context.Context, userid, role string) (*Model, error)
		IntersectRoles(ctx context.Context, userid string, roles rank.Rank) (rank.Rank, error)
		GetByRole(ctx context.Context, role string, limit, offset int) ([]string, error)
		GetRoles(ctx context.Context, userid string, limit, offset int) (rank.Rank, error)
		GetRolesPrefix(ctx context.Context, userid string, prefix string, limit, offset int) (rank.Rank, error)
		InsertRoles(ctx context.Context, userid string, roles rank.Rank) error
		DeleteRoles(ctx context.Context, userid string, roles rank.Rank) error
		DeleteByRole(ctx context.Context, role string, userids []string) error
		DeleteUserRoles(ctx context.Context, userid string) error
		Setup(ctx context.Context) error
	}

	repo struct {
		table *roleModelTable
		db    db.Database
	}

	// Model is the db User role model
	Model struct {
		Userid string `model:"userid,VARCHAR(31);index,role" query:"userid;getgroupeq,role;deleq,userid"`
		Role   string `model:"role,VARCHAR(255), PRIMARY KEY (userid, role)" query:"role;getoneeq,userid,role;getgroupeq,userid;getgroupeq,userid,role|arr;getgroupeq,userid,role|like;deleq,role,userid|arr;deleq,userid,role;deleq,userid,role|arr"`
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

// NewInCtx creates a new role repo from a context and sets it in the context
func NewInCtx(inj governor.Injector, table string) {
	SetCtxRepo(inj, NewCtx(inj, table))
}

// NewCtx creates a new role repo from a context
func NewCtx(inj governor.Injector, table string) Repo {
	dbService := db.GetCtxDB(inj)
	return New(dbService, table)
}

// New creates a new user role repository
func New(database db.Database, table string) Repo {
	return &repo{
		table: &roleModelTable{
			TableName: table,
		},
		db: database,
	}
}

// New creates a new User role Model
func (r *repo) New(userid, role string) *Model {
	return &Model{
		Userid: userid,
		Role:   role,
	}
}

// GetByID returns a user role model with the given id
func (r *repo) GetByID(ctx context.Context, userid, role string) (*Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelEqUseridEqRole(ctx, d, userid, role)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get role")
	}
	return m, nil
}

// IntersectRoles gets the intersection of user roles and the input roles
func (r *repo) IntersectRoles(ctx context.Context, userid string, roles rank.Rank) (rank.Rank, error) {
	if len(roles) == 0 {
		return rank.Rank{}, nil
	}

	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelEqUseridHasRoleOrdRole(ctx, d, userid, roles.ToSlice(), true, len(roles), 0)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get user roles")
	}
	res := make(rank.Rank, len(m))
	for _, i := range m {
		res[i.Role] = struct{}{}
	}
	return res, nil
}

// GetByRole returns a list of userids with the given role
func (r *repo) GetByRole(ctx context.Context, role string, limit, offset int) ([]string, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelEqRoleOrdUserid(ctx, d, role, true, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get userids of role")
	}
	userids := make([]string, 0, len(m))
	for _, i := range m {
		userids = append(userids, i.Userid)
	}
	return userids, nil
}

// GetRoles returns a list of a user's roles
func (r *repo) GetRoles(ctx context.Context, userid string, limit, offset int) (rank.Rank, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelEqUseridOrdRole(ctx, d, userid, true, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get roles of userid")
	}
	roles := make(rank.Rank, len(m))
	for _, i := range m {
		roles[i.Role] = struct{}{}
	}
	return roles, nil
}

// GetRolesPrefix returns a list of a user's roles with a prefix
func (r *repo) GetRolesPrefix(ctx context.Context, userid string, prefix string, limit, offset int) (rank.Rank, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelEqUseridLikeRoleOrdRole(ctx, d, userid, prefix+"%", true, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get roles of userid")
	}
	roles := make(rank.Rank, len(m))
	for _, i := range m {
		roles[i.Role] = struct{}{}
	}
	return roles, nil
}

// Insert inserts the model into the db
func (r *repo) Insert(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.Insert(ctx, d, m); err != nil {
		return kerrors.WithMsg(err, "Failed to insert role")
	}
	return nil
}

// InsertRoles inserts roles for a user into the db
func (r *repo) InsertRoles(ctx context.Context, userid string, roles rank.Rank) error {
	if len(roles) == 0 {
		return nil
	}

	m := make([]*Model, 0, len(roles))
	for _, i := range roles.ToSlice() {
		m = append(m, &Model{
			Userid: userid,
			Role:   i,
		})
	}
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.InsertBulk(ctx, d, m, true); err != nil {
		return kerrors.WithMsg(err, "Failed to insert roles")
	}
	return nil
}

// Delete deletes the model in the db
func (r *repo) Delete(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelEqUseridEqRole(ctx, d, m.Userid, m.Role); err != nil {
		return kerrors.WithMsg(err, "Failed to delete role")
	}
	return nil
}

// DeleteRoles deletes multiple roles from the db of a userid
func (r *repo) DeleteRoles(ctx context.Context, userid string, roles rank.Rank) error {
	if len(roles) == 0 {
		return nil
	}

	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelEqUseridHasRole(ctx, d, userid, roles.ToSlice()); err != nil {
		return kerrors.WithMsg(err, "Failed to delete roles")
	}
	return nil
}

// DeleteByRole deletes by role name
func (r *repo) DeleteByRole(ctx context.Context, role string, userids []string) error {
	if len(userids) == 0 {
		return nil
	}

	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelEqRoleHasUserid(ctx, d, role, userids); err != nil {
		return kerrors.WithMsg(err, "Failed to delete roles")
	}
	return nil
}

// DeleteUserRoles deletes all the roles of a user
func (r *repo) DeleteUserRoles(ctx context.Context, userid string) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelEqUserid(ctx, d, userid); err != nil {
		return kerrors.WithMsg(err, "Failed to delete user roles")
	}
	return nil
}

// Setup creates a new User role table
func (r *repo) Setup(ctx context.Context) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.Setup(ctx, d); err != nil {
		err = kerrors.WithMsg(err, "Failed to setup role model")
		if !errors.Is(err, db.ErrAuthz{}) {
			return err
		}
	}
	return nil
}
