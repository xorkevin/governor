package connectionmodel

import (
	"net/http"
	"time"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/hunter2"
)

//go:generate forge model -m Model -t oauthconnections -p connection -o model_gen.go Model

const (
	keySize = 32
)

type (
	// Repo is the OAuth account connection repository
	Repo interface {
		New(userid, clientid, scope string) (*Model, string, error)
		ValidateCode(code string, m *Model) (bool, error)
		RehashCode(m *Model) (string, error)
		GetByID(userid, clientid string) (*Model, error)
		GetUserConnections(userid string, limit, offset int) ([]Model, error)
		Insert(m *Model) error
		Update(m *Model) error
		Delete(userid string, clientids []string) error
		DeleteUserConnections(userid string) error
		Setup() error
	}

	repo struct {
		db       db.Database
		hasher   *hunter2.Blake2bHasher
		verifier *hunter2.Verifier
	}

	// Model is an connected OAuth app to a user account
	Model struct {
		Userid       string `model:"userid,VARCHAR(31);index" query:"userid,deleq,userid"`
		ClientID     string `model:"clientid,VARCHAR(31), PRIMARY KEY (userid, clientid);index" query:"clientid,getoneeq,userid,clientid;updeq,userid,clientid;deleq,userid,clientid|arr"`
		Scope        string `model:"scope,VARCHAR(4095) NOT NULL" query:"scope"`
		CodeHash     string `model:"codehash,VARCHAR(31) NOT NULL" query:"codehash"`
		Time         int64  `model:"time,BIGINT NOT NULL;index" query:"time,getgroupeq,userid"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL" query:"creation_time"`
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

func NewInCtx(inj governor.Injector) {
	SetCtxRepo(inj, NewCtx(inj))
}

func NewCtx(inj governor.Injector) Repo {
	dbService := db.GetCtxDB(inj)
	return New(dbService)
}

// New creates a new OAuth connection repository
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

func (r *repo) New(userid, clientid, scope string) (*Model, string, error) {
	code, err := uid.New(keySize)
	if err != nil {
		return nil, "", governor.NewError("Failed to create OAuth authorization code", http.StatusInternalServerError, err)
	}
	codestr := code.Base64()
	codehash, err := r.hasher.Hash(codestr)
	if err != nil {
		return nil, "", governor.NewError("Failed to hash OAuth authorization code", http.StatusInternalServerError, err)
	}

	now := time.Now().Round(0).Unix()
	return &Model{
		Userid:       userid,
		ClientID:     clientid,
		Scope:        scope,
		CodeHash:     codehash,
		Time:         now,
		CreationTime: now,
	}, codestr, nil
}

func (r *repo) ValidateCode(code string, m *Model) (bool, error) {
	ok, err := r.verifier.Verify(code, m.CodeHash)
	if err != nil {
		return false, governor.NewError("Failed to verify code", http.StatusInternalServerError, err)
	}
	return ok, nil
}

func (r *repo) RehashCode(m *Model) (string, error) {
	code, err := uid.New(keySize)
	if err != nil {
		return "", governor.NewError("Failed to create OAuth authorization code", http.StatusInternalServerError, err)
	}
	codestr := code.Base64()
	codehash, err := r.hasher.Hash(codestr)
	if err != nil {
		return "", governor.NewError("Failed to hash OAuth authorization code", http.StatusInternalServerError, err)
	}
	now := time.Now().Round(0).Unix()
	m.CodeHash = codehash
	m.Time = now
	return codestr, nil
}

func (r *repo) GetByID(userid, clientid string) (*Model, error) {
	db, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, code, err := connectionModelGetModelEqUseridEqClientID(db, userid, clientid)
	if err != nil {
		if code == 2 {
			return nil, governor.NewError("No connected OAuth app found with that id", http.StatusNotFound, err)
		}
		return nil, governor.NewError("Failed to get connected OAuth app", http.StatusInternalServerError, err)
	}
	return m, nil
}

func (r *repo) GetUserConnections(userid string, limit, offset int) ([]Model, error) {
	db, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := connectionModelGetModelEqUseridOrdTime(db, userid, false, limit, offset)
	if err != nil {
		return nil, governor.NewError("Failed to get connected OAuth apps", http.StatusInternalServerError, err)
	}
	return m, nil
}

func (r *repo) Insert(m *Model) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if code, err := connectionModelInsert(db, m); err != nil {
		if code == 3 {
			return governor.NewError("OAuth app already connected", http.StatusBadRequest, err)
		}
		return governor.NewError("Failed to add connected OAuth app", http.StatusInternalServerError, err)
	}
	return nil
}

func (r *repo) Update(m *Model) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if _, err := connectionModelUpdModelEqUseridEqClientID(db, m, m.Userid, m.ClientID); err != nil {
		return governor.NewError("Failed to update connected OAuth app", http.StatusInternalServerError, err)
	}
	return nil
}

func (r *repo) Delete(userid string, clientids []string) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := connectionModelDelEqUseridHasClientID(db, userid, clientids); err != nil {
		return governor.NewError("Failed to delete connected OAuth app", http.StatusInternalServerError, err)
	}
	return nil
}

func (r *repo) DeleteUserConnections(userid string) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := connectionModelDelEqUserid(db, userid); err != nil {
		return governor.NewError("Failed to delete connected OAuth apps", http.StatusInternalServerError, err)
	}
	return nil
}

func (r *repo) Setup() error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := connectionModelSetup(db); err != nil {
		return governor.NewError("Failed to setup OAuth connection model", http.StatusInternalServerError, err)
	}
	return nil
}
