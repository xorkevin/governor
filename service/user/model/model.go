package model

import (
	"context"
	"crypto/subtle"
	"errors"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/hunter2"
	"xorkevin.dev/kerrors"
)

//go:generate forge model -m Model -p user -o model_gen.go Model Info userEmail userPassHash userGenOTP userFailLogin

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
		RehashPass(ctx context.Context, m *Model, password string) error
		ValidateOTPCode(decrypter *hunter2.Decrypter, m *Model, code string) (bool, error)
		ValidateOTPBackup(decrypter *hunter2.Decrypter, m *Model, backup string) (bool, error)
		GenerateOTPSecret(ctx context.Context, cipher hunter2.Cipher, m *Model, issuer string, alg string, digits int) (string, string, error)
		EnableOTP(ctx context.Context, m *Model) error
		DisableOTP(ctx context.Context, m *Model) error
		UpdateLoginFailed(ctx context.Context, m *Model) error
		GetGroup(ctx context.Context, limit, offset int) ([]Info, error)
		GetBulk(ctx context.Context, userids []string) ([]Info, error)
		GetByUsernamePrefix(ctx context.Context, prefix string, limit, offset int) ([]Info, error)
		GetByID(ctx context.Context, userid string) (*Model, error)
		GetByUsername(ctx context.Context, username string) (*Model, error)
		GetByEmail(ctx context.Context, email string) (*Model, error)
		Insert(ctx context.Context, m *Model) error
		UpdateEmail(ctx context.Context, m *Model) error
		Delete(ctx context.Context, m *Model) error
		Setup(ctx context.Context) error
	}

	repo struct {
		table    *userModelTable
		db       db.Database
		hasher   hunter2.Hasher
		verifier *hunter2.Verifier
	}

	// Model is the db User model
	Model struct {
		Userid           string `model:"userid,VARCHAR(31) PRIMARY KEY" query:"userid;getoneeq,userid;deleq,userid"`
		Username         string `model:"username,VARCHAR(255) NOT NULL UNIQUE" query:"username;getoneeq,username"`
		PassHash         string `model:"pass_hash,VARCHAR(255) NOT NULL" query:"pass_hash"`
		OTPEnabled       bool   `model:"otp_enabled,BOOLEAN NOT NULL" query:"otp_enabled"`
		OTPSecret        string `model:"otp_secret,VARCHAR(255) NOT NULL" query:"otp_secret"`
		OTPBackup        string `model:"otp_backup,VARCHAR(255) NOT NULL" query:"otp_backup"`
		Email            string `model:"email,VARCHAR(255) NOT NULL UNIQUE" query:"email;getoneeq,email"`
		FirstName        string `model:"first_name,VARCHAR(255) NOT NULL" query:"first_name"`
		LastName         string `model:"last_name,VARCHAR(255) NOT NULL" query:"last_name"`
		CreationTime     int64  `model:"creation_time,BIGINT NOT NULL" query:"creation_time"`
		FailedLoginTime  int64  `model:"failed_login_time,BIGINT NOT NULL" query:"failed_login_time"`
		FailedLoginCount int    `model:"failed_login_count,INT NOT NULL" query:"failed_login_count"`
	}

	userEmail struct {
		Email string `query:"email;updeq,userid"`
	}

	userPassHash struct {
		PassHash string `query:"pass_hash;updeq,userid"`
	}

	userGenOTP struct {
		OTPEnabled       bool   `query:"otp_enabled;updeq,userid"`
		OTPSecret        string `query:"otp_secret"`
		OTPBackup        string `query:"otp_backup"`
		FailedLoginTime  int64  `query:"failed_login_time"`
		FailedLoginCount int    `query:"failed_login_count"`
	}

	userFailLogin struct {
		FailedLoginTime  int64 `query:"failed_login_time;updeq,userid"`
		FailedLoginCount int   `query:"failed_login_count"`
	}

	// Info is the metadata of a user
	Info struct {
		Userid    string `query:"userid;getgroup;getgroupeq,userid|arr"`
		Username  string `query:"username;getgroupeq,username|like"`
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
func NewInCtx(inj governor.Injector, table string) {
	SetCtxRepo(inj, NewCtx(inj, table))
}

// NewCtx creates a new user repo from a context
func NewCtx(inj governor.Injector, table string) Repo {
	dbService := db.GetCtxDB(inj)
	return New(dbService, table)
}

// New creates a new user repository
func New(database db.Database, table string) Repo {
	hasher := hunter2.NewScryptHasher(passHashLen, passSaltLen, hunter2.DefaultScryptConfig)
	verifier := hunter2.NewVerifier()
	verifier.RegisterHash(hasher)

	return &repo{
		table: &userModelTable{
			TableName: table,
		},
		db:       database,
		hasher:   hasher,
		verifier: verifier,
	}
}

// New creates a new User Model
func (r *repo) New(username, password, email, firstname, lastname string) (*Model, error) {
	mUID, err := uid.New(uidSize)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to create new uid")
	}

	mHash, err := r.hasher.Hash(password)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to hash password")
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
		return false, kerrors.WithMsg(err, "Failed to verify password")
	}
	return ok, nil
}

