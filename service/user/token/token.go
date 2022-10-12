package token

import (
	"context"
	"crypto"
	"strings"
	"time"

	"gopkg.in/square/go-jose.v2"
	"gopkg.in/square/go-jose.v2/jwt"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/util/ksync"
	"xorkevin.dev/governor/util/lifecycle"
	"xorkevin.dev/hunter2"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

const (
	// ScopeAll grants all scopes to a token
	ScopeAll = "all"
	// ScopeForbidden denies all access
	ScopeForbidden = "forbidden"
)

const (
	// KeyIDSystem is the system id key
	KeyIDSystem = "gov:system"
)

type (
	// Kind is a token kind
	Kind string
)

const (
	// KindAccess is an access token kind
	KindAccess Kind = "access"
	// KindRefresh is a refresh token kind
	KindRefresh = "refresh"
	// KindSystem is a system token kind
	KindSystem = "system"
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
		Kind     Kind   `json:"kind"`
		AuthTime int64  `json:"auth_time,omitempty"`
		Scope    string `json:"scope,omitempty"`
		Key      string `json:"key,omitempty"`
	}

	// Tokenizer is a token generator
	Tokenizer interface {
		GetJWKS(ctx context.Context) (*jose.JSONWebKeySet, error)
		Generate(ctx context.Context, kind Kind, userid string, duration time.Duration, id string, authTime int64, scope string, key string) (string, *Claims, error)
		GenerateExt(ctx context.Context, kind Kind, issuer string, userid string, audience []string, duration time.Duration, id string, authTime int64, claims interface{}) (string, error)
		Validate(ctx context.Context, kind Kind, tokenString string) (bool, *Claims)
		GetClaims(ctx context.Context, kind Kind, tokenString string) (bool, *Claims)
		GetClaimsExt(ctx context.Context, kind Kind, tokenString string, audience []string, claims interface{}) (bool, *Claims)
	}

	tokenSigner struct {
		signer          jose.Signer
		extsigner       jose.Signer
		signingkeys     *hunter2.SigningKeyring
		extsigningkeys  *hunter2.SigningKeyring
		sysverifierkeys *hunter2.VerifierKeyring
		jwks            []jose.JSONWebKey
		hs512id         string
		rs256id         string
		eddsaid         string
	}

	Service struct {
		lc         *lifecycle.Lifecycle[tokenSigner]
		issuer     string
		audience   string
		config     governor.SecretReader
		log        *klog.LevelLogger
		hbfailed   int
		hbmaxfail  int
		keyrefresh time.Duration
		wg         *ksync.WaitGroup
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
func New() *Service {
	return &Service{
		hbfailed: 0,
		wg:       ksync.NewWaitGroup(),
	}
}

func (s *Service) Register(name string, inj governor.Injector, r governor.ConfigRegistrar) {
	setCtxTokenizer(inj, s)

	r.SetDefault("tokensecret", "")
	r.SetDefault("issuer", "governor")
	r.SetDefault("audience", "governor")
	r.SetDefault("hbinterval", "5s")
	r.SetDefault("hbmaxfail", 6)
	r.SetDefault("keyrefresh", "1m")
}

func (s *Service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, log klog.Logger, m governor.Router) error {
	s.log = klog.NewLevelLogger(log)
	s.config = r

	issuer := r.GetStr("issuer")
	if issuer == "" {
		return kerrors.WithMsg(nil, "Token issuer is not set")
	}
	s.issuer = issuer

	audience := r.GetStr("audience")
	if audience == "" {
		return kerrors.WithMsg(nil, "Token audience is not set")
	}
	s.audience = audience

	hbinterval, err := r.GetDuration("hbinterval")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse hbinterval")
	}
	s.hbmaxfail = r.GetInt("hbmaxfail")
	s.keyrefresh, err = r.GetDuration("keyrefresh")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse keyrefresh")
	}

	s.log.Info(ctx, "Loaded config", klog.Fields{
		"token.issuer":     issuer,
		"token.audience":   audience,
		"token.hbinterval": hbinterval.String(),
		"token.hbmaxfail":  s.hbmaxfail,
		"token.keyrefresh": s.keyrefresh.String(),
	})

	ctx = klog.WithFields(ctx, klog.Fields{
		"gov.service.phase": "run",
	})

	s.lc = lifecycle.New(
		ctx,
		s.getSecrets,
		s.closeSecrets,
		s.handlePing,
		hbinterval,
	)
	go s.lc.Heartbeat(ctx, s.wg)

	return nil
}

