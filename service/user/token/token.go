package token

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/blake2b"
	_ "golang.org/x/crypto/blake2b" // depends on registering blake2b hash
	"gopkg.in/square/go-jose.v2"
	"gopkg.in/square/go-jose.v2/jwt"
	"xorkevin.dev/governor"
	"xorkevin.dev/kerrors"
)

const (
	// ScopeAll grants all scopes to a token
	ScopeAll = "all"
)

const (
	// KindAccess is an access token kind
	KindAccess = "access"
	// KindRefresh is a refresh token kind
	KindRefresh = "refresh"
	// KindOAuthAccess is an oauth access token kind
	KindOAuthAccess = "oauth:access"
	// KindOAuthRefresh is an oauth refresh token kind
	KindOAuthRefresh = "oauth:refresh"
	// KindOAuthID is an openid id token kind
	KindOAuthID = "oauth:id"
)

const (
	jwtHeaderKid = "kid"
	jwtHeaderJWT = "JWT"
)

type (
	// Claims is a set of fields to describe a user
	Claims struct {
		jwt.Claims
		Kind     string `json:"kind"`
		AuthTime int64  `json:"auth_time,omitempty"`
		Scope    string `json:"scope,omitempty"`
		Key      string `json:"key,omitempty"`
	}

	// Tokenizer is a token generator
	Tokenizer interface {
		GetJWKS() *jose.JSONWebKeySet
		Generate(kind string, userid string, duration int64, id string, authTime int64, scope string, key string) (string, *Claims, error)
		GenerateExt(kind string, issuer string, userid string, audience []string, duration int64, id string, authTime int64, claims interface{}) (string, error)
		Validate(kind string, tokenString string) (bool, *Claims)
		GetClaims(kind string, tokenString string) (bool, *Claims)
		GetClaimsExt(kind string, tokenString string, audience []string, claims interface{}) (bool, *Claims)
	}

	// Service is a Tokenizer and governor.Service
	Service interface {
		governor.Service
		Tokenizer
	}

	getSignerRes struct {
		err error
	}

	getOp struct {
		ctx context.Context
		res chan<- getSignerRes
	}

	service struct {
		hs512id    string
		signer     jose.Signer
		rs256id    string
		keySigner  jose.Signer
		keys       signingKeys
		jwks       []jose.JSONWebKey
		issuer     string
		audience   string
		config     governor.SecretReader
		logger     governor.Logger
		ops        chan getOp
		ready      *atomic.Bool
		hbfailed   int
		hbinterval int
		hbmaxfail  int
		done       <-chan struct{}
		keyrefresh int
	}

	ctxKeyTokenizer struct{}
)

// GetCtxTokenizer returns a Tokenizer from the context
func GetCtxTokenizer(inj governor.Injector) Tokenizer {
	v := inj.Get(ctxKeyTokenizer{})
	if v == nil {
		return nil
	}
	return v.(Tokenizer)
}

// setCtxTokenizer sets a Tokenizer in the context
func setCtxTokenizer(inj governor.Injector, t Tokenizer) {
	inj.Set(ctxKeyTokenizer{}, t)
}

// New creates a new Tokenizer
func New() Service {
	return &service{
		issuer:   "",
		audience: "",
		signer:   nil,
		ops:      make(chan getOp),
		ready:    &atomic.Bool{},
		hbfailed: 0,
	}
}

func (s *service) Register(name string, inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	setCtxTokenizer(inj, s)

	r.SetDefault("tokensecret", "")
	r.SetDefault("issuer", "governor")
	r.SetDefault("audience", "governor")
	r.SetDefault("hbinterval", 5)
	r.SetDefault("hbmaxfail", 6)
	r.SetDefault("signerrefresh", 60)
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, m governor.Router) error {
	s.logger = l
	l = s.logger.WithData(map[string]string{
		"phase": "init",
	})

	s.config = r

	issuer := r.GetStr("issuer")
	if issuer == "" {
		return kerrors.WithKind(nil, governor.ErrInvalidConfig{}, "Token issuer is not set")
	}
	s.issuer = issuer

	audience := r.GetStr("audience")
	if audience == "" {
		return kerrors.WithKind(nil, governor.ErrInvalidConfig{}, "Token audience is not set")
	}
	s.audience = audience

	s.hbinterval = r.GetInt("hbinterval")
	s.hbmaxfail = r.GetInt("hbmaxfail")
	s.keyrefresh = r.GetInt("keyrefresh")

	l.Info("Loaded config", map[string]string{
		"issuer":     issuer,
		"audience":   audience,
		"hbinterval": strconv.Itoa(s.hbinterval),
		"hbmaxfail":  strconv.Itoa(s.hbmaxfail),
		"keyrefresh": strconv.Itoa(s.keyrefresh),
	})

	done := make(chan struct{})
	go s.execute(ctx, done)
	s.done = done

	return nil
}