// RehashPass updates the password with a new hash
func (r *repo) RehashPass(ctx context.Context, m *Model, password string) error {
	mHash, err := r.hasher.Hash(password)
	if err != nil {
		return kerrors.WithMsg(err, "Failed to rehash password")
	}
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.UpduserPassHashEqUserid(ctx, d, &userPassHash{
		PassHash: mHash,
	}, m.Userid); err != nil {
		return kerrors.WithMsg(err, "Failed to update user")
	}
	m.PassHash = mHash
	return nil
}

// ValidateOTPCode validates an otp code
func (r *repo) ValidateOTPCode(decrypter *hunter2.Decrypter, m *Model, code string) (bool, error) {
	params, err := decrypter.Decrypt(m.OTPSecret)
	if err != nil {
		return false, kerrors.WithMsg(err, "Failed to decrypt otp secret")
	}
	ok, err := hunter2.TOTPVerify(params, code, hunter2.DefaultOTPHashes)
	if err != nil {
		return false, kerrors.WithMsg(err, "Failed to verify otp code")
	}
	return ok, nil
}

// ValidateOTPBackup validates an otp backup code
func (r *repo) ValidateOTPBackup(decrypter *hunter2.Decrypter, m *Model, backup string) (bool, error) {
	code, err := decrypter.Decrypt(m.OTPBackup)
	if err != nil {
		return false, kerrors.WithMsg(err, "Failed to decrypt otp backup")
	}
	return subtle.ConstantTimeCompare([]byte(code), []byte(backup)) == 1, nil
}

// GenerateOTPSecret generates an otp secret
func (r *repo) GenerateOTPSecret(ctx context.Context, cipher hunter2.Cipher, m *Model, issuer string, alg string, digits int) (string, string, error) {
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
		return "", "", kerrors.WithMsg(err, "Failed to generate otp secret")
	}
	backup, err := hunter2.GenerateRandomCode(totpBackupLen)
	if err != nil {
		return "", "", kerrors.WithMsg(err, "Failed to generate otp backup")
	}
	encryptedParams, err := cipher.Encrypt(params)
	if err != nil {
		return "", "", kerrors.WithMsg(err, "Failed to encrypt otp secret")
	}
	encryptedBackup, err := cipher.Encrypt(backup)
	if err != nil {
		return "", "", kerrors.WithMsg(err, "Failed to encrypt otp backup")
	}
	d, err := r.db.DB(ctx)
	if err != nil {
		return "", "", err
	}
	if err := r.table.UpduserGenOTPEqUserid(ctx, d, &userGenOTP{
		OTPEnabled:       false,
		OTPSecret:        encryptedParams,
		OTPBackup:        encryptedBackup,
		FailedLoginTime:  0,
		FailedLoginCount: 0,
	}, m.Userid); err != nil {
		return "", "", kerrors.WithMsg(err, "Failed to update user otp settings")
	}
	m.OTPEnabled = false
	m.OTPSecret = encryptedParams
	m.OTPBackup = encryptedBackup
	m.FailedLoginTime = 0
	m.FailedLoginCount = 0
	return uri, backup, nil
}

