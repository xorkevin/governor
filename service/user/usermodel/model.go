package usermodel

import (
	"context"
	"crypto/hmac"
	"time"

	"xorkevin.dev/forge/model/sqldb"
	"xorkevin.dev/governor/service/dbsql"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/hunter2/h2cipher"
	"xorkevin.dev/hunter2/h2hash"
	"xorkevin.dev/hunter2/h2hash/passhash/argon2"
	"xorkevin.dev/hunter2/h2otp"
	"xorkevin.dev/kerrors"
)

//go:generate forge model

const (
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
		ValidateOTPCode(keyring *h2cipher.Keyring, m *Model, code string) (bool, error)
		ValidateOTPBackup(keyring *h2cipher.Keyring, m *Model, backup string) (bool, error)
		GenerateOTPSecret(ctx context.Context, cipher h2cipher.Cipher, m *Model, issuer string, alg string, digits int) (string, string, error)
		EnableOTP(ctx context.Context, m *Model) error
		DisableOTP(ctx context.Context, m *Model) error
		UpdateLoginFailed(ctx context.Context, m *Model) error
		GetGroup(ctx context.Context, limit, offset int) ([]Info, error)
		GetMany(ctx context.Context, userids []string) ([]Info, error)
		GetByUsernamePrefix(ctx context.Context, prefix string, limit, offset int) ([]Info, error)
		Exists(ctx context.Context, userid string) (bool, error)
		GetByID(ctx context.Context, userid string) (*Model, error)
		GetByUsername(ctx context.Context, username string) (*Model, error)
		GetByEmail(ctx context.Context, email string) (*Model, error)
		Insert(ctx context.Context, m *Model) error
		UpdateProps(ctx context.Context, m *Model) error
		UpdateEmail(ctx context.Context, m *Model) error
		Delete(ctx context.Context, m *Model) error
		Setup(ctx context.Context) error
	}

	repo struct {
		table    *userModelTable
		db       dbsql.Database
		hasher   h2hash.Hasher
		verifier *h2hash.Verifier
	}

	// Model is the db User model
	//forge:model user
	//forge:model:query user
	Model struct {
		Userid           string `model:"userid,VARCHAR(31) PRIMARY KEY"`
		Username         string `model:"username,VARCHAR(255) NOT NULL UNIQUE"`
		PassHash         string `model:"pass_hash,VARCHAR(255) NOT NULL"`
		OTPEnabled       bool   `model:"otp_enabled,BOOLEAN NOT NULL"`
		OTPSecret        string `model:"otp_secret,VARCHAR(255) NOT NULL"`
		OTPBackup        string `model:"otp_backup,VARCHAR(255) NOT NULL"`
		Email            string `model:"email,VARCHAR(255) NOT NULL UNIQUE"`
		FirstName        string `model:"first_name,VARCHAR(255) NOT NULL"`
		LastName         string `model:"last_name,VARCHAR(255) NOT NULL"`
		CreationTime     int64  `model:"creation_time,BIGINT NOT NULL"`
		FailedLoginTime  int64  `model:"failed_login_time,BIGINT NOT NULL"`
		FailedLoginCount int    `model:"failed_login_count,INT NOT NULL"`
	}

	// Info is the metadata of a user
	//forge:model:query user
	Info struct {
		Userid    string `model:"userid"`
		Username  string `model:"username"`
		Email     string `model:"email"`
		FirstName string `model:"first_name"`
		LastName  string `model:"last_name"`
	}

	//forge:model:query user
	userProps struct {
		Username  string `model:"username"`
		FirstName string `model:"first_name"`
		LastName  string `model:"last_name"`
	}

	//forge:model:query user
	userEmail struct {
		Email string `model:"email"`
	}

	//forge:model:query user
	userPassHash struct {
		PassHash string `model:"pass_hash"`
	}

	//forge:model:query user
	userGenOTP struct {
		OTPEnabled       bool   `model:"otp_enabled"`
		OTPSecret        string `model:"otp_secret"`
		OTPBackup        string `model:"otp_backup"`
		FailedLoginTime  int64  `model:"failed_login_time"`
		FailedLoginCount int    `model:"failed_login_count"`
	}

	//forge:model:query user
	userFailLogin struct {
		FailedLoginTime  int64 `model:"failed_login_time"`
		FailedLoginCount int   `model:"failed_login_count"`
	}
)

// New creates a new user repository
func New(database dbsql.Database, table string) Repo {
	hasher := argon2.New(passHashLen, passSaltLen, argon2.Config{
		Version:  argon2.Version,
		Time:     2,
		Mem:      19456,
		Parallel: 1,
	})
	verifier := h2hash.NewVerifier()
	verifier.Register(hasher)

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
	mUID, err := uid.New()
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to create new uid")
	}

	mHash, err := r.hasher.Hash([]byte(password))
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
	ok, err := r.verifier.Verify([]byte(password), m.PassHash)
	if err != nil {
		return false, kerrors.WithMsg(err, "Failed to verify password")
	}
	return ok, nil
}

