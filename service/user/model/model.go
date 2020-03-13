package usermodel

import (
	"net/http"
	"time"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/hunter2"
)

//go:generate forge model -m Model -t users -p user -o model_gen.go Model Info

const (
	uidSize     = 16
	passSaltLen = 32
	passHashLen = 32
)

type (
	// Repo is a user repository
	Repo interface {
		New(username, password, email, firstname, lastname string) (*Model, error)
		NewEmpty() Model
		NewEmptyPtr() *Model
		ValidatePass(password string, m *Model) (bool, error)
		RehashPass(m *Model, password string) error
		GetGroup(limit, offset int) ([]Info, error)
		GetBulk(userids []string) ([]Info, error)
		GetByID(userid string) (*Model, error)
		GetByUsername(username string) (*Model, error)
		GetByEmail(email string) (*Model, error)
		Insert(m *Model) error
		Update(m *Model) error
		Delete(m *Model) error
		Setup() error
	}

	repo struct {
		db       db.Database
		hasher   *hunter2.ScryptHasher
		verifier *hunter2.Verifier
	}

	// Model is the db User model
	Model struct {
		Userid       string `model:"userid,VARCHAR(31) PRIMARY KEY" query:"userid,getoneeq,userid;updeq,userid;deleq,userid"`
		Username     string `model:"username,VARCHAR(255) NOT NULL UNIQUE" query:"username,getoneeq,username"`
		PassHash     string `model:"pass_hash,VARCHAR(255) NOT NULL" query:"pass_hash"`
		Email        string `model:"email,VARCHAR(255) NOT NULL UNIQUE" query:"email,getoneeq,email"`
		FirstName    string `model:"first_name,VARCHAR(255) NOT NULL" query:"first_name"`
		LastName     string `model:"last_name,VARCHAR(255) NOT NULL" query:"last_name"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL" query:"creation_time"`
	}

	// Info is the metadata of a user
	Info struct {
		Userid    string `query:"userid,getgroup;getgroupeq,userid|arr"`
		Username  string `query:"username"`
		Email     string `query:"email"`
		FirstName string `query:"first_name"`
		LastName  string `query:"last_name"`
	}
)

// New creates a new user repository
func New(database db.Database) Repo {
	hasher := hunter2.NewScryptHasher(passHashLen, passSaltLen, hunter2.NewScryptDefaultConfig())
	verifier := hunter2.NewVerifier()
	verifier.RegisterHash(hasher)

	return &repo{
		db:       database,
		hasher:   hasher,
		verifier: verifier,
	}
}

// New creates a new User Model
func (r *repo) New(username, password, email, firstname, lastname string) (*Model, error) {
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
		PassHash:     mHash,
		Email:        email,
		FirstName:    firstname,
		LastName:     lastname,
		CreationTime: time.Now().Round(0).Unix(),
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
	m, err := userModelGetInfoHasUseridOrdUserid(r.db.DB(), userids, true, len(userids), 0)
	if err != nil {
		return nil, governor.NewError("Failed to get user info of userids", http.StatusInternalServerError, err)
	}
	return m, nil
}

// GetByID returns a user model with the given id
func (r *repo) GetByID(userid string) (*Model, error) {
	m, code, err := userModelGetModelEqUserid(r.db.DB(), userid)
	if err != nil {
		if code == 2 {
			return nil, governor.NewError("No user found with that id", http.StatusNotFound, err)
		}
		return nil, governor.NewError("Failed to get user", http.StatusInternalServerError, err)
	}
	return m, nil
}

// GetByUsername returns a user model with the given username
func (r *repo) GetByUsername(username string) (*Model, error) {
	m, code, err := userModelGetModelEqUsername(r.db.DB(), username)
	if err != nil {
		if code == 2 {
			return nil, governor.NewError("No user found with that username", http.StatusNotFound, err)
		}
		return nil, governor.NewError("Failed to get user by username", http.StatusInternalServerError, err)
	}
	return m, nil
}

// GetByEmail returns a user model with the given email
func (r *repo) GetByEmail(email string) (*Model, error) {
	m, code, err := userModelGetModelEqEmail(r.db.DB(), email)
	if err != nil {
		if code == 2 {
			return nil, governor.NewError("No user found with that email", http.StatusNotFound, err)
		}
		return nil, governor.NewError("Failed to get user by email", http.StatusInternalServerError, err)
	}
	return m, nil
}

// Insert inserts the model into the db
func (r *repo) Insert(m *Model) error {
	if code, err := userModelInsert(r.db.DB(), m); err != nil {
		if code == 3 {
			return governor.NewError("User id must be unique", http.StatusBadRequest, err)
		}
		return governor.NewError("Failed to insert user", http.StatusInternalServerError, err)
	}
	return nil
}

// Update updates the model in the db
func (r *repo) Update(m *Model) error {
	if code, err := userModelUpdModelEqUserid(r.db.DB(), m, m.Userid); err != nil {
		if code == 3 {
			return governor.NewError("Username and email must be unique", http.StatusBadRequest, err)
		}
		return governor.NewError("Failed to update user", http.StatusInternalServerError, err)
	}
	return nil
}

// Delete deletes the model in the db
func (r *repo) Delete(m *Model) error {
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