func (s *service) execute(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(time.Duration(s.hbinterval) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.handlePing(ctx)
		}
	}
}

func (s *service) handlePing(ctx context.Context) {
	err := s.refreshTokenSecrets(ctx)
	if err == nil {
		s.ready.Store(true)
		s.hbfailed = 0
		return
	}
	s.hbfailed++
	if s.hbfailed < s.hbmaxfail {
		s.logger.Warn("Failed to refresh token keys", map[string]string{
			"error":      err.Error(),
			"actiontype": "token_refresh_keys",
		})
		return
	}
	s.logger.Error("Failed max refresh attempts", map[string]string{
		"error":      err.Error(),
		"actiontype": "token_refresh_keys",
	})
	s.ready.Store(false)
	s.hbfailed = 0
}

type (
	// ErrSigner is returned when failing to create a signer
	ErrSigner struct{}
	// ErrGenerate is returned when failing to generate a token
	ErrGenerate struct{}
)

func (e ErrSigner) Error() string {
	return "Error creating signer"
}

func (e ErrGenerate) Error() string {
	return "Error generating token"
}

type (
	secretToken struct {
		Secrets []string `mapstructure:"secrets"`
	}
)

type (
	signingKey interface {
		Alg() string
		ID() string
		Private() crypto.PrivateKey
		Public() crypto.PublicKey
	}

	signingKeys struct {
		keys map[string]signingKey
	}
)

func (s *signingKeys) Register(k signingKey) {
	s.keys[k.ID()] = k
}

func (s *signingKeys) Get(id string) signingKey {
	k, ok := s.keys[id]
	if !ok {
		return nil
	}
	return k
}

func signingKeyID(params string) string {
	k := blake2b.Sum256([]byte(params))
	return base64.RawURLEncoding.EncodeToString(k[:])
}

const (
	signingAlgHS512 = "hs512"
	signingAlgRS256 = "rs256"
)

func signingKeyFromParams(params string) (signingKey, error) {
	id, _, _ := strings.Cut(strings.TrimPrefix(params, "$"), "$")
	switch id {
	case signingAlgHS512:
		return hs512FromParams(params)
	case signingAlgRS256:
		return rs256FromParams(params)
	default:
		return nil, kerrors.WithMsg(nil, "Not supported")
	}
}

type (
	hs512Key struct {
		kid string
		key []byte
	}
)

func (k *hs512Key) Alg() string {
	return signingAlgHS512
}

func (k *hs512Key) ID() string {
	return k.kid
}

func (k *hs512Key) Private() crypto.PrivateKey {
	return k.key
}

func (k *hs512Key) Public() crypto.PublicKey {
	return k.key
}

func hs512FromParams(params string) (*hs512Key, error) {
	b := strings.Split(strings.TrimPrefix(params, "$"), "$")
	if len(b) != 2 || b[0] != signingAlgHS512 {
		return nil, kerrors.WithMsg(nil, "Invalid params format")
	}
	key, err := base64.RawURLEncoding.DecodeString(b[1])
	if err != nil {
		return nil, kerrors.WithMsg(err, "Invalid hs512 key")
	}
	return &hs512Key{
		kid: signingKeyID(params),
		key: key,
	}, nil
}

type (
	rs256Key struct {
		kid string
		key *rsa.PrivateKey
		pub crypto.PublicKey
	}
)

func (k *rs256Key) Alg() string {
	return signingAlgRS256
}

func (k *rs256Key) ID() string {
	return k.kid
}

func (k *rs256Key) Private() crypto.PrivateKey {
	return k.key
}

func (k *rs256Key) Public() crypto.PublicKey {
	return k.pub
}

const (
	rsaPrivateBlockType = "PRIVATE KEY"
)

