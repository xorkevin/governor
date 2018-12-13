package usermodel

import (
	"database/sql"
	"fmt"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/db"
	"github.com/hackform/governor/service/user/role/model"
	"github.com/hackform/governor/util/hash"
	"github.com/hackform/governor/util/rank"
	"github.com/hackform/governor/util/uid"
	"net/http"
	"strconv"
	"strings"
	"time"
)

//go:generate go run ../../../gen/model.go -- model_gen.go user users Model Model Info

const (
	uidTimeSize = 8
	uidRandSize = 8
	moduleID    = "usermodel"
)

const (
	// roleAdd indicates adding a role in diff
	roleAdd = iota
	// roleRemove indicates removing a role in diff
	roleRemove
)

type (
	// Repo is a user repository
	Repo interface {
		New(username, password, email, firstname, lastname string, ra rank.Rank) (*Model, *governor.Error)
		NewEmpty() Model
		NewEmptyPtr() *Model
		GetRoles(m *Model) *governor.Error
		GetGroup(limit, offset int) ([]Info, *governor.Error)
		GetBulk(userids []string) ([]Info, *governor.Error)
		GetByID(userid string) (*Model, *governor.Error)
		GetByUsername(username string) (*Model, *governor.Error)
		GetByEmail(email string) (*Model, *governor.Error)
		Insert(m *Model) *governor.Error
		UpdateRoles(m *Model, diff map[string]int) *governor.Error
		Update(m *Model) *governor.Error
		Delete(m *Model) *governor.Error
		Setup() *governor.Error
		RoleAddAction() int
		RoleRemoveAction() int
	}

	repo struct {
		db       *sql.DB
		rolerepo rolemodel.Repo
	}

	// Model is the db User model
	Model struct {
		Userid       string `model:"userid,VARCHAR(31) PRIMARY KEY" query:"userid"`
		Username     string `model:"username,VARCHAR(255) NOT NULL UNIQUE" query:"username,get"`
		AuthTags     string
		PassHash     []byte `model:"pass_hash,BYTEA NOT NULL" query:"pass_hash"`
		Email        string `model:"email,VARCHAR(255) NOT NULL UNIQUE" query:"email,get"`
		FirstName    string `model:"first_name,VARCHAR(255) NOT NULL" query:"first_name"`
		LastName     string `model:"last_name,VARCHAR(255) NOT NULL" query:"last_name"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL" query:"creation_time"`
	}

	// Info is the metadata of a user
	Info struct {
		Userid   string `query:"userid,getgroup"`
		Username string `query:"username"`
		Email    string `query:"email"`
	}
)

// New creates a new user repository
func New(conf governor.Config, l governor.Logger, database db.Database, rolerepo rolemodel.Repo) Repo {
	l.Info("initialized user repo", moduleID, "initialize user repo", 0, nil)
	return &repo{
		db:       database.DB(),
		rolerepo: rolerepo,
	}
}

const (
	moduleIDModNew = moduleID + ".New"
)

// New creates a new User Model
func (r *repo) New(username, password, email, firstname, lastname string, ra rank.Rank) (*Model, *governor.Error) {
	mUID, err := uid.NewU(uidTimeSize, uidRandSize)
	if err != nil {
		err.AddTrace(moduleIDModNew)
		return nil, err
	}

	mHash, err := hash.Hash(password)
	if err != nil {
		err.AddTrace(moduleIDModNew)
		return nil, err
	}

	return &Model{
		Userid:       mUID.Base64(),
		Username:     username,
		AuthTags:     ra.Stringify(),
		PassHash:     mHash,
		Email:        email,
		FirstName:    firstname,
		LastName:     lastname,
		CreationTime: time.Now().Unix(),
	}, nil
}

func (r *repo) NewEmpty() Model {
	return Model{}
}

func (r *repo) NewEmptyPtr() *Model {
	return &Model{}
}

func ParseB64ID(userid string) (*uid.UID, *governor.Error) {
	return uid.FromBase64(uidTimeSize, 0, uidRandSize, userid)
}