// EnableOTP enables OTP
func (r *repo) EnableOTP(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.UpduserGenOTPEqUserid(ctx, d, &userGenOTP{
		OTPEnabled:       true,
		OTPSecret:        m.OTPSecret,
		OTPBackup:        m.OTPBackup,
		FailedLoginTime:  0,
		FailedLoginCount: 0,
	}, m.Userid); err != nil {
		return kerrors.WithMsg(err, "Failed to update user otp settings")
	}
	m.OTPEnabled = true
	m.FailedLoginTime = 0
	m.FailedLoginCount = 0
	return nil
}

// DisableOTP disables OTP
func (r *repo) DisableOTP(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.UpduserGenOTPEqUserid(ctx, d, &userGenOTP{
		OTPEnabled:       false,
		OTPSecret:        "",
		OTPBackup:        "",
		FailedLoginTime:  0,
		FailedLoginCount: 0,
	}, m.Userid); err != nil {
		return kerrors.WithMsg(err, "Failed to update user otp settings")
	}
	m.OTPEnabled = false
	m.OTPSecret = ""
	m.OTPBackup = ""
	m.FailedLoginTime = 0
	m.FailedLoginCount = 0
	return nil
}

// UpdateLoginFailed updates login failure count
func (r *repo) UpdateLoginFailed(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.UpduserFailLoginEqUserid(ctx, d, &userFailLogin{
		FailedLoginTime:  m.FailedLoginTime,
		FailedLoginCount: m.FailedLoginCount,
	}, m.Userid); err != nil {
		return kerrors.WithMsg(err, "Failed to update user otp failure count")
	}
	return nil
}

// GetGroup gets information from each user
func (r *repo) GetGroup(ctx context.Context, limit, offset int) ([]Info, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetInfoOrdUserid(ctx, d, true, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get user info")
	}
	return m, nil
}

// GetBulk gets information from users
func (r *repo) GetBulk(ctx context.Context, userids []string) ([]Info, error) {
	if len(userids) == 0 {
		return nil, nil
	}
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetInfoHasUseridOrdUserid(ctx, d, userids, true, len(userids), 0)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get user info of userids")
	}
	return m, nil
}

// GetByUsernamePrefix gets users by username prefix
func (r *repo) GetByUsernamePrefix(ctx context.Context, prefix string, limit, offset int) ([]Info, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetInfoLikeUsernameOrdUsername(ctx, d, prefix+"%", true, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get user info of username prefix")
	}
	return m, nil
}

// GetByID returns a user model with the given id
func (r *repo) GetByID(ctx context.Context, userid string) (*Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelEqUserid(ctx, d, userid)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get user")
	}
	return m, nil
}

// GetByUsername returns a user model with the given username
func (r *repo) GetByUsername(ctx context.Context, username string) (*Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelEqUsername(ctx, d, username)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get user by username")
	}
	return m, nil
}

// GetByEmail returns a user model with the given email
func (r *repo) GetByEmail(ctx context.Context, email string) (*Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelEqEmail(ctx, d, email)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get user by email")
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
		return kerrors.WithMsg(err, "Failed to insert user")
	}
	return nil
}

// UpdateEmail updates the user email
func (r *repo) UpdateEmail(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.UpduserEmailEqUserid(ctx, d, &userEmail{
		Email: m.Email,
	}, m.Userid); err != nil {
		return kerrors.WithMsg(err, "Failed to update user email")
	}
	return nil
}

// Delete deletes the model in the db
func (r *repo) Delete(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.DelEqUserid(ctx, d, m.Userid); err != nil {
		return kerrors.WithMsg(err, "Failed to delete user")
	}
	return nil
}

// Setup creates a new User table
func (r *repo) Setup(ctx context.Context) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.Setup(ctx, d); err != nil {
		err = kerrors.WithMsg(err, "Failed to setup user model")
		if !errors.Is(err, db.ErrAuthz{}) {
			return err
		}
	}
	return nil
}