func rs256FromParams(params string) (*rs256Key, error) {
	b := strings.Split(strings.TrimPrefix(params, "$"), "$")
	if len(b) != 2 || b[0] != signingAlgRS256 {
		return nil, kerrors.WithMsg(nil, "Invalid params format")
	}
	pemBlock, rest := pem.Decode([]byte(b[1]))
	if pemBlock == nil || pemBlock.Type != rsaPrivateBlockType || len(rest) != 0 {
		return nil, kerrors.WithKind(nil, governor.ErrInvalidConfig{}, "Invalid rsakey pem")
	}
	rawKey, err := x509.ParsePKCS8PrivateKey(pemBlock.Bytes)
	if err != nil {
		return nil, kerrors.WithKind(err, governor.ErrInvalidConfig{}, "Invalid pkcs8 rsa key")
	}
	key, ok := rawKey.(*rsa.PrivateKey)
	if !ok {
		return nil, kerrors.WithKind(nil, governor.ErrInvalidConfig{}, "Invalid created pkcs8 rsa key")
	}
	key.Precompute()
	return &rs256Key{
		kid: signingKeyID(params),
		key: key,
		pub: key.Public(),
	}, nil
}

func (s *service) refreshTokenSecrets(ctx context.Context) error {
	var tokenSecrets secretToken
	if err := s.config.GetSecret(ctx, "tokensecret", int64(s.keyrefresh), &tokenSecrets); err != nil {
		return kerrors.WithMsg(err, "Invalid token secret")
	}
	var khs512 signingKey
	var krs256 signingKey
	keys := signingKeys{
		keys: map[string]signingKey{},
	}
	var jwks []jose.JSONWebKey
	for _, i := range tokenSecrets.Secrets {
		k, err := signingKeyFromParams(i)
		if err != nil {
			return kerrors.WithKind(err, governor.ErrInvalidConfig{}, "Invalid key param")
		}
		switch k.Alg() {
		case signingAlgHS512:
			if khs512 == nil {
				khs512 = k
			}
		case signingAlgRS256:
			jwks = append(jwks, jose.JSONWebKey{
				Algorithm: "RS256",
				KeyID:     k.ID(),
				Key:       k.Public(),
				Use:       "sig",
			})
			if krs256 == nil {
				krs256 = k
			}
		}
		keys.Register(k)
	}
	if khs512 == nil || krs256 == nil {
		return kerrors.WithKind(nil, governor.ErrInvalidConfig{}, "No token keys present")
	}
	if khs512.ID() == s.hs512id && krs256.ID() == s.rs256id {
		// first signing keys of each type match current signing keys, therefore no
		// change in signing keys
		return nil
	}

	sig, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.HS512, Key: khs512.Private().([]byte)}, (&jose.SignerOptions{}).WithType(jwtHeaderJWT).WithHeader(jwtHeaderKid, khs512.ID()))
	if err != nil {
		return kerrors.WithKind(err, ErrSigner{}, "Failed to create new jwt HS512 signer")
	}

	keySig, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: krs256.Private().(*rsa.PrivateKey)}, (&jose.SignerOptions{}).WithType(jwtHeaderJWT).WithHeader(jwtHeaderKid, krs256.ID()))
	if err != nil {
		return kerrors.WithKind(err, ErrSigner{}, "Failed to create new jwt RS256 signer")
	}

	s.hs512id = khs512.ID()
	s.signer = sig
	s.rs256id = krs256.ID()
	s.keySigner = keySig
	s.keys = keys
	s.jwks = jwks

	s.logger.Info("Refreshed token keys with new keys", map[string]string{
		"actiontype": "token_refresh_keys",
		"hs512kid":   s.hs512id,
		"rs256kid":   s.rs256id,
		"numjwks":    strconv.Itoa(len(jwks)),
		"numother":   strconv.Itoa(len(tokenSecrets.Secrets) - len(jwks)),
	})
	return nil
}

func (s *service) Setup(req governor.ReqSetup) error {
	return nil
}

func (s *service) PostSetup(req governor.ReqSetup) error {
	return nil
}

func (s *service) Start(ctx context.Context) error {
	return nil
}

func (s *service) Stop(ctx context.Context) {
	l := s.logger.WithData(map[string]string{
		"phase": "stop",
	})
	select {
	case <-s.done:
		return
	case <-ctx.Done():
		l.Warn("Failed to stop", map[string]string{
			"error":      ctx.Err().Error(),
			"actiontype": "token_stop",
		})
	}
}

func (s *service) Health() error {
	if !s.ready.Load() {
		return kerrors.WithKind(nil, governor.ErrInvalidConfig{}, "Token service not ready")
	}
	return nil
}

// GetJWKS returns an RFC 7517 representation of the public signing key
func (s *service) GetJWKS() *jose.JSONWebKeySet {
	return &jose.JSONWebKeySet{
		Keys: s.jwks,
	}
}

