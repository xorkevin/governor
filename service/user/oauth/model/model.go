package oauthmodel

import (
	"net/http"
	"time"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/hunter2"
)

//go:generate forge model -m Model -t oauthapps -p oauthapp -o model_gen.go Model
//go:generate forge model -m SessionModel -t oauthsessions -p session -o session_model_gen.go SessionModel

const (
	uidSize = 16
	keySize = 32
)

type (
	// Repo is an OAuthApp repository
	Repo interface {
		New(name, url, callbackURL string) (*Model, string, error)
		ValidateKey(key string, m *Model) (bool, error)
		RehashKey(m *Model) (string, error)
		GetByID(clientid string) (*Model, error)
		GetApps(limit, offset int) ([]Model, error)
		Insert(m *Model) error
		Update(m *Model) error
		Delete(m *Model) error
		Setup() error
	}

	repo struct {
		db       db.Database
		hasher   *hunter2.Blake2bHasher
		verifier *hunter2.Verifier
	}

	// Model is the db OAuthApp model
	Model struct {
		ClientID     string `model:"clientid,VARCHAR(31) PRIMARY KEY" query:"clientid,getoneeq,clientid;updeq,clientid;deleq,clientid"`
		Name         string `model:"name,VARCHAR(255) NOT NULL" query:"name"`
		URL          string `model:"url,VARCHAR(255) NOT NULL" query:"url"`
		RedirectURI  string `model:"redirect_uri,VARCHAR(2047) NOT NULL" query:"redirect_uri"`
		Logo         string `model:"logo,VARCHAR(4095)" query:"logo"`
		KeyHash      string `model:"keyhash,VARCHAR(255) NOT NULL" query:"keyhash"`
		Time         int64  `model:"time,BIGINT NOT NULL" query:"time,getgroup"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL" query:"creation_time"`
	}
)

// New creates a new OAuthApp repository
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

func (r *repo) New(name, url, redirectURI string) (*Model, string, error) {
	mUID, err := uid.New(uidSize)
	if err != nil {
		return nil, "", governor.NewError("Failed to create new uid", http.StatusInternalServerError, err)
	}
	clientid := mUID.Base64()

	key, err := uid.New(keySize)
	if err != nil {
		return nil, "", governor.NewError("Failed to create oauth client secret", http.StatusInternalServerError, err)
	}
	keystr := key.Base64()
	hash, err := r.hasher.Hash(keystr)
	if err != nil {
		return nil, "", governor.NewError("Failed to hash oauth client secret", http.StatusInternalServerError, err)
	}

	now := time.Now().Round(0).Unix()
	return &Model{
		ClientID:     clientid,
		Name:         name,
		URL:          url,
		RedirectURI:  redirectURI,
		KeyHash:      hash,
		Time:         now,
		CreationTime: now,
	}, keystr, nil
}

func (r *repo) ValidateKey(key string, m *Model) (bool, error) {
	ok, err := r.verifier.Verify(key, m.KeyHash)
	if err != nil {
		return false, governor.NewError("Failed to verify key", http.StatusInternalServerError, err)
	}
	return ok, nil
}

func (r *repo) RehashKey(m *Model) (string, error) {
	key, err := uid.New(keySize)
	if err != nil {
		return "", governor.NewError("Failed to create oauth client secret", http.StatusInternalServerError, err)
	}
	keystr := key.Base64()
	keyhash, err := r.hasher.Hash(keystr)
	if err != nil {
		return "", governor.NewError("Failed to hash oauth client secret", http.StatusInternalServerError, err)
	}
	now := time.Now().Round(0).Unix()
	m.KeyHash = keyhash
	m.Time = now
	return keystr, nil
}

func (r *repo) GetByID(clientid string) (*Model, error) {
	db, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, code, err := oauthappModelGetModelEqClientID(db, clientid)
	if err != nil {
		if code == 2 {
			return nil, governor.NewError("No OAuth app found with that id", http.StatusNotFound, err)
		}
		return nil, governor.NewError("Failed to get OAuth app config", http.StatusInternalServerError, err)
	}
	return m, nil
}

func (r *repo) GetApps(limit, offset int) ([]Model, error) {
	db, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := oauthappModelGetModelOrdTime(db, false, limit, offset)
	if err != nil {
		return nil, governor.NewError("Failed to get OAuth app configs", http.StatusInternalServerError, err)
	}
	return m, nil
}

func (r *repo) Insert(m *Model) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if code, err := oauthappModelInsert(db, m); err != nil {
		if code == 3 {
			return governor.NewError("clientid must be unique", http.StatusBadRequest, err)
		}
		return governor.NewError("Failed to insert OAuth app config", http.StatusInternalServerError, err)
	}
	return nil
}

func (r *repo) Update(m *Model) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if _, err := oauthappModelUpdModelEqClientID(db, m, m.ClientID); err != nil {
		return governor.NewError("Failed to update OAuth app config", http.StatusInternalServerError, err)
	}
	return nil
}

func (r *repo) Delete(m *Model) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := oauthappModelDelEqClientID(db, m.ClientID); err != nil {
		return governor.NewError("Failed to delete OAuth app config", http.StatusInternalServerError, err)
	}
	return nil
}

func (r *repo) Setup() error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := oauthappModelSetup(db); err != nil {
		return governor.NewError("Failed to setup OAuth app model", http.StatusInternalServerError, err)
	}
	return nil
}

