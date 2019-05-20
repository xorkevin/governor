package usermodel

import (
	"database/sql"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/db"
	"github.com/hackform/governor/service/user/role/model"
	"github.com/hackform/governor/util/rank"
	"github.com/hackform/governor/util/uid"
	"github.com/hackform/hunter2"
	"net/http"
	"strings"
	"time"
)

//go:generate forge model -m Model -t users -p user -o model_gen.go Model Info

const (
	uidTimeSize = 8
	uidRandSize = 8
	passSaltLen = 32
	passHashLen = 32
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
		New(username, password, email, firstname, lastname string, ra rank.Rank) (*Model, error)
		NewEmpty() Model
		NewEmptyPtr() *Model
		ValidatePass(password string, m *Model) (bool, error)
		RehashPass(m *Model, password string) error
		GetRoles(m *Model) error
		GetGroup(limit, offset int) ([]Info, error)
		GetBulk(userids []string) ([]Info, error)
		GetByID(userid string) (*Model, error)
		GetByUsername(username string) (*Model, error)
		GetByEmail(email string) (*Model, error)
		Insert(m *Model) error
		UpdateRoles(m *Model, diff map[string]int) error
		Update(m *Model) error
		Delete(m *Model) error
		Setup() error
		RoleAddAction() int
		RoleRemoveAction() int
	}

	repo struct {
		db       *sql.DB
		rolerepo rolemodel.Repo
		hasher   *hunter2.ScryptHasher
		verifier *hunter2.Verifier
	}

	// Model is the db User model
	Model struct {
		Userid       string `model:"userid,VARCHAR(31) PRIMARY KEY" query:"userid"`
		Username     string `model:"username,VARCHAR(255) NOT NULL UNIQUE" query:"username,get"`
		AuthTags     string
		PassHash     string `model:"pass_hash,VARCHAR(255) NOT NULL" query:"pass_hash"`
		Email        string `model:"email,VARCHAR(255) NOT NULL UNIQUE" query:"email,get"`
		FirstName    string `model:"first_name,VARCHAR(255) NOT NULL" query:"first_name"`
		LastName     string `model:"last_name,VARCHAR(255) NOT NULL" query:"last_name"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL" query:"creation_time"`
	}

	// Info is the metadata of a user
	Info struct {
		Userid    string `query:"userid,getgroup;getgroupset"`
		Username  string `query:"username"`
		Email     string `query:"email"`
		FirstName string `query:"first_name"`
		LastName  string `query:"last_name"`
	}
)

// New creates a new user repository
func New(conf governor.Config, l governor.Logger, database db.Database, rolerepo rolemodel.Repo) Repo {
	l.Info("initialize user repo", nil)

	hasher := hunter2.NewScryptHasher(passHashLen, passSaltLen, hunter2.NewScryptDefaultConfig())
	verifier := hunter2.NewVerifier()
	verifier.RegisterHash(hasher)

	return &repo{
		db:       database.DB(),
		rolerepo: rolerepo,
		hasher:   hasher,
		verifier: verifier,
	}
}

