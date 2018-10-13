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
	"github.com/lib/pq"
	"net/http"
	"strings"
	"time"
)

const (
	uidTimeSize = 8
	uidRandSize = 8
	tableName   = "users"
	moduleID    = "usermodel"
)

const (
	// RoleAdd indicates adding a role in diff
	RoleAdd = iota
	// RoleRemove indicates removing a role in diff
	RoleRemove
)

type (
	// Repo is a user repository
	Repo interface {
		New(username, password, email, firstname, lastname string, ra rank.Rank) (*Model, *governor.Error)
		NewEmpty() Model
		GetRoles(m *Model) *governor.Error
		GetGroup(limit, offset int) ([]Info, *governor.Error)
		GetByIDB64(idb64 string) (*Model, *governor.Error)
		GetByUsername(username string) (*Model, *governor.Error)
		GetByEmail(email string) (*Model, *governor.Error)
		Insert(m *Model) *governor.Error
		UpdateRoles(m *Model, diff map[string]int) *governor.Error
		Update(m *Model) *governor.Error
		Delete(m *Model) *governor.Error
		Setup() *governor.Error
	}

	repo struct {
		db       *sql.DB
		rolerepo rolemodel.Repo
	}

	// Model is the db User model
	Model struct {
		ID
		Auth
		Passhash
		Props
	}

	// ID is user identification
	ID struct {
		Userid   []byte `json:"userid"`
		Username string `json:"username"`
	}

	// Auth manages user permissions
	Auth struct {
		Tags string `json:"auth_tags"`
	}

	// Passhash controls the user password
	Passhash struct {
		Hash []byte `json:"pass_hash"`
	}

	// Props stores user info
	Props struct {
		Email        string `json:"email"`
		FirstName    string `json:"first_name"`
		LastName     string `json:"last_name"`
		CreationTime int64  `json:"creation_time"`
	}

	// Info is the metadata of a user
	Info struct {
		Userid []byte `json:"userid"`
		Email  string `json:"email"`
	}
)

// New creates a new user repository
func New(database db.Database, rolerepo rolemodel.Repo) (Repo, *governor.Error) {
	return &repo{
		db:       database.DB(),
		rolerepo: rolerepo,
	}, nil
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
		ID: ID{
			Userid:   mUID.Bytes(),
			Username: username,
		},
		Auth: Auth{
			Tags: ra.Stringify(),
		},
		Passhash: Passhash{
			Hash: mHash,
		},
		Props: Props{
			Email:        email,
			FirstName:    firstname,
			LastName:     lastname,
			CreationTime: time.Now().Unix(),
		},
	}, nil
}

func (r *repo) NewEmpty() Model {
	return Model{}
}

// ValidatePass validates the password against a hash
func (m *Model) ValidatePass(password string) bool {
	return hash.Verify(password, m.Passhash.Hash)
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
	m.Passhash.Hash = mHash
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
	m.Auth.Tags = strings.Join(roles, ",")
	return nil
}

const (
	moduleIDModGetGroup = moduleID + ".GetGroup"
)

