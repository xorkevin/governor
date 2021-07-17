package model

import (
	"crypto/hmac"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/hunter2"
)

//go:generate forge model -m Model -t users -p user -o model_gen.go Model Info

const (
	uidSize       = 16
	passSaltLen   = 32
	passHashLen   = 32
	totpSecretLen = 32
	totpBackupLen = 18
)

type (
	// Repo is a user repository
	Repo interface {
		New(username, password, email, firstname, lastname string) (*Model, error)
		ValidatePass(password string, m *Model) (bool, error)
		RehashPass(m *Model, password string) error
		ValidateOTPCode(decrypter *hunter2.Decrypter, m *Model, code string) (bool, error)
		ValidateOTPBackup(decrypter *hunter2.Decrypter, m *Model, backup string) (bool, error)
		GenerateOTPSecret(cipher hunter2.Cipher, m *Model, issuer string, alg string, digits int) (string, string, error)
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
		hasher   hunter2.Hasher
		verifier *hunter2.Verifier
	}

	// Model is the db User model
	Model struct {
		Userid           string `model:"userid,VARCHAR(31) PRIMARY KEY" query:"userid,getoneeq,userid;updeq,userid;deleq,userid"`
		Username         string `model:"username,VARCHAR(255) NOT NULL UNIQUE" query:"username,getoneeq,username"`
		PassHash         string `model:"pass_hash,VARCHAR(255) NOT NULL" query:"pass_hash"`
		OTPEnabled       bool   `model:"otp_enabled,BOOLEAN NOT NULL" query:"otp_enabled"`
		OTPSecret        string `model:"otp_secret,VARCHAR(255) NOT NULL" query:"otp_secret"`
		OTPBackup        string `model:"otp_backup,VARCHAR(255) NOT NULL" query:"otp_backup"`
		Email            string `model:"email,VARCHAR(255) NOT NULL UNIQUE" query:"email,getoneeq,email"`
		FirstName        string `model:"first_name,VARCHAR(255) NOT NULL" query:"first_name"`
		LastName         string `model:"last_name,VARCHAR(255) NOT NULL" query:"last_name"`
		CreationTime     int64  `model:"creation_time,BIGINT NOT NULL" query:"creation_time"`
		FailedLoginTime  int64  `model:"failed_login_time,BIGINT NOT NULL" query:"failed_login_time"`
		FailedLoginCount int    `model:"failed_login_count,INT NOT NULL" query:"failed_login_count"`
	}

	// Info is the metadata of a user
	Info struct {
		Userid    string `query:"userid,getgroup;getgroupeq,userid|arr"`
		Username  string `query:"username"`
		Email     string `query:"email"`
		FirstName string `query:"first_name"`
		LastName  string `query:"last_name"`
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

// NewInCtx creates a new user repo from a context and sets it in the context
func NewInCtx(inj governor.Injector) {
	SetCtxRepo(inj, NewCtx(inj))
}

// NewCtx creates a new user repo from a context
func NewCtx(inj governor.Injector) Repo {
	dbService := db.GetCtxDB(inj)
	return New(dbService)
}

// New creates a new user repository
func New(database db.Database) Repo {
	hasher := hunter2.NewScryptHasher(passHashLen, passSaltLen, hunter2.DefaultScryptConfig)
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
		return nil, governor.ErrWithMsg(err, "Failed to create new uid")
	}

	mHash, err := r.hasher.Hash(password)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to hash password")
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

// ValidatePass validates the password against a hash
func (r *repo) ValidatePass(password string, m *Model) (bool, error) {
	ok, err := r.verifier.Verify(password, m.PassHash)
	if err != nil {
		return false, governor.ErrWithMsg(err, "Failed to verify password")
	}
	return ok, nil
}

// RehashPass updates the password with a new hash
func (r *repo) RehashPass(m *Model, password string) error {
	mHash, err := r.hasher.Hash(password)
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to rehash password")
	}
	m.PassHash = mHash
	return nil
}

// ValidateOTPCode validates an otp code
func (r *repo) ValidateOTPCode(decrypter *hunter2.Decrypter, m *Model, code string) (bool, error) {
	params, err := decrypter.Decrypt(m.OTPSecret)
	if err != nil {
		return false, governor.ErrWithMsg(err, "Failed to decrypt otp secret")
	}
	ok, err := hunter2.TOTPVerify(params, code, hunter2.DefaultOTPHashes)
	if err != nil {
		return false, governor.ErrWithMsg(err, "Failed to verify otp code")
	}
	return ok, nil
}

// ValidateOTPBackup validates an otp backup code
func (r *repo) ValidateOTPBackup(decrypter *hunter2.Decrypter, m *Model, backup string) (bool, error) {
	code, err := decrypter.Decrypt(m.OTPBackup)
	if err != nil {
		return false, governor.ErrWithMsg(err, "Failed to decrypt otp backup")
	}
	return hmac.Equal([]byte(code), []byte(backup)), nil
}

// GenerateOTPSecret generates an otp secret
func (r *repo) GenerateOTPSecret(cipher hunter2.Cipher, m *Model, issuer string, alg string, digits int) (string, string, error) {
	params, uri, err := hunter2.TOTPGenerateSecret(totpSecretLen, hunter2.TOTPURI{
		TOTPConfig: hunter2.TOTPConfig{
			Alg:    alg,
			Digits: digits,
			Period: 30,
			Leeway: 1,
		},
		Issuer:      issuer,
		AccountName: m.Username,
	})
	if err != nil {
		return "", "", governor.ErrWithMsg(err, "Failed to generate otp secret")
	}
	backup, err := hunter2.GenerateRandomCode(totpBackupLen)
	if err != nil {
		return "", "", governor.ErrWithMsg(err, "Failed to generate otp backup")
	}
	encryptedParams, err := cipher.Encrypt(params)
	if err != nil {
		return "", "", governor.ErrWithMsg(err, "Failed to encrypt otp secret")
	}
	encryptedBackup, err := cipher.Encrypt(backup)
	if err != nil {
		return "", "", governor.ErrWithMsg(err, "Failed to encrypt otp backup")
	}
	m.OTPSecret = encryptedParams
	m.OTPBackup = encryptedBackup
	return uri, backup, nil
}

// GetGroup gets information from each user
func (r *repo) GetGroup(limit, offset int) ([]Info, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := userModelGetInfoOrdUserid(d, true, limit, offset)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get user info")
	}
	return m, nil
}