// ValidatePass validates the password against a hash
func (m *Model) ValidatePass(password string) bool {
	return hash.Verify(password, m.PassHash)
}

const (
	moduleIDHash = moduleID + ".Hash"
)

// RehashPass updates the password with a new hash
func (m *Model) RehashPass(password string) *governor.Error {
	mHash, err := hash.Hash(password)
	if err != nil {
		err.AddTrace(moduleIDHash)
		return err
	}
	m.PassHash = mHash
	return nil
}

const (
	moduleIDModGetRoles = moduleID + ".GetRoles"
)

// GetRoles gets the roles of the user for the model
func (r *repo) GetRoles(m *Model) *governor.Error {
	roles, err := r.rolerepo.GetUserRoles(m.Userid, 1024, 0)
	if err != nil {
		err.AddTrace(moduleIDModGetRoles)
		return err
	}
	m.AuthTags = strings.Join(roles, ",")
	return nil
}

const (
	moduleIDModGetGroup = moduleID + ".GetGroup"
)

// GetGroup gets information from each user
func (r *repo) GetGroup(limit, offset int) ([]Info, *governor.Error) {
	m, err := userModelGetInfoOrdUserid(r.db, true, limit, offset)
	if err != nil {
		return nil, governor.NewError(moduleIDModGetGroup, err.Error(), 0, http.StatusInternalServerError)
	}
	return m, nil
}

const (
	moduleIDModGetBulk = moduleID + ".GetBulk"
	sqlGetBulk         = "SELECT userid, username, email FROM " + userModelTableName + " WHERE userid IN (VALUES %s);"
)

// GetBulk gets information from users
func (r *repo) GetBulk(userids []string) ([]Info, *governor.Error) {
	placeholderStart := 1
	placeholders := make([]string, 0, len(userids))
	for i := range userids {
		placeholders = append(placeholders, "($"+strconv.Itoa(i+placeholderStart)+")")
	}

	args := make([]interface{}, 0, len(userids))
	for _, i := range userids {
		args = append(args, i)
	}

	stmt := fmt.Sprintf(sqlGetBulk, strings.Join(placeholders, ","))

	m := make([]Info, 0, len(userids))
	rows, err := r.db.Query(stmt, args...)
	if err != nil {
		return nil, governor.NewError(moduleIDModGetBulk, err.Error(), 0, http.StatusInternalServerError)
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		i := Info{}
		if err := rows.Scan(&i.Userid, &i.Username, &i.Email); err != nil {
			return nil, governor.NewError(moduleIDModGetBulk, err.Error(), 0, http.StatusInternalServerError)
		}
		m = append(m, i)
	}
	if err := rows.Err(); err != nil {
		return nil, governor.NewError(moduleIDModGetBulk, err.Error(), 0, http.StatusInternalServerError)
	}
	return m, nil
}

const (
	moduleIDModGet = moduleID + ".Get"
)

func (r *repo) getApplyRoles(m *Model) (*Model, *governor.Error) {
	if err := r.GetRoles(m); err != nil {
		err.AddTrace(moduleIDModGet)
		return nil, err
	}
	return m, nil
}

// GetByID returns a user model with the given base64 id
func (r *repo) GetByID(userid string) (*Model, *governor.Error) {
	var m *Model
	if mUser, code, err := userModelGet(r.db, userid); err != nil {
		if code == 2 {
			return nil, governor.NewError(moduleIDModGet, "no user found with that id", 2, http.StatusNotFound)
		}
		return nil, governor.NewError(moduleIDModGet, err.Error(), 0, http.StatusInternalServerError)
	} else {
		m = mUser
	}
	return r.getApplyRoles(m)
}

// GetByUsername returns a user model with the given username
func (r *repo) GetByUsername(username string) (*Model, *governor.Error) {
	var m *Model
	if mUser, code, err := userModelGetModelByUsername(r.db, username); err != nil {
		if code == 2 {
			return nil, governor.NewError(moduleIDModGet, "no user found with that username", 2, http.StatusNotFound)
		}
		return nil, governor.NewError(moduleIDModGet, err.Error(), 0, http.StatusInternalServerError)
	} else {
		m = mUser
	}
	return r.getApplyRoles(m)
}