var (
	sqlGetGroup = fmt.Sprintf("SELECT userid, email FROM %s ORDER BY userid ASC LIMIT $1 OFFSET $2;", tableName)
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
		if err := rows.Scan(&i.Userid, &i.Email); err != nil {
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
	moduleIDModGet64 = moduleID + ".GetByIDB64"
)

var (
	sqlGetByIDB64 = fmt.Sprintf("SELECT userid, username, pass_hash, email, first_name, last_name, creation_time FROM %s WHERE userid=$1;", tableName)
)

// ParseB64ToUID converts a userid in base64 into a UID
func ParseB64ToUID(idb64 string) (*uid.UID, *governor.Error) {
	return uid.FromBase64TRSplit(idb64)
}

// GetByIDB64 returns a user model with the given base64 id
func (r *repo) GetByIDB64(idb64 string) (*Model, *governor.Error) {
	u, err := ParseB64ToUID(idb64)
	if err != nil {
		err.AddTrace(moduleIDModGet64)
		return nil, err
	}
	mUser := &Model{}
	if err := r.db.QueryRow(sqlGetByIDB64, u.Bytes()).Scan(&mUser.Userid, &mUser.Username, &mUser.Passhash.Hash, &mUser.Email, &mUser.FirstName, &mUser.LastName, &mUser.CreationTime); err != nil {
		if err == sql.ErrNoRows {
			return nil, governor.NewError(moduleIDModGet64, "no user found with that id", 2, http.StatusNotFound)
		}
		return nil, governor.NewError(moduleIDModGet64, err.Error(), 0, http.StatusInternalServerError)
	}
	if err := r.GetRoles(mUser); err != nil {
		err.AddTrace(moduleIDModGet64)
		return nil, err
	}
	return mUser, nil
}

const (
	moduleIDModGetUN = moduleID + ".GetByUsername"
)

var (
	sqlGetByUsername = fmt.Sprintf("SELECT userid, username, pass_hash, email, first_name, last_name, creation_time FROM %s WHERE username=$1;", tableName)
)

// GetByUsername returns a user model with the given username
func (r *repo) GetByUsername(username string) (*Model, *governor.Error) {
	mUser := &Model{}
	if err := r.db.QueryRow(sqlGetByUsername, username).Scan(&mUser.Userid, &mUser.Username, &mUser.Passhash.Hash, &mUser.Email, &mUser.FirstName, &mUser.LastName, &mUser.CreationTime); err != nil {
		if err == sql.ErrNoRows {
			return nil, governor.NewError(moduleIDModGetUN, "no user found with that username", 2, http.StatusNotFound)
		}
		return nil, governor.NewError(moduleIDModGetUN, err.Error(), 0, http.StatusInternalServerError)
	}
	if err := r.GetRoles(mUser); err != nil {
		err.AddTrace(moduleIDModGetUN)
		return nil, err
	}
	return mUser, nil
}

const (
	moduleIDModGetEm = moduleID + ".GetByEmail"
)

var (
	sqlGetByEmail = fmt.Sprintf("SELECT userid, username, pass_hash, email, first_name, last_name, creation_time FROM %s WHERE email=$1;", tableName)
)

// GetByEmail returns a user model with the given email
func (r *repo) GetByEmail(email string) (*Model, *governor.Error) {
	mUser := &Model{}
	if err := r.db.QueryRow(sqlGetByEmail, email).Scan(&mUser.Userid, &mUser.Username, &mUser.Passhash.Hash, &mUser.Email, &mUser.FirstName, &mUser.LastName, &mUser.CreationTime); err != nil {
		if err == sql.ErrNoRows {
			return nil, governor.NewError(moduleIDModGetEm, "no user found with that email", 2, http.StatusNotFound)
		}
		return nil, governor.NewError(moduleIDModGetEm, err.Error(), 0, http.StatusInternalServerError)
	}
	if err := r.GetRoles(mUser); err != nil {
		err.AddTrace(moduleIDModGetEm)
		return nil, err
	}
	return mUser, nil
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
	roles := strings.Split(m.Auth.Tags, ",")
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

var (
	sqlInsert = fmt.Sprintf("INSERT INTO %s (userid, username, pass_hash, email, first_name, last_name, creation_time) VALUES ($1, $2, $3, $4, $5, $6, $7);", tableName)
)

// Insert inserts the model into the db
func (r *repo) Insert(m *Model) *governor.Error {
	_, err := r.db.Exec(sqlInsert, m.Userid, m.Username, m.Passhash.Hash, m.Email, m.FirstName, m.LastName, m.CreationTime)
	if err != nil {
		if postgresErr, ok := err.(*pq.Error); ok {
			switch postgresErr.Code {
			case "23505": // unique_violation
				return governor.NewError(moduleIDModIns, err.Error(), 3, http.StatusBadRequest)
			default:
				return governor.NewError(moduleIDModIns, err.Error(), 0, http.StatusInternalServerError)
			}
		}
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
			case RoleRemove:
				if err := r.rolerepo.Delete(originalRole); err != nil {
					err.AddTrace(moduleIDModUpRoles)
					return err
				}
			}
		} else if err.Code() == 2 {
			switch v {
			case RoleAdd:
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

var (
	sqlUpdate = fmt.Sprintf("UPDATE %s SET (userid, username, pass_hash, email, first_name, last_name, creation_time) = ($1, $2, $3, $4, $5, $6, $7) WHERE userid = $1;", tableName)
)

// Update updates the model in the db
func (r *repo) Update(m *Model) *governor.Error {
	_, err := r.db.Exec(sqlUpdate, m.Userid, m.Username, m.Passhash.Hash, m.Email, m.FirstName, m.LastName, m.CreationTime)
	if err != nil {
		return governor.NewError(moduleIDModUp, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

const (
	moduleIDModDel = moduleID + ".Delete"
)

var (
	sqlDelete = fmt.Sprintf("DELETE FROM %s WHERE userid = $1;", tableName)
)

// Delete deletes the model in the db
func (r *repo) Delete(m *Model) *governor.Error {
	if _, err := r.db.Exec(sqlDelete, m.Userid); err != nil {
		return governor.NewError(moduleIDModDel, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

const (
	moduleIDSetup = moduleID + ".Setup"
)

var (
	sqlSetup = fmt.Sprintf("CREATE TABLE %s (userid BYTEA PRIMARY KEY, username VARCHAR(255) NOT NULL UNIQUE, pass_hash BYTEA NOT NULL, email VARCHAR(4096) NOT NULL UNIQUE, first_name VARCHAR(255) NOT NULL, last_name VARCHAR(255) NOT NULL, creation_time BIGINT NOT NULL);", tableName)
)

// Setup creates a new User table
func (r *repo) Setup() *governor.Error {
	_, err := r.db.Exec(sqlSetup)
	if err != nil {
		return governor.NewError(moduleIDSetup, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}