func (s *Service) handlePing(ctx context.Context, m *lifecycle.Manager[tokenSigner]) {
	_, err := m.Construct(ctx)
	if err == nil {
		s.hbfailed = 0
		return
	}
	s.hbfailed++
	if s.hbfailed < s.hbmaxfail {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to refresh token keys"), nil)
		return
	}
	s.log.Err(ctx, kerrors.WithMsg(err, "Failed max refresh attempts"), nil)
	s.hbfailed = 0
	// clear the cached signer because its secret may be invalid
	m.Stop(ctx)
}

type (
	// ErrorSigner is returned when failing to create a signer
	ErrorSigner struct{}
	// ErrorGenerate is returned when failing to generate a token
	ErrorGenerate struct{}
)

func (e ErrorSigner) Error() string {
	return "Error creating signer"
}

func (e ErrorGenerate) Error() string {
	return "Error generating token"
}

type (
	secretToken struct {
		Secrets    []string `mapstructure:"secrets"`
		ExtKeys    []string `mapstructure:"extkeys"`
		SysPubKeys []string `mapstructure:"syspubkeys"`
	}
)

func (s *Service) getSecrets(ctx context.Context, m *lifecycle.Manager[tokenSigner]) (*tokenSigner, error) {
	currentSigner := m.Load(ctx)
	var tokenSecrets secretToken
	if err := s.config.GetSecret(ctx, "tokensecret", s.keyrefresh, &tokenSecrets); err != nil {
		return nil, kerrors.WithMsg(err, "Invalid token secret")
	}
	signingkeys, sig, hs512id, err := s.getTokenSecrets(tokenSecrets.Secrets, currentSigner)
	if err != nil {
		return nil, err
	}
	extsigningkeys, extsig, rs256id, jwks, err := s.getExtSecrets(tokenSecrets.ExtKeys, currentSigner)
	if err != nil {
		return nil, err
	}
	sysverifierkeys, eddsaid, err := s.getSysVerifierSecrets(tokenSecrets.SysPubKeys, currentSigner)
	if err != nil {
		return nil, err
	}
	if currentSigner != nil && hs512id == currentSigner.hs512id && rs256id == currentSigner.rs256id && eddsaid == currentSigner.eddsaid {
		return currentSigner, nil
	}

	m.Stop(ctx)

	signer := &tokenSigner{
		signer:          sig,
		extsigner:       extsig,
		signingkeys:     signingkeys,
		extsigningkeys:  extsigningkeys,
		sysverifierkeys: sysverifierkeys,
		jwks:            jwks,
		hs512id:         hs512id,
		rs256id:         rs256id,
		eddsaid:         eddsaid,
	}

	s.log.Info(ctx, "Refreshed token keys with new keys", klog.Fields{
		"token.hs512kid":        signer.hs512id,
		"token.rs256kid":        signer.rs256id,
		"token.eddsakid":        signer.eddsaid,
		"token.numjwks":         len(jwks),
		"token.numtokensigners": signingkeys.Size(),
		"token.numextsigners":   extsigningkeys.Size(),
		"token.numsysverifiers": sysverifierkeys.Size(),
	})

	m.Store(signer)

	return signer, nil
}

func (s *Service) getTokenSecrets(secrets []string, current *tokenSigner) (*hunter2.SigningKeyring, jose.Signer, string, error) {
	var khs512 hunter2.SigningKey
	signingkeys := hunter2.NewSigningKeyring()
	for _, i := range secrets {
		k, err := hunter2.SigningKeyFromParams(i, hunter2.DefaultSigningKeyAlgs)
		if err != nil {
			return nil, nil, "", kerrors.WithKind(err, governor.ErrorInvalidConfig{}, "Invalid key param")
		}
		switch k.Alg() {
		case hunter2.SigningAlgHS512:
			if khs512 == nil {
				khs512 = k
			}
		}
		if current != nil && khs512 != nil && khs512.ID() == current.hs512id {
			// first signing key matches current signing key, therefore no change in
			// signing keys
			return current.signingkeys, current.signer, current.hs512id, nil
		}
		signingkeys.RegisterSigningKey(k)
	}
	if khs512 == nil {
		return nil, nil, "", kerrors.WithKind(nil, governor.ErrorInvalidConfig{}, "No token keys present")
	}
	sig, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.HS512, Key: khs512.Private()}, (&jose.SignerOptions{}).WithType(jwtHeaderJWT).WithHeader(jwtHeaderKid, khs512.ID()))
	if err != nil {
		return nil, nil, "", kerrors.WithKind(err, ErrorSigner{}, "Failed to create new jwt HS512 signer")
	}
	return signingkeys, sig, khs512.ID(), nil
}

