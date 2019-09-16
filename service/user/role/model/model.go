package rolemodel

import (
	"net/http"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
)

//go:generate forge model -m Model -t userroles -p role -o model_gen.go Model qUserid qRole

type (
	Repo interface {
		New(userid, role string) *Model
		GetByID(userid, role string) (*Model, error)
		GetByRole(role string, limit, offset int) ([]string, error)
		GetUserRoles(userid string, limit, offset int) ([]string, error)
		InsertBulk(m []*Model) error
		DeleteBulk(roleids []*Model) error
		DeleteUserRoles(userid string) error
		Setup() error
	}

	repo struct {
		db db.Database
	}

	// Model is the db User role model
	Model struct {
		roleid string `model:"roleid,VARCHAR(511) PRIMARY KEY" query:"roleid,get;deleq,userid;delset"`
		Userid string `model:"userid,VARCHAR(31) NOT NULL" query:"userid"`
		Role   string `model:"role,VARCHAR(255) NOT NULL" query:"role"`
	}

	qUserid struct {
		Userid string `query:"userid,getgroupeq,role"`
	}

	qRole struct {
		Role string `query:"role,getgroupeq,userid"`
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

func calcRoleid(userid, role string) string {
	return userid + "|" + role
}
func (m *Model) ensureRoleid() {
	m.roleid = calcRoleid(m.Userid, m.Role)
}

// GetByID returns a user role model with the given id
func (r *repo) GetByID(userid, role string) (*Model, error) {
	var m *Model
	if mRole, code, err := roleModelGetModelByroleid(r.db.DB(), calcRoleid(userid, role)); err != nil {
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
	m, err := roleModelGetqUseridEqRoleOrdUserid(r.db.DB(), role, true, limit, offset)
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
	m, err := roleModelGetqRoleEqUseridOrdRole(r.db.DB(), userid, true, limit, offset)
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
	m.ensureRoleid()
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
	for _, i := range m {
		i.ensureRoleid()
	}
	if _, err := roleModelInsertBulk(r.db.DB(), m, true); err != nil {
		return governor.NewError("Failed to insert roles", http.StatusInternalServerError, err)
	}
	return nil
}

// Delete deletes the model in the db
func (r *repo) Delete(m *Model) error {
	m.ensureRoleid()
	if err := roleModelDelEqUserid(r.db.DB(), m.roleid); err != nil {
		return governor.NewError("Failed to delete role", http.StatusInternalServerError, err)
	}
	return nil
}

// DeleteBulk deletes multiple models from the db
func (r *repo) DeleteBulk(m []*Model) error {
	args := make([]string, 0, len(m))
	for _, i := range m {
		i.ensureRoleid()
		args = append(args, i.roleid)
	}
	if err := roleModelDelSetroleid(r.db.DB(), args); err != nil {
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