// GetByEmail returns a user model with the given email
func (r *repo) GetByEmail(email string) (*Model, *governor.Error) {
	var m *Model
	if mUser, code, err := userModelGetModelByEmail(r.db, email); err != nil {
		if code == 2 {
			return nil, governor.NewError(moduleIDModGet, "no user found with that email", 2, http.StatusNotFound)
		}
		return nil, governor.NewError(moduleIDModGet, err.Error(), 0, http.StatusInternalServerError)
	} else {
		m = mUser
	}
	return r.getApplyRoles(m)
}

const (
	moduleIDModInsRoles = moduleID + ".InsertRoles"
)

func (r *repo) insertRoles(m *Model) *governor.Error {
	roles := strings.Split(m.AuthTags, ",")
	for _, i := range roles {
		rModel, err := r.rolerepo.New(m.Userid, i)
		if err != nil {
			err.AddTrace(moduleIDModInsRoles)
			return err
		}
		if err := r.rolerepo.Insert(rModel); err != nil {
			err.AddTrace(moduleIDModInsRoles)
			return err
		}
	}
	return nil
}

const (
	moduleIDModIns = moduleID + ".Insert"
)

// Insert inserts the model into the db
func (r *repo) Insert(m *Model) *governor.Error {
	if code, err := userModelInsert(r.db, m); err != nil {
		if code == 3 {
			return governor.NewError(moduleIDModIns, err.Error(), 3, http.StatusBadRequest)
		}
		return governor.NewError(moduleIDModIns, err.Error(), 0, http.StatusInternalServerError)
	}
	if err := r.insertRoles(m); err != nil {
		err.AddTrace(moduleIDModIns)
		return err
	}
	return nil
}

const (
	moduleIDModUpRoles = moduleID + ".UpdateRoles"
)

// UpdateRoles updates the model's roles into the db
func (r *repo) UpdateRoles(m *Model, diff map[string]int) *governor.Error {
	for k, v := range diff {
		if originalRole, err := r.rolerepo.GetByID(m.Userid, k); err == nil {
			switch v {
			case roleRemove:
				if err := r.rolerepo.Delete(originalRole); err != nil {
					err.AddTrace(moduleIDModUpRoles)
					return err
				}
			}
		} else if err.Code() == 2 {
			switch v {
			case roleAdd:
				if roleM, err := r.rolerepo.New(m.Userid, k); err == nil {
					if err := r.rolerepo.Insert(roleM); err != nil {
						err.AddTrace(moduleIDModUpRoles)
						return err
					}
				} else {
					err.AddTrace(moduleIDModUpRoles)
					return err
				}
			}
		} else {
			err.AddTrace(moduleIDModUpRoles)
			return err
		}
	}
	return nil
}

const (
	moduleIDModUp = moduleID + ".Update"
)

// Update updates the model in the db
func (r *repo) Update(m *Model) *governor.Error {
	if err := userModelUpdate(r.db, m); err != nil {
		return governor.NewError(moduleIDModUp, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

const (
	moduleIDModDel = moduleID + ".Delete"
)

// Delete deletes the model in the db
func (r *repo) Delete(m *Model) *governor.Error {
	if err := r.rolerepo.DeleteUserRoles(m.Userid); err != nil {
		err.AddTrace(moduleIDModDel)
		return err
	}

	if err := userModelDelete(r.db, m); err != nil {
		return governor.NewError(moduleIDModDel, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

const (
	moduleIDSetup = moduleID + ".Setup"
)

// Setup creates a new User table
func (r *repo) Setup() *governor.Error {
	if err := userModelSetup(r.db); err != nil {
		return governor.NewError(moduleIDSetup, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

// RoleRemoveAction returns the action to add a role
func (r *repo) RoleAddAction() int {
	return roleAdd
}

// RoleRemoveAction returns the action to remove a role
func (r *repo) RoleRemoveAction() int {
	return roleRemove
}
