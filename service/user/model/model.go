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

//go:generate go run ../../../gen/model.go -- model_gen.go user users Model

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
		GetByIDB64(idb64 string) (*Model, *governor.Error)
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
		Userid       []byte `model:"userid,BYTEA PRIMARY KEY"`
		Username     string `model:"username,VARCHAR(255) NOT NULL UNIQUE"`
		AuthTags     string
		PassHash     []byte `model:"pass_hash,BYTEA NOT NULL"`
		Email        string `model:"email,VARCHAR(4096) NOT NULL UNIQUE"`
		FirstName    string `model:"first_name,VARCHAR(255) NOT NULL"`
		LastName     string `model:"last_name,VARCHAR(255) NOT NULL"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL"`
	}

	// Info is the metadata of a user
	Info struct {
		Userid   []byte `json:"userid"`
		Username string `json:"username"`
		Email    string `json:"email"`
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
		Userid:       mUID.Bytes(),
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
	moduleIDModB64 = moduleID + ".IDBase64"
)

// ParseUIDToB64 converts a UID userid into base64
func ParseUIDToB64(userid []byte) (*uid.UID, *governor.Error) {
	return uid.FromBytesTRSplit(userid)
}

// IDBase64 returns the userid as a base64 encoded string
func (m *Model) IDBase64() (string, *governor.Error) {
	u, err := ParseUIDToB64(m.Userid)
	if err != nil {
		err.AddTrace(moduleIDModB64)
		return "", err
	}
	return u.Base64(), nil
}

// IDBase64 returns the userid as a base64 encoded string
func (m *Info) IDBase64() (string, *governor.Error) {
	u, err := ParseUIDToB64(m.Userid)
	if err != nil {
		err.AddTrace(moduleIDModB64)
		return "", err
	}
	return u.Base64(), nil
}

const (
	moduleIDModGetRoles = moduleID + ".GetRoles"
)

// GetRoles gets the roles of the user for the model
func (r *repo) GetRoles(m *Model) *governor.Error {
	idb64, err := m.IDBase64()
	if err != nil {
		err.AddTrace(moduleIDModGetRoles)
		return err
	}
	roles, err := r.rolerepo.GetUserRoles(idb64, 1024, 0)
	if err != nil {
		err.AddTrace(moduleIDModGetRoles)
		return err
	}
	m.AuthTags = strings.Join(roles, ",")
	return nil
}

const (
	moduleIDModGetGroup = moduleID + ".GetGroup"
	sqlGetGroup         = "SELECT userid, username, email FROM " + userModelTableName + " ORDER BY userid ASC LIMIT $1 OFFSET $2;"
)

