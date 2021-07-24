package model

import (
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

const (
	keySeparator = "."
)

type (
	// Repo is a user session repository
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
		hasher   hunter2.Hasher
		verifier *hunter2.Verifier
	}

	// Model is the db User session model
	Model struct {
		SessionID string `model:"sessionid,VARCHAR(63) PRIMARY KEY" query:"sessionid,getoneeq,sessionid;updeq,sessionid;deleq,sessionid;deleq,sessionid|arr"`
		Userid    string `model:"userid,VARCHAR(31) NOT NULL;index" query:"userid,deleq,userid"`
		KeyHash   string `model:"keyhash,VARCHAR(127) NOT NULL" query:"keyhash"`
		Time      int64  `model:"time,BIGINT NOT NULL;index" query:"time,getgroupeq,userid"`
		AuthTime  int64  `model:"auth_time,BIGINT NOT NULL" query:"auth_time"`
		IPAddr    string `model:"ipaddr,VARCHAR(63)" query:"ipaddr"`
		UserAgent string `model:"user_agent,VARCHAR(1023)" query:"user_agent"`
	}

	qID struct {
		SessionID string `query:"sessionid,getgroupeq,userid"`
	}

	ctxKeyRepo struct{}
)

// GetCtxRepo returns a Repo from the context
func GetCtxRepo(inj governor.Injector) Repo {
	v := inj.Get(ctxKeyRepo{})
	if v == nil {
		return nil
	}
	return v.(Repo)
}

// SetCtxRepo sets a Repo in the context
func SetCtxRepo(inj governor.Injector, r Repo) {
	inj.Set(ctxKeyRepo{}, r)
}

// NewInCtx creates a new session repo from a context and sets it in the context
func NewInCtx(inj governor.Injector) {
	SetCtxRepo(inj, NewCtx(inj))
}

// NewCtx creates a new session repo from a context
func NewCtx(inj governor.Injector) Repo {
	dbService := db.GetCtxDB(inj)
	return New(dbService)
}

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
		return nil, "", governor.ErrWithMsg(err, "Failed to create new session id")
	}
	key, err := uid.New(keySize)
	if err != nil {
		return nil, "", governor.ErrWithMsg(err, "Failed to create new session key")
	}
	keystr := key.Base64()
	hash, err := r.hasher.Hash(keystr)
	if err != nil {
		return nil, "", governor.ErrWithMsg(err, "Failed to hash session key")
	}
	now := time.Now().Round(0).Unix()
	return &Model{
		SessionID: userid + keySeparator + sid.Base64(),
		Userid:    userid,
		KeyHash:   hash,
		Time:      now,
		AuthTime:  now,
		IPAddr:    ipaddr,
		UserAgent: useragent,
	}, keystr, nil
}

// ValidateKey validates the key against a hash
func (r *repo) ValidateKey(key string, m *Model) (bool, error) {
	ok, err := r.verifier.Verify(key, m.KeyHash)
	if err != nil {
		return false, governor.ErrWithMsg(err, "Failed to verify key")
	}
	return ok, nil
}

// RehashKey generates a new key and saves its hash
func (r *repo) RehashKey(m *Model) (string, error) {
	key, err := uid.New(keySize)
	if err != nil {
		return "", governor.ErrWithMsg(err, "Failed to create new session key")
	}
	keystr := key.Base64()
	hash, err := r.hasher.Hash(keystr)
	if err != nil {
		return "", governor.ErrWithMsg(err, "Failed to hash session key")
	}
	now := time.Now().Round(0).Unix()
	m.KeyHash = hash
	m.Time = now
	return keystr, nil
}

// GetByID returns a user session model with the given id
func (r *repo) GetByID(sessionID string) (*Model, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, code, err := sessionModelGetModelEqSessionID(d, sessionID)
	if err != nil {
		if code == 2 {
			return nil, governor.ErrWithKind(err, db.ErrNotFound{}, "No session found with that id")
		}
		return nil, governor.ErrWithMsg(err, "Failed to get session")
	}
	return m, nil
}

// GetUserSessions returns all the sessions of a user
func (r *repo) GetUserSessions(userid string, limit, offset int) ([]Model, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := sessionModelGetModelEqUseridOrdTime(d, userid, false, limit, offset)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get user sessions")
	}
	return m, nil
}

// GetUserSessionIDs returns all the session ids of a user
func (r *repo) GetUserSessionIDs(userid string, limit, offset int) ([]string, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := sessionModelGetqIDEqUseridOrdSessionID(d, userid, true, limit, offset)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get user session ids")
	}
	res := make([]string, 0, len(m))
	for _, i := range m {
		res = append(res, i.SessionID)
	}
	return res, nil
}

// Insert inserts the model into the db
func (r *repo) Insert(m *Model) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if code, err := sessionModelInsert(d, m); err != nil {
		if code == 3 {
			return governor.ErrWithKind(err, db.ErrUnique{}, "Session id must be unique")
		}
		return governor.ErrWithMsg(err, "Failed to insert session")
	}
	return nil
}

// Update updates the model in the db
func (r *repo) Update(m *Model) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if _, err := sessionModelUpdModelEqSessionID(d, m, m.SessionID); err != nil {
		return governor.ErrWithMsg(err, "Failed to update session")
	}
	return nil
}

// Delete deletes the model in the db
func (r *repo) Delete(m *Model) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := sessionModelDelEqSessionID(d, m.SessionID); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete session")
	}
	return nil
}

// DeleteSessions deletes the sessions in the set of session ids
func (r *repo) DeleteSessions(sessionids []string) error {
	if len(sessionids) == 0 {
		return nil
	}
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := sessionModelDelHasSessionID(d, sessionids); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete sessions")
	}
	return nil
}

// DeleteUserSessions deletes all the sessions of a user
func (r *repo) DeleteUserSessions(userid string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := sessionModelDelEqUserid(d, userid); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete sessions")
	}
	return nil
}

// Setup creates a new User session table
func (r *repo) Setup() error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if code, err := sessionModelSetup(d); err != nil {
		if code != 5 {
			return governor.ErrWithMsg(err, "Failed to setup user session model")
		}
	}
	return nil
}
