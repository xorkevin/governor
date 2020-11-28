package rolemodel

import (
	"net/http"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/rank"
)

//go:generate forge model -m Model -t userroles -p role -o model_gen.go Model

type (
	Repo interface {
		New(userid, role string) *Model
		GetByID(userid, role string) (*Model, error)
		IntersectRoles(userid string, roles rank.Rank) (rank.Rank, error)
		GetByRole(role string, limit, offset int) ([]string, error)
		GetRoles(userid string, limit, offset int) (rank.Rank, error)
		GetRolesPrefix(userid string, prefix string, limit, offset int) (rank.Rank, error)
		InsertRoles(userid string, roles rank.Rank) error
		DeleteRoles(userid string, roles rank.Rank) error
		DeleteByRole(role string) error
		DeleteUserRoles(userid string) error
		Setup() error
	}

	repo struct {
		db db.Database
	}

	// Model is the db User role model
	Model struct {
		Userid string `model:"userid,VARCHAR(31);index" query:"userid,getgroupeq,role;deleq,userid"`
		Role   string `model:"role,VARCHAR(255), PRIMARY KEY (userid, role);index" query:"role,getoneeq,userid,role;getgroupeq,userid;getgroupeq,userid,role|arr;getgroupeq,userid,role|like;deleq,role;deleq,userid,role;deleq,userid,role|arr"`
	}
)

func New(database db.Database) Repo {
	return &repo{
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
func (r *repo) GetByID(userid, role string) (*Model, error) {
	db, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, code, err := roleModelGetModelEqUseridEqRole(db, userid, role)
	if err != nil {
		if code == 2 {
			return nil, governor.NewError("Role not found for user", http.StatusNotFound, err)
		}
		return nil, governor.NewError("Failed to get role", http.StatusInternalServerError, err)
	}
	return m, nil
}

// IntersectRoles gets the intersection of user roles and the input roles
func (r *repo) IntersectRoles(userid string, roles rank.Rank) (rank.Rank, error) {
	if len(roles) == 0 {
		return rank.Rank{}, nil
	}

	db, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := roleModelGetModelEqUseridHasRoleOrdRole(db, userid, roles.ToSlice(), true, len(roles), 0)
	if err != nil {
		return nil, governor.NewError("Failed to get user roles", http.StatusInternalServerError, err)
	}
	res := make(rank.Rank, len(m))
	for _, i := range m {
		res[i.Role] = struct{}{}
	}
	return res, nil
}

// GetByRole returns a list of userids with the given role
func (r *repo) GetByRole(role string, limit, offset int) ([]string, error) {
	db, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := roleModelGetModelEqRoleOrdUserid(db, role, true, limit, offset)
	if err != nil {
		return nil, governor.NewError("Failed to get userids of role", http.StatusInternalServerError, err)
	}
	userids := make([]string, 0, len(m))
	for _, i := range m {
		userids = append(userids, i.Userid)
	}
	return userids, nil
}

// GetRoles returns a list of a user's roles
func (r *repo) GetRoles(userid string, limit, offset int) (rank.Rank, error) {
	db, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := roleModelGetModelEqUseridOrdRole(db, userid, true, limit, offset)
	if err != nil {
		return nil, governor.NewError("Failed to get roles of userid", http.StatusInternalServerError, err)
	}
	roles := make(rank.Rank, len(m))
	for _, i := range m {
		roles[i.Role] = struct{}{}
	}
	return roles, nil
}

// GetRolesPrefix returns a list of a user's roles with a prefix
func (r *repo) GetRolesPrefix(userid string, prefix string, limit, offset int) (rank.Rank, error) {
	db, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := roleModelGetModelEqUseridLikeRoleOrdRole(db, userid, prefix+"%", true, limit, offset)
	if err != nil {
		return nil, governor.NewError("Failed to get roles of userid", http.StatusInternalServerError, err)
	}
	roles := make(rank.Rank, len(m))
	for _, i := range m {
		roles[i.Role] = struct{}{}
	}
	return roles, nil
}

// Insert inserts the model into the db
func (r *repo) Insert(m *Model) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if code, err := roleModelInsert(db, m); err != nil {
		if code == 3 {
			return governor.NewErrorUser("Role id must be unique", http.StatusBadRequest, err)
		}
		return governor.NewError("Failed to insert role", http.StatusInternalServerError, err)
	}
	return nil
}

// InsertRoles inserts roles for a user into the db
func (r *repo) InsertRoles(userid string, roles rank.Rank) error {
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
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if _, err := roleModelInsertBulk(db, m, true); err != nil {
		return governor.NewError("Failed to insert roles", http.StatusInternalServerError, err)
	}
	return nil
}

// Delete deletes the model in the db
func (r *repo) Delete(m *Model) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := roleModelDelEqUseridEqRole(db, m.Userid, m.Role); err != nil {
		return governor.NewError("Failed to delete role", http.StatusInternalServerError, err)
	}
	return nil
}

// DeleteRoles deletes multiple roles from the db of a userid
func (r *repo) DeleteRoles(userid string, roles rank.Rank) error {
	if len(roles) == 0 {
		return nil
	}

	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := roleModelDelEqUseridHasRole(db, userid, roles.ToSlice()); err != nil {
		return governor.NewError("Failed to delete roles", http.StatusInternalServerError, err)
	}
	return nil
}

// DeleteByRole deletes by role name
func (r *repo) DeleteByRole(role string) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := roleModelDelEqRole(db, role); err != nil {
		return governor.NewError("Failed to delete roles", http.StatusInternalServerError, err)
	}
	return nil
}

// DeleteUserRoles deletes all the roles of a user
func (r *repo) DeleteUserRoles(userid string) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := roleModelDelEqUserid(db, userid); err != nil {
		return governor.NewError("Failed to delete user roles", http.StatusInternalServerError, err)
	}
	return nil
}

// Setup creates a new User role table
func (r *repo) Setup() error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := roleModelSetup(db); err != nil {
		return governor.NewError("Failed to setup role model", http.StatusInternalServerError, err)
	}
	return nil
}