// GetGroup gets information from each user
func (r *repo) GetGroup(limit, offset int) ([]Info, *governor.Error) {
	m := make([]Info, 0, limit)
	rows, err := r.db.Query(sqlGetGroup, limit, offset)
	if err != nil {
		return nil, governor.NewError(moduleIDModGetGroup, err.Error(), 0, http.StatusInternalServerError)
	}
	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()
	for rows.Next() {
		i := Info{}
		if err := rows.Scan(&i.Userid, &i.Username, &i.Email); err != nil {
			return nil, governor.NewError(moduleIDModGetGroup, err.Error(), 0, http.StatusInternalServerError)
		}
		m = append(m, i)
	}
	if err := rows.Err(); err != nil {
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
	uids := make([]interface{}, 0, len(userids))
	for _, i := range userids {
		u, err := ParseB64ToUID(i)
		if err != nil {
			err.AddTrace(moduleIDModGetBulk)
			err.SetErrorUser()
			return nil, err
		}
		uids = append(uids, u.Bytes())
	}

	placeholderStart := 1
	placeholders := make([]string, 0, len(userids))
	for i := range uids {
		if i == 0 {
			placeholders = append(placeholders, "($"+strconv.Itoa(i+placeholderStart)+"::BYTEA)")
		} else {
			placeholders = append(placeholders, "($"+strconv.Itoa(i+placeholderStart)+")")
		}
	}

	stmt := fmt.Sprintf(sqlGetBulk, strings.Join(placeholders, ","))

	m := make([]Info, 0, len(userids))
	rows, err := r.db.Query(stmt, uids...)
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

func (r *repo) getByID(userid []byte) (*Model, *governor.Error) {
	var m *Model
	if mUser, code, err := userModelGet(r.db, userid); err != nil {
		if code == 2 {
			return nil, governor.NewError(moduleIDModGet, "no user found with that id", 2, http.StatusNotFound)
		}
		return nil, governor.NewError(moduleIDModGet, err.Error(), 0, http.StatusInternalServerError)
	} else {
		m = mUser
	}
	if err := r.GetRoles(m); err != nil {
		err.AddTrace(moduleIDModGet)
		return nil, err
	}
	return m, nil
}

// ParseB64ToUID converts a userid in base64 into a UID
func ParseB64ToUID(idb64 string) (*uid.UID, *governor.Error) {
	return uid.FromBase64TRSplit(idb64)
}

// GetByIDB64 returns a user model with the given base64 id
func (r *repo) GetByIDB64(idb64 string) (*Model, *governor.Error) {
	u, err := ParseB64ToUID(idb64)
	if err != nil {
		err.AddTrace(moduleIDModGet)
		return nil, err
	}
	return r.getByID(u.Bytes())
}

const (
	sqlGetByUsername = "SELECT userid FROM " + userModelTableName + " WHERE username=$1;"
)

// GetByUsername returns a user model with the given username
func (r *repo) GetByUsername(username string) (*Model, *governor.Error) {
	var userid []byte
	if err := r.db.QueryRow(sqlGetByUsername, username).Scan(&userid); err != nil {
		if err == sql.ErrNoRows {
			return nil, governor.NewError(moduleIDModGet, "no user found with that username", 2, http.StatusNotFound)
		}
		return nil, governor.NewError(moduleIDModGet, err.Error(), 0, http.StatusInternalServerError)
	}
	return r.getByID(userid)
}

const (
	moduleIDModGetEm = moduleID + ".GetByEmail"
	sqlGetByEmail    = "SELECT userid FROM " + userModelTableName + " WHERE email=$1;"
)

// GetByEmail returns a user model with the given email
func (r *repo) GetByEmail(email string) (*Model, *governor.Error) {
	var userid []byte
	if err := r.db.QueryRow(sqlGetByEmail, email).Scan(&userid); err != nil {
		if err == sql.ErrNoRows {
			return nil, governor.NewError(moduleIDModGet, "no user found with that email", 2, http.StatusNotFound)
		}
		return nil, governor.NewError(moduleIDModGet, err.Error(), 0, http.StatusInternalServerError)
	}
	return r.getByID(userid)
}

const (
	moduleIDModInsRoles = moduleID + ".InsertRoles"
)

func (r *repo) insertRoles(m *Model) *governor.Error {
	idb64, err := m.IDBase64()
	if err != nil {
		err.AddTrace(moduleIDModInsRoles)
		return err
	}
	roles := strings.Split(m.AuthTags, ",")
	for _, i := range roles {
		rModel, err := r.rolerepo.New(idb64, i)
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
	idb64, err := m.IDBase64()
	if err != nil {
		err.AddTrace(moduleIDModUpRoles)
		return err
	}
	for k, v := range diff {
		if originalRole, err := r.rolerepo.GetByID(idb64, k); err == nil {
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
				if roleM, err := r.rolerepo.New(idb64, k); err == nil {
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
	idb64, err := m.IDBase64()
	if err != nil {
		err.AddTrace(moduleIDModDel)
		return err
	}

	if err := r.rolerepo.DeleteUserRoles(idb64); err != nil {
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