// New creates a new User Model
func (r *repo) New(username, password, email, firstname, lastname string, ra rank.Rank) (*Model, error) {
	mUID, err := uid.NewU(uidTimeSize, uidRandSize)
	if err != nil {
		return nil, governor.NewError("Failed to create new uid", http.StatusInternalServerError, err)
	}

	mHash := ""
	if h, err := r.hasher.Hash(password); err == nil {
		mHash = h
	} else {
		return nil, governor.NewError("Failed to hash password", http.StatusInternalServerError, err)
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

func ParseB64ID(userid string) (*uid.UID, error) {
	return uid.FromBase64(uidTimeSize, 0, uidRandSize, userid)
}

// ValidatePass validates the password against a hash
func (r *repo) ValidatePass(password string, m *Model) (bool, error) {
	ok, err := r.verifier.Verify(password, m.PassHash)
	if err != nil {
		return false, governor.NewError("Failed to verify password", http.StatusInternalServerError, err)
	}
	return ok, nil
}

// RehashPass updates the password with a new hash
func (r *repo) RehashPass(m *Model, password string) error {
	mHash, err := r.hasher.Hash(password)
	if err != nil {
		return governor.NewError("Failed to rehash password", http.StatusInternalServerError, err)
	}
	m.PassHash = mHash
	return nil
}

// GetRoles gets the roles of the user for the model
func (r *repo) GetRoles(m *Model) error {
	roles, err := r.rolerepo.GetUserRoles(m.Userid, 1024, 0)
	if err != nil {
		return governor.NewError("Failed to get roles of user", http.StatusInternalServerError, err)
	}
	m.AuthTags = strings.Join(roles, ",")
	return nil
}

// GetGroup gets information from each user
func (r *repo) GetGroup(limit, offset int) ([]Info, error) {
	m, err := userModelGetInfoOrdUserid(r.db, true, limit, offset)
	if err != nil {
		return nil, governor.NewError("Failed to get user info", http.StatusInternalServerError, err)
	}
	return m, nil
}

// GetBulk gets information from users
func (r *repo) GetBulk(userids []string) ([]Info, error) {
	m, err := userModelGetInfoSetUserid(r.db, userids)
	if err != nil {
		return nil, governor.NewError("Failed to get user info of userids", http.StatusInternalServerError, err)
	}
	return m, nil
}

func (r *repo) getApplyRoles(m *Model) (*Model, error) {
	if err := r.GetRoles(m); err != nil {
		return nil, err
	}
	return m, nil
}

// GetByID returns a user model with the given base64 id
func (r *repo) GetByID(userid string) (*Model, error) {
	var m *Model
	if mUser, code, err := userModelGet(r.db, userid); err != nil {
		if code == 2 {
			return nil, governor.NewError("No user found with that id", http.StatusNotFound, err)
		}
		return nil, governor.NewError("Failed to get user", http.StatusInternalServerError, err)
	} else {
		m = mUser
	}
	return r.getApplyRoles(m)
}

// GetByUsername returns a user model with the given username
func (r *repo) GetByUsername(username string) (*Model, error) {
	var m *Model
	if mUser, code, err := userModelGetModelByUsername(r.db, username); err != nil {
		if code == 2 {
			return nil, governor.NewError("No user found with that username", http.StatusNotFound, err)
		}
		return nil, governor.NewError("Failed to get user by username", http.StatusInternalServerError, err)
	} else {
		m = mUser
	}
	return r.getApplyRoles(m)
}

// GetByEmail returns a user model with the given email
func (r *repo) GetByEmail(email string) (*Model, error) {
	var m *Model
	if mUser, code, err := userModelGetModelByEmail(r.db, email); err != nil {
		if code == 2 {
			return nil, governor.NewError("No user found with that email", http.StatusNotFound, err)
		}
		return nil, governor.NewError("Failed to get user by email", http.StatusInternalServerError, err)
	} else {
		m = mUser
	}
	return r.getApplyRoles(m)
}

func (r *repo) insertRoles(m *Model) error {
	roles := strings.Split(m.AuthTags, ",")
	for _, i := range roles {
		rModel, err := r.rolerepo.New(m.Userid, i)
		if err != nil {
			return governor.NewError("Failed to create user role", http.StatusInternalServerError, err)
		}
		if err := r.rolerepo.Insert(rModel); err != nil {
			return governor.NewError("Failed to insert user role", http.StatusInternalServerError, err)
		}
	}
	return nil
}

// Insert inserts the model into the db
func (r *repo) Insert(m *Model) error {
	if code, err := userModelInsert(r.db, m); err != nil {
		if code == 3 {
			return governor.NewError("User id must be unique", http.StatusBadRequest, err)
		}
		return governor.NewError("Failed to insert user", http.StatusInternalServerError, err)
	}
	if err := r.insertRoles(m); err != nil {
		return err
	}
	return nil
}

// UpdateRoles updates the model's roles into the db
func (r *repo) UpdateRoles(m *Model, diff map[string]int) error {
	for k, v := range diff {
		if originalRole, err := r.rolerepo.GetByID(m.Userid, k); err == nil {
			switch v {
			case roleRemove:
				if err := r.rolerepo.Delete(originalRole); err != nil {
					return governor.NewError("Failed to delete role", http.StatusInternalServerError, err)
				}
			}
		} else if governor.ErrorStatus(err) == http.StatusNotFound {
			switch v {
			case roleAdd:
				if roleM, err := r.rolerepo.New(m.Userid, k); err == nil {
					if err := r.rolerepo.Insert(roleM); err != nil {
						return governor.NewError("Failed to insert role", http.StatusInternalServerError, err)
					}
				} else {
					return governor.NewError("Failed to create role", http.StatusInternalServerError, err)
				}
			}
		} else {
			return governor.NewError("Failed to update role", http.StatusInternalServerError, err)
		}
	}
	return nil
}

// Update updates the model in the db
func (r *repo) Update(m *Model) error {
	if err := userModelUpdate(r.db, m); err != nil {
		return governor.NewError("Failed to update user", http.StatusInternalServerError, err)
	}
	return nil
}

// Delete deletes the model in the db
func (r *repo) Delete(m *Model) error {
	if err := r.rolerepo.DeleteUserRoles(m.Userid); err != nil {
		return governor.NewError("Failed to delete user roles", http.StatusInternalServerError, err)
	}

	if err := userModelDelete(r.db, m); err != nil {
		return governor.NewError("Failed to delete user", http.StatusInternalServerError, err)
	}
	return nil
}

// Setup creates a new User table
func (r *repo) Setup() error {
	if err := userModelSetup(r.db); err != nil {
		return governor.NewError("Failed to setup user model", http.StatusInternalServerError, err)
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
