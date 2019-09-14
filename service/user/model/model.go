package usermodel

import (
	"net/http"
	"time"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/user/role/model"
	"xorkevin.dev/governor/util/rank"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/hunter2"
)

//go:generate forge model -m Model -t users -p user -o model_gen.go Model Info

const (
	uidSize     = 16
	passSaltLen = 32
	passHashLen = 32
	roleLimit   = 1024
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
		UpdateRoles(m *Model, addRoles, rmRoles []string) error
		Update(m *Model) error
		Delete(m *Model) error
		Setup() error
	}

	repo struct {
		db       db.Database
		rolerepo rolemodel.Repo
		hasher   *hunter2.ScryptHasher
		verifier *hunter2.Verifier
	}

	// Model is the db User model
	Model struct {
		Userid       string `model:"userid,VARCHAR(31) PRIMARY KEY" query:"userid,get;updeq,userid;deleq,userid"`
		Username     string `model:"username,VARCHAR(255) NOT NULL UNIQUE" query:"username,get"`
		AuthTags     rank.Rank
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
func New(database db.Database, rolerepo rolemodel.Repo) Repo {
	hasher := hunter2.NewScryptHasher(passHashLen, passSaltLen, hunter2.NewScryptDefaultConfig())
	verifier := hunter2.NewVerifier()
	verifier.RegisterHash(hasher)

	return &repo{
		db:       database,
		rolerepo: rolerepo,
		hasher:   hasher,
		verifier: verifier,
	}
}

// New creates a new User Model
func (r *repo) New(username, password, email, firstname, lastname string, ra rank.Rank) (*Model, error) {
	mUID, err := uid.New(uidSize)
	if err != nil {
		return nil, governor.NewError("Failed to create new uid", http.StatusInternalServerError, err)
	}

	mHash, err := r.hasher.Hash(password)
	if err != nil {
		return nil, governor.NewError("Failed to hash password", http.StatusInternalServerError, err)
	}

	return &Model{
		Userid:       mUID.Base64(),
		Username:     username,
		AuthTags:     ra,
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
	roles, err := r.rolerepo.GetUserRoles(m.Userid, roleLimit, 0)
	if err != nil {
		return governor.NewError("Failed to get roles of user", http.StatusInternalServerError, err)
	}
	m.AuthTags = rank.FromSlice(roles)
	return nil
}

// GetGroup gets information from each user
func (r *repo) GetGroup(limit, offset int) ([]Info, error) {
	m, err := userModelGetInfoOrdUserid(r.db.DB(), true, limit, offset)
	if err != nil {
		return nil, governor.NewError("Failed to get user info", http.StatusInternalServerError, err)
	}
	return m, nil
}

// GetBulk gets information from users
func (r *repo) GetBulk(userids []string) ([]Info, error) {
	m, err := userModelGetInfoSetUserid(r.db.DB(), userids)
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

// GetByID returns a user model with the given id
func (r *repo) GetByID(userid string) (*Model, error) {
	var m *Model
	if mUser, code, err := userModelGetModelByUserid(r.db.DB(), userid); err != nil {
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
	if mUser, code, err := userModelGetModelByUsername(r.db.DB(), username); err != nil {
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
	if mUser, code, err := userModelGetModelByEmail(r.db.DB(), email); err != nil {
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
	roles := make([]*rolemodel.Model, 0, len(m.AuthTags))
	for k := range m.AuthTags {
		roles = append(roles, r.rolerepo.New(m.Userid, k))
	}
	if err := r.rolerepo.InsertBulk(roles); err != nil {
		return governor.NewError("Failed to insert user roles", http.StatusInternalServerError, err)
	}
	return nil
}

// Insert inserts the model into the db
func (r *repo) Insert(m *Model) error {
	if code, err := userModelInsert(r.db.DB(), m); err != nil {
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
func (r *repo) UpdateRoles(m *Model, addRoles, rmRoles []string) error {
	addModels := make([]*rolemodel.Model, 0, len(addRoles))
	for _, i := range addRoles {
		addModels = append(addModels, r.rolerepo.New(m.Userid, i))
	}
	rmModels := make([]*rolemodel.Model, 0, len(rmRoles))
	for _, i := range rmRoles {
		rmModels = append(rmModels, r.rolerepo.New(m.Userid, i))
	}

	if len(rmModels) > 0 {
		if err := r.rolerepo.DeleteBulk(rmModels); err != nil {
			return governor.NewError("Failed to delete roles", http.StatusInternalServerError, err)
		}
	}
	if len(addModels) > 0 {
		if err := r.rolerepo.InsertBulk(addModels); err != nil {
			return governor.NewError("Failed to insert roles", http.StatusInternalServerError, err)
		}
	}
	return nil
}

// Update updates the model in the db
func (r *repo) Update(m *Model) error {
	if err := userModelUpdateModelEqUserid(r.db.DB(), m, m.Userid); err != nil {
		return governor.NewError("Failed to update user", http.StatusInternalServerError, err)
	}
	return nil
}

// Delete deletes the model in the db
func (r *repo) Delete(m *Model) error {
	if err := r.rolerepo.DeleteUserRoles(m.Userid); err != nil {
		return governor.NewError("Failed to delete user roles", http.StatusInternalServerError, err)
	}

	if err := userModelDelEqUserid(r.db.DB(), m.Userid); err != nil {
		return governor.NewError("Failed to delete user", http.StatusInternalServerError, err)
	}
	return nil
}

// Setup creates a new User table
func (r *repo) Setup() error {
	if err := userModelSetup(r.db.DB()); err != nil {
		return governor.NewError("Failed to setup user model", http.StatusInternalServerError, err)
	}
	return nil
}
