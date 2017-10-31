package rolemodel

import (
	"database/sql"
	"fmt"
	"github.com/hackform/governor"
	"github.com/lib/pq"
	"net/http"
)

const (
	tableName     = "userroles"
	moduleID      = "rolemodel"
	moduleIDModel = moduleID + ".Model"
)

type (
	// Model is the db User role model
	Model struct {
		Userid string `json:"userid"`
		Role   string `json:"role"`
	}
)

const (
	moduleIDModNew = moduleIDModel + ".New"
)

// New creates a new User role Model
func New(userid, role string) (*Model, *governor.Error) {
	return &Model{
		Userid: userid,
		Role:   role,
	}, nil
}

func roleid(userid, role string) string {
	return userid + role
}

// Roleid returns the combined userid and role
func (m *Model) Roleid() string {
	return roleid(m.Userid, m.Role)
}

const (
	moduleIDModGet = moduleIDModel + ".GetByID"
)

var (
	sqlGetByID = fmt.Sprintf("SELECT userid, role FROM %s WHERE roleid=$1;", tableName)
)

// GetByID returns a user role model with the given id
func GetByID(db *sql.DB, userid, role string) (*Model, *governor.Error) {
	m := &Model{}
	if err := db.QueryRow(sqlGetByID, roleid(userid, role)).Scan(&m.Userid, &m.Role); err != nil {
		if err == sql.ErrNoRows {
			return nil, governor.NewError(moduleIDModGet, "role not found for user", 2, http.StatusNotFound)
		}
		return nil, governor.NewError(moduleIDModGet, err.Error(), 0, http.StatusInternalServerError)
	}
	return m, nil
}

const (
	moduleIDModGetRole = moduleIDModel + ".GetByRole"
)

var (
	sqlGetByRole = fmt.Sprintf("SELECT userid FROM %s WHERE role=$1 ORDER BY roleid ASC LIMIT $2 OFFSET $3;", tableName)
)

// GetByRole returns a list of userids with the given role
func GetByRole(db *sql.DB, role string, limit, offset int) ([]string, *governor.Error) {
	m := make([]string, 0, limit)
	rows, err := db.Query(sqlGetByRole, role, limit, offset)
	if err != nil {
		return nil, governor.NewError(moduleIDModGetRole, err.Error(), 0, http.StatusInternalServerError)
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, governor.NewError(moduleIDModGetRole, err.Error(), 0, http.StatusInternalServerError)
		}
		m = append(m, s)
	}
	if err := rows.Err(); err != nil {
		return nil, governor.NewError(moduleIDModGetRole, err.Error(), 0, http.StatusInternalServerError)
	}
	return m, nil
}

const (
	moduleIDModGetUser = moduleIDModel + ".GetUserRoles"
)

var (
	sqlGetUser = fmt.Sprintf("SELECT role FROM %s WHERE userid=$1 ORDER BY roleid ASC LIMIT $2 OFFSET $3;", tableName)
)

// GetUserRoles returns a list of a user's roles
func GetUserRoles(db *sql.DB, userid string, limit, offset int) ([]string, *governor.Error) {
	m := make([]string, 0, limit)
	rows, err := db.Query(sqlGetUser, userid, limit, offset)
	if err != nil {
		return nil, governor.NewError(moduleIDModGetUser, err.Error(), 0, http.StatusInternalServerError)
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, governor.NewError(moduleIDModGetUser, err.Error(), 0, http.StatusInternalServerError)
		}
		m = append(m, s)
	}
	if err := rows.Err(); err != nil {
		return nil, governor.NewError(moduleIDModGetUser, err.Error(), 0, http.StatusInternalServerError)
	}
	return m, nil
}

const (
	moduleIDModIns = moduleIDModel + ".Insert"
)

var (
	sqlInsert = fmt.Sprintf("INSERT INTO %s (roleid, userid, role) VALUES ($1, $2, $3);", tableName)
)

// Insert inserts the model into the db
func (m *Model) Insert(db *sql.DB) *governor.Error {
	_, err := db.Exec(sqlInsert, m.Roleid(), m.Userid, m.Role)
	if err != nil {
		if postgresErr, ok := err.(*pq.Error); ok {
			switch postgresErr.Code {
			case "23505": // unique_violation
				return governor.NewErrorUser(moduleIDModIns, err.Error(), 3, http.StatusBadRequest)
			default:
				return governor.NewError(moduleIDModIns, err.Error(), 0, http.StatusInternalServerError)
			}
		}
	}
	return nil
}

const (
	moduleIDModUp = moduleIDModel + ".Update"
)

var (
	sqlUpdate = fmt.Sprintf("UPDATE %s SET (roleid, userid, role) = ($1, $2, $3) WHERE roleid=$1;", tableName)
)

// Update updates the model in the db
func (m *Model) Update(db *sql.DB) *governor.Error {
	_, err := db.Exec(sqlUpdate, m.Roleid(), m.Userid, m.Role)
	if err != nil {
		return governor.NewError(moduleIDModUp, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

const (
	moduleIDModDel = moduleIDModel + ".Delete"
)

var (
	sqlDelete = fmt.Sprintf("DELETE FROM %s WHERE roleid=$1;", tableName)
)

// Delete deletes the model in the db
func (m *Model) Delete(db *sql.DB) *governor.Error {
	_, err := db.Exec(sqlDelete, m.Roleid())
	if err != nil {
		return governor.NewError(moduleIDModDel, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

const (
	moduleIDModDelUser = moduleIDModel + ".DeleteUserRoles"
)

var (
	sqlDeleteItem = fmt.Sprintf("DELETE FROM %s WHERE userid=$1;", tableName)
)

// DeleteUserRoles deletes all the roles of a user
func DeleteUserRoles(db *sql.DB, userid string) *governor.Error {
	_, err := db.Exec(sqlDeleteItem, userid)
	if err != nil {
		return governor.NewError(moduleIDModDelUser, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

const (
	moduleIDSetup = moduleID + ".Setup"
)

var (
	sqlSetup = fmt.Sprintf("CREATE TABLE %s (roleid VARCHAR(512) PRIMARY KEY, userid VARCHAR(255) NOT NULL, role VARCHAR(255) NOT NULL);", tableName)
)

// Setup creates a new User role table
func Setup(db *sql.DB) *governor.Error {
	_, err := db.Exec(sqlSetup)
	if err != nil {
		return governor.NewError(moduleIDSetup, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}