type (
	// SessionRepo is the OAuth session repository
	SessionRepo interface {
		New(userid, clientid, scope string) (*SessionModel, string, error)
		ValidateCode(code string, m *SessionModel) (bool, error)
		RehashCode(m *SessionModel) (string, error)
		GetByID(userid, clientid string) (*SessionModel, error)
		GetUserSessions(userid string, limit, offset int) ([]SessionModel, error)
		Insert(m *SessionModel) error
		Update(m *SessionModel) error
		Delete(userid string, clientids []string) error
		DeleteUserSessions(userid string) error
		Setup() error
	}

	srepo struct {
		db       db.Database
		hasher   *hunter2.Blake2bHasher
		verifier *hunter2.Verifier
	}

	// SessionModel is an connected OAuth session to a user account
	SessionModel struct {
		Userid       string `model:"userid,VARCHAR(31);index" query:"userid,deleq,userid"`
		ClientID     string `model:"clientid,VARCHAR(31), PRIMARY KEY (userid, clientid);index" query:"clientid,getoneeq,userid,clientid;updeq,userid,clientid;deleq,userid,clientid|arr"`
		Scope        string `model:"scope,VARCHAR(4095) NOT NULL" query:"scope"`
		CodeHash     string `model:"codehash,VARCHAR(31) NOT NULL" query:"codehash"`
		Time         int64  `model:"time,BIGINT NOT NULL" query:"time,getgroupeq,userid"`
		LastAuthTime int64  `model:"auth_time,BIGINT NOT NULL" query:"auth_time"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL" query:"creation_time"`
	}
)

// NewSessionRepo creates a new OAuth session repository
func NewSessionRepo(database db.Database) SessionRepo {
	hasher := hunter2.NewBlake2bHasher()
	verifier := hunter2.NewVerifier()
	verifier.RegisterHash(hasher)

	return &srepo{
		db:       database,
		hasher:   hasher,
		verifier: verifier,
	}
}

func (r *srepo) New(userid, clientid, scope string) (*SessionModel, string, error) {
	code, err := uid.New(keySize)
	if err != nil {
		return nil, "", governor.NewError("Failed to create oauth session code", http.StatusInternalServerError, err)
	}
	codestr := code.Base64()
	codehash, err := r.hasher.Hash(codestr)
	if err != nil {
		return nil, "", governor.NewError("Failed to hash oauth session code", http.StatusInternalServerError, err)
	}

	now := time.Now().Round(0).Unix()
	return &SessionModel{
		Userid:       userid,
		ClientID:     clientid,
		Scope:        scope,
		CodeHash:     codehash,
		Time:         now,
		LastAuthTime: now,
		CreationTime: now,
	}, codestr, nil
}

func (r *srepo) ValidateCode(code string, m *SessionModel) (bool, error) {
	ok, err := r.verifier.Verify(code, m.CodeHash)
	if err != nil {
		return false, governor.NewError("Failed to verify code", http.StatusInternalServerError, err)
	}
	return ok, nil
}

func (r *srepo) RehashCode(m *SessionModel) (string, error) {
	code, err := uid.New(keySize)
	if err != nil {
		return "", governor.NewError("Failed to create oauth session code", http.StatusInternalServerError, err)
	}
	codestr := code.Base64()
	codehash, err := r.hasher.Hash(codestr)
	if err != nil {
		return "", governor.NewError("Failed to hash oauth session code", http.StatusInternalServerError, err)
	}
	now := time.Now().Round(0).Unix()
	m.CodeHash = codehash
	m.Time = now
	return codestr, nil
}

func (r *srepo) GetByID(userid, clientid string) (*SessionModel, error) {
	db, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, code, err := sessionModelGetSessionModelEqUseridEqClientID(db, userid, clientid)
	if err != nil {
		if code == 2 {
			return nil, governor.NewError("No OAuth session found with that id", http.StatusNotFound, err)
		}
		return nil, governor.NewError("Failed to get OAuth session", http.StatusInternalServerError, err)
	}
	return m, nil
}

func (r *srepo) GetUserSessions(userid string, limit, offset int) ([]SessionModel, error) {
	db, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := sessionModelGetSessionModelEqUseridOrdTime(db, userid, false, limit, offset)
	if err != nil {
		return nil, governor.NewError("Failed to get OAuth sessions", http.StatusInternalServerError, err)
	}
	return m, nil
}

func (r *srepo) Insert(m *SessionModel) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if code, err := sessionModelInsert(db, m); err != nil {
		if code == 3 {
			return governor.NewError("OAuth session already exists", http.StatusBadRequest, err)
		}
		return governor.NewError("Failed to insert OAuth session", http.StatusInternalServerError, err)
	}
	return nil
}

func (r *srepo) Update(m *SessionModel) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if _, err := sessionModelUpdSessionModelEqUseridEqClientID(db, m, m.Userid, m.ClientID); err != nil {
		return governor.NewError("Failed to update OAuth session", http.StatusInternalServerError, err)
	}
	return nil
}

func (r *srepo) Delete(userid string, clientids []string) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := sessionModelDelEqUseridHasClientID(db, userid, clientids); err != nil {
		return governor.NewError("Failed to delete OAuth session", http.StatusInternalServerError, err)
	}
	return nil
}

func (r *srepo) DeleteUserSessions(userid string) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := sessionModelDelEqUserid(db, userid); err != nil {
		return governor.NewError("Failed to delete OAuth sessions", http.StatusInternalServerError, err)
	}
	return nil
}

func (r *srepo) Setup() error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := sessionModelSetup(db); err != nil {
		return governor.NewError("Failed to setup OAuth session model", http.StatusInternalServerError, err)
	}
	return nil
}
