package sessionmodel

import (
	"net/http"
	"time"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/hunter2"
)

//go:generate forge model -m Model -t usersessions -p session -o model_gen.go Model qID

const (
	uidSize = 8
	keySize = 32
)

type (
	Repo interface {
		New(userid, ipaddr, useragent string) (*Model, string, error)
		ValidateKey(key string, m *Model) (bool, error)
		RehashKey(m *Model) (string, error)
		GetByID(sessionid string) (*Model, error)
		GetUserSessions(userid string, limit, offset int) ([]Model, error)
		GetUserSessionIDs(userid string, limit, offset int) ([]string, error)
		Insert(m *Model) error
		Update(m *Model) error
		Delete(m *Model) error
		DeleteSessions(sessionids []string) error
		DeleteUserSessions(userid string) error
		Setup() error
	}

	repo struct {
		db       db.Database
		hasher   *hunter2.Blake2bHasher
		verifier *hunter2.Verifier
	}

	// Model is the db User session model
	Model struct {
		SessionID string `model:"sessionid,VARCHAR(63) PRIMARY KEY" query:"sessionid,getoneeq,sessionid;updeq,sessionid;deleq,sessionid;deleq,sessionid|arr"`
		Userid    string `model:"userid,VARCHAR(31) NOT NULL;index" query:"userid,deleq,userid"`
		KeyHash   string `model:"keyhash,VARCHAR(127) NOT NULL" query:"keyhash"`
		Time      int64  `model:"time,BIGINT NOT NULL" query:"time,getgroupeq,userid"`
		IPAddr    string `model:"ipaddr,VARCHAR(63)" query:"ipaddr"`
		UserAgent string `model:"user_agent,VARCHAR(1023)" query:"user_agent"`
	}

	qID struct {
		SessionID string `query:"sessionid,getgroupeq,userid"`
	}
)

// New creates a new user session repository
func New(database db.Database) Repo {
	hasher := hunter2.NewBlake2bHasher()
	verifier := hunter2.NewVerifier()
	verifier.RegisterHash(hasher)

	return &repo{
		db:       database,
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
	now := time.Now().Round(0).Unix()
	return &Model{
		SessionID: userid + "|" + sid.Base64(),
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
	now := time.Now().Round(0).Unix()
	m.KeyHash = hash
	m.Time = now
	return keystr, nil
}

// GetByID returns a user session model with the given id
func (r *repo) GetByID(sessionID string) (*Model, error) {
	db, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, code, err := sessionModelGetModelEqSessionID(db, sessionID)
	if err != nil {
		if code == 2 {
			return nil, governor.NewError("No session found with that id", http.StatusNotFound, err)
		}
		return nil, governor.NewError("Failed to get session", http.StatusInternalServerError, err)
	}
	return m, nil
}

// GetUserSessions returns all the sessions of a user
func (r *repo) GetUserSessions(userid string, limit, offset int) ([]Model, error) {
	db, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := sessionModelGetModelEqUseridOrdTime(db, userid, false, limit, offset)
	if err != nil {
		return nil, governor.NewError("Failed to get user sessions", http.StatusInternalServerError, err)
	}
	return m, nil
}

// GetUserSessionIDs returns all the session ids of a user
func (r *repo) GetUserSessionIDs(userid string, limit, offset int) ([]string, error) {
	db, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := sessionModelGetqIDEqUseridOrdSessionID(db, userid, true, limit, offset)
	if err != nil {
		return nil, governor.NewError("Failed to get user session ids", http.StatusInternalServerError, err)
	}
	res := make([]string, 0, len(m))
	for _, i := range m {
		res = append(res, i.SessionID)
	}
	return res, nil
}

// Insert inserts the model into the db
func (r *repo) Insert(m *Model) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if code, err := sessionModelInsert(db, m); err != nil {
		if code == 3 {
			return governor.NewError("Session id must be unique", http.StatusBadRequest, err)
		}
		return governor.NewError("Failed to insert session", http.StatusInternalServerError, err)
	}
	return nil
}

// Update updates the model in the db
func (r *repo) Update(m *Model) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if _, err := sessionModelUpdModelEqSessionID(db, m, m.SessionID); err != nil {
		return governor.NewError("Failed to update session", http.StatusInternalServerError, err)
	}
	return nil
}

// Delete deletes the model in the db
func (r *repo) Delete(m *Model) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := sessionModelDelEqSessionID(db, m.SessionID); err != nil {
		return governor.NewError("Failed to delete session", http.StatusInternalServerError, err)
	}
	return nil
}

// DeleteSessions deletes the sessions in the set of session ids
func (r *repo) DeleteSessions(sessionids []string) error {
	if len(sessionids) == 0 {
		return nil
	}
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := sessionModelDelHasSessionID(db, sessionids); err != nil {
		return governor.NewError("Failed to delete sessions", http.StatusInternalServerError, err)
	}
	return nil
}

// DeleteUserSessions deletes all the sessions of a user
func (r *repo) DeleteUserSessions(userid string) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := sessionModelDelEqUserid(db, userid); err != nil {
		return governor.NewError("Failed to delete sessions", http.StatusInternalServerError, err)
	}
	return nil
}

// Setup creates a new User session table
func (r *repo) Setup() error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := sessionModelSetup(db); err != nil {
		return governor.NewError("Failed to setup user session model", http.StatusInternalServerError, err)
	}
	return nil
}
