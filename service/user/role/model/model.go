package rolemodel

import (
	"net/http"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
)

//go:generate forge model -m Model -t userroles -p role -o model_gen.go Model

type (
	Repo interface {
		New(userid, role string) *Model
		GetByID(userid, role string) (*Model, error)
		GetByRole(role string, limit, offset int) ([]string, error)
		GetUserRoles(userid string, limit, offset int) ([]string, error)
		InsertBulk(m []*Model) error
		DeleteRoles(userid string, roles []string) error
		DeleteUserRoles(userid string) error
		Setup() error
	}

	repo struct {
		db db.Database
	}

	// Model is the db User role model
	Model struct {
		Userid string `model:"userid,VARCHAR(31);index" query:"userid,getgroupeq,role;deleq,userid"`
		Role   string `model:"role,VARCHAR(255), PRIMARY KEY (userid, role);index" query:"role,getoneeq,userid,role;getgroupeq,userid;deleq,userid,role;deleq,userid,role|arr"`
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
	var m *Model
	if mRole, code, err := roleModelGetModelEqUseridRole(r.db.DB(), userid, role); err != nil {
		if code == 2 {
			return nil, governor.NewError("Role not found for user", http.StatusNotFound, err)
		}
		return nil, governor.NewError("Failed to get role", http.StatusInternalServerError, err)
	} else {
		m = mRole
	}
	return m, nil
}

// GetByRole returns a list of userids with the given role
func (r *repo) GetByRole(role string, limit, offset int) ([]string, error) {
	m, err := roleModelGetModelEqRoleOrdUserid(r.db.DB(), role, true, limit, offset)
	if err != nil {
		return nil, governor.NewError("Failed to get userids of role", http.StatusInternalServerError, err)
	}
	userids := make([]string, 0, len(m))
	for _, i := range m {
		userids = append(userids, i.Userid)
	}
	return userids, nil
}

// GetUserRoles returns a list of a user's roles
func (r *repo) GetUserRoles(userid string, limit, offset int) ([]string, error) {
	m, err := roleModelGetModelEqUseridOrdRole(r.db.DB(), userid, true, limit, offset)
	if err != nil {
		return nil, governor.NewError("Failed to get roles of userid", http.StatusInternalServerError, err)
	}
	roles := make([]string, 0, len(m))
	for _, i := range m {
		roles = append(roles, i.Role)
	}
	return roles, nil
}

// Insert inserts the model into the db
func (r *repo) Insert(m *Model) error {
	if code, err := roleModelInsert(r.db.DB(), m); err != nil {
		if code == 3 {
			return governor.NewErrorUser("Role id must be unique", http.StatusBadRequest, err)
		}
		return governor.NewError("Failed to insert role", http.StatusInternalServerError, err)
	}
	return nil
}

// InsertBulk inserts multiple models into the db
func (r *repo) InsertBulk(m []*Model) error {
	if _, err := roleModelInsertBulk(r.db.DB(), m, true); err != nil {
		return governor.NewError("Failed to insert roles", http.StatusInternalServerError, err)
	}
	return nil
}

// Delete deletes the model in the db
func (r *repo) Delete(m *Model) error {
	if err := roleModelDelEqUseridRole(r.db.DB(), m.Userid, m.Role); err != nil {
		return governor.NewError("Failed to delete role", http.StatusInternalServerError, err)
	}
	return nil
}

// DeleteRoles deletes multiple roles from the db of a userid
func (r *repo) DeleteRoles(userid string, roles []string) error {
	if err := roleModelDelEqUseridHasRole(r.db.DB(), userid, roles); err != nil {
		return governor.NewError("Failed to delete roles", http.StatusInternalServerError, err)
	}
	return nil
}

// DeleteUserRoles deletes all the roles of a user
func (r *repo) DeleteUserRoles(userid string) error {
	if err := roleModelDelEqUserid(r.db.DB(), userid); err != nil {
		return governor.NewError("Failed to delete roles of userid", http.StatusInternalServerError, err)
	}
	return nil
}

// Setup creates a new User role table
func (r *repo) Setup() error {
	if err := roleModelSetup(r.db.DB()); err != nil {
		return governor.NewError("Failed to setup role model", http.StatusInternalServerError, err)
	}
	return nil
}