// RehashPass updates the password with a new hash
func (r *repo) RehashPass(ctx context.Context, m *Model, password string) error {
	mHash, err := r.hasher.Hash([]byte(password))
	if err != nil {
		return kerrors.WithMsg(err, "Failed to rehash password")
	}
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.UpduserPassHashByID(ctx, d, &userPassHash{
		PassHash: mHash,
	}, m.Userid); err != nil {
		return kerrors.WithMsg(err, "Failed to update user")
	}
	m.PassHash = mHash
	return nil
}

// ValidateOTPCode validates an otp code
func (r *repo) ValidateOTPCode(keyring *h2cipher.Keyring, m *Model, code string) (bool, error) {
	params, err := keyring.Decrypt(m.OTPSecret)
	if err != nil {
		return false, kerrors.WithMsg(err, "Failed to decrypt otp secret")
	}
	ok, err := h2otp.TOTPVerify(string(params), code, h2otp.DefaultHashes)
	if err != nil {
		return false, kerrors.WithMsg(err, "Failed to verify otp code")
	}
	return ok, nil
}

// ValidateOTPBackup validates an otp backup code
func (r *repo) ValidateOTPBackup(keyring *h2cipher.Keyring, m *Model, backup string) (bool, error) {
	code, err := keyring.Decrypt(m.OTPBackup)
	if err != nil {
		return false, kerrors.WithMsg(err, "Failed to decrypt otp backup")
	}
	return hmac.Equal([]byte(code), []byte(backup)), nil
}

// GenerateOTPSecret generates an otp secret
func (r *repo) GenerateOTPSecret(ctx context.Context, cipher h2cipher.Cipher, m *Model, issuer string, alg string, digits int) (string, string, error) {
	params, uri, err := h2otp.TOTPGenerateSecret(totpSecretLen, h2otp.TOTPURI{
		TOTPConfig: h2otp.TOTPConfig{
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
	backup, err := h2otp.GenerateRandomCode(totpBackupLen)
	if err != nil {
		return "", "", kerrors.WithMsg(err, "Failed to generate otp backup")
	}
	encryptedParams, err := cipher.Encrypt([]byte(params))
	if err != nil {
		return "", "", kerrors.WithMsg(err, "Failed to encrypt otp secret")
	}
	encryptedBackup, err := cipher.Encrypt([]byte(backup))
	if err != nil {
		return "", "", kerrors.WithMsg(err, "Failed to encrypt otp backup")
	}
	d, err := r.db.DB(ctx)
	if err != nil {
		return "", "", err
	}
	if err := r.table.UpduserGenOTPByID(ctx, d, &userGenOTP{
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
	if err := r.table.UpduserGenOTPByID(ctx, d, &userGenOTP{
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
	if err := r.table.UpduserGenOTPByID(ctx, d, &userGenOTP{
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
	if err := r.table.UpduserFailLoginByID(ctx, d, &userFailLogin{
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
	m, err := r.table.GetInfoAll(ctx, d, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get user info")
	}
	return m, nil
}

// GetMany gets information from users
func (r *repo) GetMany(ctx context.Context, userids []string) ([]Info, error) {
	if len(userids) == 0 {
		return nil, nil
	}
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetInfoByIDs(ctx, d, userids, len(userids), 0)
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
	m, err := r.table.GetInfoByUsernamePrefix(ctx, d, prefix+"%", limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get user info of username prefix")
	}
	return m, nil
}

func (r *repo) userExists(ctx context.Context, d sqldb.Executor, userid string) (bool, error) {
	var exists bool
	if err := d.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM "+r.table.TableName+" WHERE userid = $1);", userid).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

// Exists returns if a user exists
func (r *repo) Exists(ctx context.Context, userid string) (bool, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return false, err
	}
	m, err := r.userExists(ctx, d, userid)
	if err != nil {
		return false, kerrors.WithMsg(err, "Failed to get user exists")
	}
	return m, nil
}

// GetByID returns a user model with the given id
func (r *repo) GetByID(ctx context.Context, userid string) (*Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelByID(ctx, d, userid)
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
	m, err := r.table.GetModelByUsername(ctx, d, username)
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
	m, err := r.table.GetModelByEmail(ctx, d, email)
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

// UpdateProps updates the user props
func (r *repo) UpdateProps(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.UpduserPropsByID(ctx, d, &userProps{
		Username:  m.Username,
		FirstName: m.FirstName,
		LastName:  m.LastName,
	}, m.Userid); err != nil {
		return kerrors.WithMsg(err, "Failed to update user props")
	}
	return nil
}

// UpdateEmail updates the user email
func (r *repo) UpdateEmail(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.UpduserEmailByID(ctx, d, &userEmail{
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
	if err := r.table.DelByID(ctx, d, m.Userid); err != nil {
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
		return kerrors.WithMsg(err, "Failed to setup user model")
	}
	return nil
}
