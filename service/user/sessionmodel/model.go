package sessionmodel

import (
	"context"
	"errors"
	"strings"
	"time"

	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/hunter2/h2hash"
	"xorkevin.dev/hunter2/h2hash/blake2b"
	"xorkevin.dev/kerrors"
)

//go:generate forge model

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
		GetByID(ctx context.Context, sessionid string) (*Model, error)
		GetUserSessions(ctx context.Context, userid string, limit, offset int) ([]Model, error)
		Insert(ctx context.Context, m *Model) error
		Update(ctx context.Context, m *Model) error
		Delete(ctx context.Context, m *Model) error
		DeleteSessions(ctx context.Context, sessionids []string) error
		DeleteUserSessions(ctx context.Context, userid string) error
		Setup(ctx context.Context) error
	}

	repo struct {
		table    *sessionModelTable
		db       db.Database
		hasher   h2hash.Hasher
		verifier *h2hash.Verifier
	}

	// Model is the db User session model
	//forge:model session
	//forge:model:query session
	Model struct {
		SessionID string `model:"sessionid,VARCHAR(63) PRIMARY KEY"`
		Userid    string `model:"userid,VARCHAR(31) NOT NULL"`
		KeyHash   string `model:"keyhash,VARCHAR(127) NOT NULL"`
		Time      int64  `model:"time,BIGINT NOT NULL"`
		AuthTime  int64  `model:"auth_time,BIGINT NOT NULL"`
		IPAddr    string `model:"ipaddr,VARCHAR(63)"`
		UserAgent string `model:"user_agent,VARCHAR(1023)"`
	}
)

// New creates a new user session repository
func New(database db.Database, table string) Repo {
	hasher := blake2b.New(blake2b.Config{})
	verifier := h2hash.NewVerifier()
	verifier.Register(hasher)

	return &repo{
		table: &sessionModelTable{
			TableName: table,
		},
		db:       database,
		hasher:   hasher,
		verifier: verifier,
	}
}

// New creates a new User session Model
func (r *repo) New(userid, ipaddr, useragent string) (*Model, string, error) {
	sid, err := uid.New(uidSize)
	if err != nil {
		return nil, "", kerrors.WithMsg(err, "Failed to create new session id")
	}
	keybytes, err := uid.New(keySize)
	if err != nil {
		return nil, "", kerrors.WithMsg(err, "Failed to create new session key")
	}
	key := keybytes.Base64()
	hash, err := r.hasher.Hash([]byte(key))
	if err != nil {
		return nil, "", kerrors.WithMsg(err, "Failed to hash session key")
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
	}, key, nil
}

// ParseIDUserid gets the userid from a keyid
func ParseIDUserid(sessionID string) (string, error) {
	userid, _, ok := strings.Cut(sessionID, keySeparator)
	if !ok {
		return "", kerrors.WithMsg(nil, "Invalid session id")
	}
	return userid, nil
}

// ValidateKey validates the key against a hash
func (r *repo) ValidateKey(key string, m *Model) (bool, error) {
	ok, err := r.verifier.Verify([]byte(key), m.KeyHash)
	if err != nil {
		return false, kerrors.WithMsg(err, "Failed to verify key")
	}
	return ok, nil
}

// RehashKey generates a new key and saves its hash
func (r *repo) RehashKey(m *Model) (string, error) {
	keybytes, err := uid.New(keySize)
	if err != nil {
		return "", kerrors.WithMsg(err, "Failed to create new session key")
	}
	key := keybytes.Base64()
	hash, err := r.hasher.Hash([]byte(key))
	if err != nil {
		return "", kerrors.WithMsg(err, "Failed to hash session key")
	}
	now := time.Now().Round(0).Unix()
	m.KeyHash = hash
	m.Time = now
	return key, nil
}

// GetByID returns a user session model with the given id
func (r *repo) GetByID(ctx context.Context, sessionID string) (*Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelByID(ctx, d, sessionID)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get session")
	}
	return m, nil
}

// GetUserSessions returns all the sessions of a user
func (r *repo) GetUserSessions(ctx context.Context, userid string, limit, offset int) ([]Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelByUserid(ctx, d, userid, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get user sessions")
	}
	return m, nil
}

// Insert inserts the model into the db
func (r *repo) Insert(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.Insert(ctx, d, m); err != nil {
		return kerrors.WithMsg(err, "Failed to insert session")
	}
	return nil
}

// Update updates the model in the db
func (r *repo) Update(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.UpdModelByID(ctx, d, m, m.SessionID); err != nil {
		return kerrors.WithMsg(err, "Failed to update session")
	}
	return nil
}

// Delete deletes the model in the db
func (r *repo) Delete(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelByID(ctx, d, m.SessionID); err != nil {
		return kerrors.WithMsg(err, "Failed to delete session")
	}
	return nil
}

// DeleteSessions deletes the sessions in the set of session ids
func (r *repo) DeleteSessions(ctx context.Context, sessionids []string) error {
	if len(sessionids) == 0 {
		return nil
	}
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelByIDs(ctx, d, sessionids); err != nil {
		return kerrors.WithMsg(err, "Failed to delete sessions")
	}
	return nil
}

// DeleteUserSessions deletes all the sessions of a user
func (r *repo) DeleteUserSessions(ctx context.Context, userid string) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelByUserid(ctx, d, userid); err != nil {
		return kerrors.WithMsg(err, "Failed to delete sessions")
	}
	return nil
}

// Setup creates a new User session table
func (r *repo) Setup(ctx context.Context) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.Setup(ctx, d); err != nil {
		err = kerrors.WithMsg(err, "Failed to setup user session model")
		if !errors.Is(err, db.ErrAuthz) {
			return err
		}
	}
	return nil
}
