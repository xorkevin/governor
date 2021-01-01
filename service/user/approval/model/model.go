package approvalmodel

import (
	"net/http"
	"time"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/user/model"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/hunter2"
)

//go:generate forge model -m Model -t userapprovals -p approval -o model_gen.go Model

const (
	keySize = 16
)

type (
	Repo interface {
		New(m *usermodel.Model) *Model
		ToUserModel(m *Model) *usermodel.Model
		ValidateCode(code string, m *Model) (bool, error)
		RehashCode(m *Model) (string, error)
		GetByID(userid string) (*Model, error)
		GetGroup(limit, offset int) ([]Model, error)
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

	Model struct {
		Userid       string `model:"userid,VARCHAR(31) PRIMARY KEY" query:"userid,getoneeq,userid;updeq,userid;deleq,userid"`
		Username     string `model:"username,VARCHAR(255) NOT NULL" query:"username"`
		PassHash     string `model:"pass_hash,VARCHAR(255) NOT NULL" query:"pass_hash"`
		Email        string `model:"email,VARCHAR(255) NOT NULL" query:"email"`
		FirstName    string `model:"first_name,VARCHAR(255) NOT NULL" query:"first_name"`
		LastName     string `model:"last_name,VARCHAR(255) NOT NULL" query:"last_name"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL;index" query:"creation_time,getgroup"`
		Approved     bool   `model:"approved,BOOL NOT NULL" query:"approved"`
		CodeHash     string `model:"code_hash,VARCHAR(255) NOT NULL" query:"code_hash"`
		CodeTime     int64  `model:"code_time,BIGINT NOT NULL" query:"code_time"`
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

func (r *repo) New(m *usermodel.Model) *Model {
	return &Model{
		Userid:       m.Userid,
		Username:     m.Username,
		PassHash:     m.PassHash,
		Email:        m.Email,
		FirstName:    m.FirstName,
		LastName:     m.LastName,
		CreationTime: m.CreationTime,
		Approved:     false,
		CodeHash:     "",
		CodeTime:     0,
	}
}

func (r *repo) ToUserModel(m *Model) *usermodel.Model {
	return &usermodel.Model{
		Userid:       m.Userid,
		Username:     m.Username,
		PassHash:     m.PassHash,
		Email:        m.Email,
		FirstName:    m.FirstName,
		LastName:     m.LastName,
		CreationTime: m.CreationTime,
	}
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
		return "", governor.NewError("Failed to create new user code", http.StatusInternalServerError, err)
	}
	codestr := code.Base64()
	codehash, err := r.hasher.Hash(codestr)
	if err != nil {
		return "", governor.NewError("Failed to hash new user code", http.StatusInternalServerError, err)
	}
	now := time.Now().Round(0).Unix()
	m.Approved = true
	m.CodeHash = codehash
	m.CodeTime = now
	return codestr, nil
}

func (r *repo) GetByID(userid string) (*Model, error) {
	db, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, code, err := approvalModelGetModelEqUserid(db, userid)
	if err != nil {
		if code == 2 {
			return nil, governor.NewError("No user found with that id", http.StatusNotFound, err)
		}
		return nil, governor.NewError("Failed to get user", http.StatusInternalServerError, err)
	}
	return m, nil
}

func (r *repo) GetGroup(limit, offset int) ([]Model, error) {
	db, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := approvalModelGetModelOrdCreationTime(db, true, limit, offset)
	if err != nil {
		return nil, governor.NewError("Failed to get user approvals", http.StatusInternalServerError, err)
	}
	return m, nil
}

func (r *repo) Insert(m *Model) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if code, err := approvalModelInsert(db, m); err != nil {
		if code == 3 {
			return governor.NewError("Userid must be unique", http.StatusBadRequest, err)
		}
		return governor.NewError("Failed to insert user", http.StatusInternalServerError, err)
	}
	return nil
}

func (r *repo) Update(m *Model) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if _, err := approvalModelUpdModelEqUserid(db, m, m.Userid); err != nil {
		return governor.NewError("Failed to update user approval", http.StatusInternalServerError, err)
	}
	return nil
}

func (r *repo) Delete(m *Model) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := approvalModelDelEqUserid(db, m.Userid); err != nil {
		return governor.NewError("Failed to delete user approval", http.StatusInternalServerError, err)
	}
	return nil
}

func (r *repo) Setup() error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := approvalModelSetup(db); err != nil {
		return governor.NewError("Failed to setup user approval model", http.StatusInternalServerError, err)
	}
	return nil
}