// Generate returns a new jwt token from a user model
func (s *service) Generate(kind string, userid string, duration int64, id string, authTime int64, scope string, key string) (string, *Claims, error) {
	now := time.Now().Round(0)
	claims := Claims{
		Claims: jwt.Claims{
			Issuer:    s.issuer,
			Subject:   userid,
			Audience:  []string{s.audience},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Expiry:    jwt.NewNumericDate(time.Unix(now.Unix()+duration, 0)),
			ID:        id,
		},
		Kind:     kind,
		AuthTime: authTime,
		Scope:    scope,
		Key:      key,
	}
	token, err := jwt.Signed(s.signer).Claims(claims).CompactSerialize()
	if err != nil {
		return "", nil, kerrors.WithKind(err, ErrGenerate{}, "Failed to generate a new jwt token")
	}
	return token, &claims, nil
}

// GenerateExt creates a new id token
func (s *service) GenerateExt(kind string, issuer string, userid string, audience []string, duration int64, id string, authTime int64, claims interface{}) (string, error) {
	now := time.Now().Round(0)
	baseClaims := Claims{
		Claims: jwt.Claims{
			Issuer:    issuer,
			Subject:   userid,
			Audience:  audience,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Expiry:    jwt.NewNumericDate(time.Unix(now.Unix()+duration, 0)),
			ID:        id,
		},
		Kind:     kind,
		AuthTime: authTime,
	}
	token, err := jwt.Signed(s.keySigner).Claims(baseClaims).Claims(claims).CompactSerialize()
	if err != nil {
		return "", kerrors.WithKind(err, ErrGenerate{}, "Failed to generate a new jwt token")
	}
	return token, nil
}

// HasScope returns if a token scope contains a scope
func HasScope(tokenScope string, scope string) bool {
	if scope == "" {
		return true
	}
	for _, i := range strings.Fields(tokenScope) {
		if i == ScopeAll || i == scope {
			return true
		}
	}
	return false
}

// Validate returns whether a token is valid
func (s *service) Validate(kind string, tokenString string) (bool, *Claims) {
	token, err := jwt.ParseSigned(tokenString)
	if err != nil {
		return false, nil
	}
	if len(token.Headers) != 1 {
		return false, nil
	}
	key := s.keys.Get(token.Headers[0].KeyID)
	if key == nil {
		return false, nil
	}
	claims := &Claims{}
	if err := token.Claims(key.Public(), claims); err != nil {
		return false, nil
	}
	if claims.Kind != kind {
		return false, nil
	}
	now := time.Now().Round(0)
	if err := claims.ValidateWithLeeway(jwt.Expected{
		Issuer:   s.issuer,
		Audience: []string{s.audience},
		Time:     now,
	}, 0); err != nil {
		return false, nil
	}
	return true, claims
}

// GetClaims returns token claims without validating time
func (s *service) GetClaims(kind string, tokenString string) (bool, *Claims) {
	token, err := jwt.ParseSigned(tokenString)
	if err != nil {
		return false, nil
	}
	if len(token.Headers) != 1 {
		return false, nil
	}
	key := s.keys.Get(token.Headers[0].KeyID)
	if key == nil {
		return false, nil
	}
	claims := &Claims{}
	if err := token.Claims(key.Public(), claims); err != nil {
		return false, nil
	}
	if claims.Kind != kind {
		return false, nil
	}
	if err := claims.ValidateWithLeeway(jwt.Expected{
		Issuer:   s.issuer,
		Audience: []string{s.audience},
	}, 0); err != nil {
		return false, nil
	}
	return true, claims
}

// GetClaimsExt returns external token claims without validating time
func (s *service) GetClaimsExt(kind string, tokenString string, audience []string, claims interface{}) (bool, *Claims) {
	token, err := jwt.ParseSigned(tokenString)
	if err != nil {
		return false, nil
	}
	if len(token.Headers) != 1 {
		return false, nil
	}
	key := s.keys.Get(token.Headers[0].KeyID)
	if key == nil {
		return false, nil
	}
	if claims == nil {
		claims = &struct{}{}
	}
	baseClaims := &Claims{}
	if err := token.Claims(key.Public(), baseClaims, claims); err != nil {
		return false, nil
	}
	if baseClaims.Kind != kind {
		return false, nil
	}
	if err := baseClaims.ValidateWithLeeway(jwt.Expected{
		Issuer:   s.issuer,
		Audience: audience,
	}, 0); err != nil {
		return false, nil
	}
	return true, baseClaims
}
