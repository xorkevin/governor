package sessionmodel

import (
	"database/sql"
	"net/http"
	"time"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/hunter2"
)

//go:generate forge model -m Model -t usersessions -p session -o model_gen.go

const (
	uidSize = 4
	keySize = 32
)

type (
	Repo interface {
		New(userid, ipaddr, useragent string) (*Model, string, error)
		ValidateKey(key string, m *Model) (bool, error)
		RehashKey(m *Model) (string, error)
		GetByID(sessionid string) (*Model, error)
		Insert(m *Model) error
		Update(m *Model) error
		Delete(m *Model) error
		Setup() error
	}

	repo struct {
		db       *sql.DB
		hasher   *hunter2.Blake2bHasher
		verifier *hunter2.Verifier
	}

	// Model is the db User session model
	Model struct {
		SessionID string `model:"sessionid,VARCHAR(31) PRIMARY KEY"`
		Userid    string `model:"userid,VARCHAR(31) NOT NULL"`
		KeyHash   string `model:"keyhash,VARCHAR(127) NOT NULL"`
		Time      int64  `model:"time,BIGINT NOT NULL"`
		IPAddr    string `model:"ipaddr,VARCHAR(63)"`
		UserAgent string `model:"user_agent,VARCHAR(1023)"`
	}
)

// New creates a new user session repository
func New(conf governor.Config, l governor.Logger, database db.Database) Repo {
	l.Info("initialize user role model", nil)
	hasher := hunter2.NewBlake2bHasher()
	verifier := hunter2.NewVerifier()
	verifier.RegisterHash(hasher)

	return &repo{
		db:       database.DB(),
		hasher:   hasher,
		verifier: verifier,
	}
}

// New creates a new User session Model
func (r *repo) New(userid, ipaddr, useragent string) (*Model, string, error) {
	sid, err := uid.New(uidSize)
	if err != nil {
		return nil, "", governor.NewError("Failed to create new session id", http.StatusInternalServerError, err)
	}
	key, err := uid.New(keySize)
	if err != nil {
		return nil, "", governor.NewError("Failed to create new session key", http.StatusInternalServerError, err)
	}
	keystr := key.Base64()
	hash, err := r.hasher.Hash(keystr)
	if err != nil {
		return nil, "", governor.NewError("Failed to hash session key", http.StatusInternalServerError, err)
	}
	now := time.Now().Unix()
	return &Model{
		SessionID: userid + sid.Base64(),
		Userid:    userid,
		KeyHash:   hash,
		Time:      now,
		IPAddr:    ipaddr,
		UserAgent: useragent,
	}, keystr, nil
}

// ValidateKey validates the key against a hash
func (r *repo) ValidateKey(key string, m *Model) (bool, error) {
	ok, err := r.verifier.Verify(key, m.KeyHash)
	if err != nil {
		return false, governor.NewError("Failed to verify key", http.StatusInternalServerError, err)
	}
	return ok, nil
}

// RehashKey generates a new key and saves its hash
func (r *repo) RehashKey(m *Model) (string, error) {
	key, err := uid.New(keySize)
	if err != nil {
		return "", governor.NewError("Failed to create new session key", http.StatusInternalServerError, err)
	}
	keystr := key.Base64()
	hash, err := r.hasher.Hash(keystr)
	if err != nil {
		return "", governor.NewError("Failed to hash session key", http.StatusInternalServerError, err)
	}
	now := time.Now().Unix()
	m.KeyHash = hash
	m.Time = now
	return keystr, nil
}

// GetByID returns a user session model with the given id
func (r *repo) GetByID(sessionID string) (*Model, error) {
	m, code, err := sessionModelGet(r.db, sessionID)
	if err != nil {
		if code == 2 {
			return nil, governor.NewError("No session found with that id", http.StatusNotFound, err)
		}
		return nil, governor.NewError("Failed to get session", http.StatusInternalServerError, err)
	}
	return m, nil
}

// Insert inserts the model into the db
func (r *repo) Insert(m *Model) error {
	if code, err := sessionModelInsert(r.db, m); err != nil {
		if code == 3 {
			return governor.NewError("Session id must be unique", http.StatusBadRequest, err)
		}
		return governor.NewError("Failed to insert session", http.StatusInternalServerError, err)
	}
	return nil
}

// Update updates the model in the db
func (r *repo) Update(m *Model) error {
	if err := sessionModelUpdate(r.db, m); err != nil {
		return governor.NewError("Failed to update session", http.StatusInternalServerError, err)
	}
	return nil
}

// Delete deletes the model in the db
func (r *repo) Delete(m *Model) error {
	if err := sessionModelDelete(r.db, m); err != nil {
		return governor.NewError("Failed to delete session", http.StatusInternalServerError, err)
	}
	return nil
}

// Setup creates a new User session table
func (r *repo) Setup() error {
	if err := sessionModelSetup(r.db); err != nil {
		return governor.NewError("Failed to setup user session model", http.StatusInternalServerError, err)
	}
	return nil
}
