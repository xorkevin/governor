package rolemodel

import (
	"database/sql"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/db"
	"net/http"
)

//go:generate go run ../../../../gen/model.go -- model_gen.go role userroles Model qUserid qRole

const (
	moduleID      = "rolemodel"
	moduleIDModel = moduleID + ".Model"
)

type (
	Repo interface {
		New(userid, role string) (*Model, *governor.Error)
		GetByID(userid, role string) (*Model, *governor.Error)
		GetByRole(role string, limit, offset int) ([]string, *governor.Error)
		GetUserRoles(userid string, limit, offset int) ([]string, *governor.Error)
		Insert(m *Model) *governor.Error
		Delete(m *Model) *governor.Error
		DeleteUserRoles(userid string) *governor.Error
		Setup() *governor.Error
	}

	repo struct {
		db *sql.DB
	}

	// Model is the db User role model
	Model struct {
		roleid string `model:"roleid,VARCHAR(511) PRIMARY KEY"`
		Userid string `model:"userid,VARCHAR(31) NOT NULL"`
		Role   string `model:"role,VARCHAR(255) NOT NULL"`
	}

	qUserid struct {
		Userid string `query:"userid,getgroupeq,role"`
	}

	qRole struct {
		Role string `query:"role,getgroupeq,userid"`
	}
)

func New(conf governor.Config, l governor.Logger, database db.Database) Repo {
	l.Info("initialized user role model", moduleID, "initialize user role model", 0, nil)
	return &repo{
		db: database.DB(),
	}
}

const (
	moduleIDModNew = moduleIDModel + ".New"
)

// New creates a new User role Model
func (r *repo) New(userid, role string) (*Model, *governor.Error) {
	m := &Model{
		Userid: userid,
		Role:   role,
	}
	m.ensureRoleid()
	return m, nil
}

func (m *Model) ensureRoleid() string {
	r := m.Userid + "|" + m.Role
	m.roleid = r
	return r
}

const (
	moduleIDModGet = moduleIDModel + ".GetByID"
)

// GetByID returns a user role model with the given id
func (r *repo) GetByID(userid, role string) (*Model, *governor.Error) {
	var m *Model
	if mRole, code, err := roleModelGet(r.db, (&Model{Userid: userid, Role: role}).ensureRoleid()); err != nil {
		if code == 2 {
			return nil, governor.NewError(moduleIDModGet, "role not found for user", 2, http.StatusNotFound)
		}
		return nil, governor.NewError(moduleIDModGet, err.Error(), 0, http.StatusInternalServerError)
	} else {
		m = mRole
	}
	return m, nil
}

const (
	moduleIDModGetRole = moduleIDModel + ".GetByRole"
)

// GetByRole returns a list of userids with the given role
func (r *repo) GetByRole(role string, limit, offset int) ([]string, *governor.Error) {
	m, err := roleModelGetqUseridEqRoleOrdUserid(r.db, role, true, limit, offset)
	if err != nil {
		return nil, governor.NewError(moduleIDModGetRole, err.Error(), 0, http.StatusInternalServerError)
	}
	userids := make([]string, 0, len(m))
	for _, i := range m {
		userids = append(userids, i.Userid)
	}
	return userids, nil
}

const (
	moduleIDModGetUser = moduleIDModel + ".GetUserRoles"
)

// GetUserRoles returns a list of a user's roles
func (r *repo) GetUserRoles(userid string, limit, offset int) ([]string, *governor.Error) {
	m, err := roleModelGetqRoleEqUseridOrdRole(r.db, userid, true, limit, offset)
	if err != nil {
		return nil, governor.NewError(moduleIDModGetUser, err.Error(), 0, http.StatusInternalServerError)
	}
	roles := make([]string, 0, len(m))
	for _, i := range m {
		roles = append(roles, i.Role)
	}
	return roles, nil
}

const (
	moduleIDModIns = moduleIDModel + ".Insert"
)

// Insert inserts the model into the db
func (r *repo) Insert(m *Model) *governor.Error {
	m.ensureRoleid()
	if code, err := roleModelInsert(r.db, m); err != nil {
		if code == 3 {
			return governor.NewErrorUser(moduleIDModIns, err.Error(), 3, http.StatusBadRequest)
		}
		return governor.NewError(moduleIDModIns, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

const (
	moduleIDModDel = moduleIDModel + ".Delete"
)

// Delete deletes the model in the db
func (r *repo) Delete(m *Model) *governor.Error {
	m.ensureRoleid()
	if err := roleModelDelete(r.db, m); err != nil {
		return governor.NewError(moduleIDModDel, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

const (
	moduleIDModDelUser = moduleIDModel + ".DeleteUserRoles"
	sqlDeleteItem      = "DELETE FROM " + roleModelTableName + " WHERE userid=$1;"
)

// DeleteUserRoles deletes all the roles of a user
func (r *repo) DeleteUserRoles(userid string) *governor.Error {
	_, err := r.db.Exec(sqlDeleteItem, userid)
	if err != nil {
		return governor.NewError(moduleIDModDelUser, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

const (
	moduleIDSetup = moduleID + ".Setup"
)

// Setup creates a new User role table
func (r *repo) Setup() *governor.Error {
	if err := roleModelSetup(r.db); err != nil {
		return governor.NewError(moduleIDSetup, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}