// GetBulk gets information from users
func (r *repo) GetBulk(userids []string) ([]Info, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := userModelGetInfoHasUseridOrdUserid(d, userids, true, len(userids), 0)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get user info of userids")
	}
	return m, nil
}

// GetByID returns a user model with the given id
func (r *repo) GetByID(userid string) (*Model, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, code, err := userModelGetModelEqUserid(d, userid)
	if err != nil {
		if code == 2 {
			return nil, governor.ErrWithKind(err, db.ErrNotFound{}, "No user found with that id")
		}
		return nil, governor.ErrWithMsg(err, "Failed to get user")
	}
	return m, nil
}

// GetByUsername returns a user model with the given username
func (r *repo) GetByUsername(username string) (*Model, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, code, err := userModelGetModelEqUsername(d, username)
	if err != nil {
		if code == 2 {
			return nil, governor.ErrWithKind(err, db.ErrNotFound{}, "No user found with that username")
		}
		return nil, governor.ErrWithMsg(err, "Failed to get user by username")
	}
	return m, nil
}

// GetByEmail returns a user model with the given email
func (r *repo) GetByEmail(email string) (*Model, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, code, err := userModelGetModelEqEmail(d, email)
	if err != nil {
		if code == 2 {
			return nil, governor.ErrWithKind(err, db.ErrNotFound{}, "No user found with that email")
		}
		return nil, governor.ErrWithMsg(err, "Failed to get user by email")
	}
	return m, nil
}

// Insert inserts the model into the db
func (r *repo) Insert(m *Model) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if code, err := userModelInsert(d, m); err != nil {
		if code == 3 {
			return governor.ErrWithKind(err, db.ErrUnique{}, "Username and email must be unique")
		}
		return governor.ErrWithMsg(err, "Failed to insert user")
	}
	return nil
}

// Update updates the model in the db
func (r *repo) Update(m *Model) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if code, err := userModelUpdModelEqUserid(d, m, m.Userid); err != nil {
		if code == 3 {
			return governor.ErrWithKind(err, db.ErrUnique{}, "Username and email must be unique")
		}
		return governor.ErrWithMsg(err, "Failed to update user")
	}
	return nil
}

// Delete deletes the model in the db
func (r *repo) Delete(m *Model) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := userModelDelEqUserid(d, m.Userid); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete user")
	}
	return nil
}

// Setup creates a new User table
func (r *repo) Setup() error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if code, err := userModelSetup(d); err != nil {
		if code != 5 {
			return governor.ErrWithMsg(err, "Failed to setup user model")
		}
	}
	return nil
}