func (s *Service) getExtSecrets(secrets []string, current *tokenSigner) (*hunter2.SigningKeyring, jose.Signer, string, []jose.JSONWebKey, error) {
	var krs256 hunter2.SigningKey
	signingkeys := hunter2.NewSigningKeyring()
	var jwks []jose.JSONWebKey
	for _, i := range secrets {
		k, err := hunter2.SigningKeyFromParams(i, hunter2.DefaultSigningKeyAlgs)
		if err != nil {
			return nil, nil, "", nil, kerrors.WithKind(err, governor.ErrorInvalidConfig{}, "Invalid key param")
		}
		switch k.Alg() {
		case hunter2.SigningAlgRS256:
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
		if current != nil && krs256 != nil && krs256.ID() == current.rs256id {
			// first signing key matches current signing key, therefore no change in
			// signing keys
			return current.extsigningkeys, current.extsigner, current.rs256id, current.jwks, nil
		}
		signingkeys.RegisterSigningKey(k)
	}
	if krs256 == nil {
		return nil, nil, "", nil, kerrors.WithKind(nil, governor.ErrorInvalidConfig{}, "No token keys present")
	}
	extsig, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: krs256.Private()}, (&jose.SignerOptions{}).WithType(jwtHeaderJWT).WithHeader(jwtHeaderKid, krs256.ID()))
	if err != nil {
		return nil, nil, "", nil, kerrors.WithKind(err, ErrorSigner{}, "Failed to create new jwt RS256 signer")
	}
	return signingkeys, extsig, krs256.ID(), jwks, nil
}

func (s *Service) getSysVerifierSecrets(secrets []string, current *tokenSigner) (*hunter2.VerifierKeyring, string, error) {
	var keddsa hunter2.VerifierKey
	sysverifierkeys := hunter2.NewVerifierKeyring()
	for _, i := range secrets {
		k, err := hunter2.VerifierKeyFromParams(i, hunter2.DefaultVerifierKeyAlgs)
		if err != nil {
			return nil, "", kerrors.WithKind(err, governor.ErrorInvalidConfig{}, "Invalid key param")
		}
		switch k.Alg() {
		case hunter2.SigningAlgEdDSA:
			if keddsa == nil {
				keddsa = k
			}
		}
		if current != nil && keddsa != nil && keddsa.ID() == current.eddsaid {
			// first verifier key matches current verifier key, therefore no change
			// in verifier keys
			return current.sysverifierkeys, current.eddsaid, nil
		}
		sysverifierkeys.RegisterVerifierKey(k)
	}
	return sysverifierkeys, keddsa.ID(), nil
}

func (s *Service) closeSecrets(ctx context.Context, signer *tokenSigner) {
	// nothing to close
}

func (s *Service) getSigner(ctx context.Context) (*tokenSigner, error) {
	if signer := s.lc.Load(ctx); signer != nil {
		return signer, nil
	}

	return s.lc.Construct(ctx)
}

func (s *Service) Start(ctx context.Context) error {
	return nil
}

func (s *Service) Stop(ctx context.Context) {
	if err := s.wg.Wait(ctx); err != nil {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to stop"), nil)
	}
}

func (s *Service) Setup(ctx context.Context, req governor.ReqSetup) error {
	return nil
}

func (s *Service) Health(ctx context.Context) error {
	if s.lc.Load(ctx) == nil {
		return kerrors.WithKind(nil, governor.ErrorInvalidConfig{}, "Token service not ready")
	}
	return nil
}

// GetJWKS returns an RFC 7517 representation of the public signing key
func (s *Service) GetJWKS(ctx context.Context) (*jose.JSONWebKeySet, error) {
	signer, err := s.getSigner(ctx)
	if err != nil {
		return nil, err
	}
	return &jose.JSONWebKeySet{
		Keys: signer.jwks,
	}, nil
}

