package sessionmodel

import (
	"database/sql"
	"net/http"
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
		New(userid string) (*Model, string, error)
		//Setup() error
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
	}
)

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
func (r *repo) New(userid string) (*Model, string, error) {
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
	return &Model{
		SessionID: userid + sid.Base64(),
		Userid:    userid,
		KeyHash:   hash,
	}, keystr, nil
}