// Generate returns a new jwt token from a user model
func (s *Service) Generate(ctx context.Context, kind Kind, userid string, duration time.Duration, id string, authTime int64, scope string, key string) (string, *Claims, error) {
	signer, err := s.getSigner(ctx)
	if err != nil {
		return "", nil, err
	}
	now := time.Now().Round(0).UTC()
	claims := Claims{
		Claims: jwt.Claims{
			Issuer:    s.issuer,
			Subject:   userid,
			Audience:  []string{s.audience},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Expiry:    jwt.NewNumericDate(now.Add(duration)),
			ID:        id,
		},
		Kind:     kind,
		AuthTime: authTime,
		Scope:    scope,
		Key:      key,
	}
	token, err := jwt.Signed(signer.signer).Claims(claims).CompactSerialize()
	if err != nil {
		return "", nil, kerrors.WithKind(err, ErrorGenerate{}, "Failed to generate a new jwt token")
	}
	return token, &claims, nil
}

// GenerateExt creates a new id token
func (s *Service) GenerateExt(ctx context.Context, kind Kind, issuer string, userid string, audience []string, duration time.Duration, id string, authTime int64, claims interface{}) (string, error) {
	signer, err := s.getSigner(ctx)
	if err != nil {
		return "", err
	}
	now := time.Now().Round(0).UTC()
	baseClaims := Claims{
		Claims: jwt.Claims{
			Issuer:    issuer,
			Subject:   userid,
			Audience:  audience,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Expiry:    jwt.NewNumericDate(now.Add(duration)),
			ID:        id,
		},
		Kind:     kind,
		AuthTime: authTime,
	}
	token, err := jwt.Signed(signer.extsigner).Claims(baseClaims).Claims(claims).CompactSerialize()
	if err != nil {
		return "", kerrors.WithKind(err, ErrorGenerate{}, "Failed to generate a new jwt token")
	}
	return token, nil
}

// HasScope returns if a token scope contains a scope
func HasScope(tokenScope string, scope string) bool {
	if scope == "" {
		return true
	}
	if scope == ScopeForbidden {
		return false
	}
	for _, i := range strings.Fields(tokenScope) {
		if i == ScopeAll || i == scope {
			return true
		}
	}
	return false
}

func (s *tokenSigner) getPubKey(kind Kind, keyid string) (crypto.PublicKey, bool) {
	if kind == KindSystem {
		if key, ok := s.sysverifierkeys.Get(keyid); ok {
			return key.Public(), true
		}
		return nil, false
	} else {
		if key, ok := s.signingkeys.Get(keyid); ok {
			return key.Public(), true
		}
		if key, ok := s.extsigningkeys.Get(keyid); ok {
			return key.Public(), true
		}
		return nil, false
	}
}

// Validate returns whether a token is valid
func (s *Service) Validate(ctx context.Context, kind Kind, tokenString string) (bool, *Claims) {
	token, err := jwt.ParseSigned(tokenString)
	if err != nil {
		return false, nil
	}
	if len(token.Headers) != 1 {
		return false, nil
	}
	signer, err := s.getSigner(ctx)
	if err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to get signer keys"), nil)
		return false, nil
	}
	pubkey, ok := signer.getPubKey(kind, token.Headers[0].KeyID)
	if !ok {
		return false, nil
	}
	claims := &Claims{}
	if err := token.Claims(pubkey, claims); err != nil {
		return false, nil
	}
	if claims.Kind != kind {
		return false, nil
	}
	now := time.Now().Round(0).UTC()
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
func (s *Service) GetClaims(ctx context.Context, kind Kind, tokenString string) (bool, *Claims) {
	token, err := jwt.ParseSigned(tokenString)
	if err != nil {
		return false, nil
	}
	if len(token.Headers) != 1 {
		return false, nil
	}
	signer, err := s.getSigner(ctx)
	if err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to get signer keys"), nil)
		return false, nil
	}
	pubkey, ok := signer.getPubKey(kind, token.Headers[0].KeyID)
	if !ok {
		return false, nil
	}
	claims := &Claims{}
	if err := token.Claims(pubkey, claims); err != nil {
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
func (s *Service) GetClaimsExt(ctx context.Context, kind Kind, tokenString string, audience []string, claims interface{}) (bool, *Claims) {
	token, err := jwt.ParseSigned(tokenString)
	if err != nil {
		return false, nil
	}
	if len(token.Headers) != 1 {
		return false, nil
	}
	signer, err := s.getSigner(ctx)
	if err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to get signer keys"), nil)
		return false, nil
	}
	pubkey, ok := signer.getPubKey(kind, token.Headers[0].KeyID)
	if !ok {
		return false, nil
	}
	if claims == nil {
		claims = &struct{}{}
	}
	baseClaims := &Claims{}
	if err := token.Claims(pubkey, baseClaims, claims); err != nil {
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
