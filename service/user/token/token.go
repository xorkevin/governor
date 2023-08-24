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
	"xorkevin.dev/hunter2/h2signer"
	"xorkevin.dev/hunter2/h2signer/eddsa"
	"xorkevin.dev/hunter2/h2signer/hs512"
	"xorkevin.dev/hunter2/h2signer/rs256"
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
		signingkeys     *h2signer.SigningKeyring
		extsigningkeys  *h2signer.SigningKeyring
		sysverifierkeys *h2signer.VerifierKeyring
		jwks            []jose.JSONWebKey
		hs512id         string
		rs256id         string
		eddsaid         string
	}

	Service struct {
		lc           *lifecycle.Lifecycle[tokenSigner]
		issuer       string
		audience     string
		signingAlgs  h2signer.SigningKeyAlgs
		verifierAlgs h2signer.VerifierKeyAlgs
		config       governor.SecretReader
		log          *klog.LevelLogger
		hbfailed     int
		hbmaxfail    int
		keyrefresh   time.Duration
		wg           *ksync.WaitGroup
	}
)

// New creates a new Tokenizer
func New() *Service {
	signingAlgs := h2signer.NewSigningKeysMap()
	hs512.Register(signingAlgs)
	rs256.Register(signingAlgs)
	eddsa.RegisterSigner(signingAlgs)
	verifierAlgs := h2signer.NewVerifierKeysMap()
	eddsa.RegisterVerifier(verifierAlgs)
	return &Service{
		signingAlgs:  signingAlgs,
		verifierAlgs: verifierAlgs,
		hbfailed:     0,
		wg:           ksync.NewWaitGroup(),
	}
}

func (s *Service) Register(r governor.ConfigRegistrar) {
	r.SetDefault("tokensecret", "")
	r.SetDefault("issuer", "governor")
	r.SetDefault("audience", "governor")
	r.SetDefault("hbinterval", "5s")
	r.SetDefault("hbmaxfail", 6)
	r.SetDefault("keyrefresh", "1m")
}

func (s *Service) Init(ctx context.Context, r governor.ConfigReader, kit governor.ServiceKit) error {
	s.log = klog.NewLevelLogger(kit.Logger)
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

	s.log.Info(ctx, "Loaded config",
		klog.AString("issuer", issuer),
		klog.AString("audience", audience),
		klog.AString("hbinterval", hbinterval.String()),
		klog.AInt("hbmaxfail", s.hbmaxfail),
		klog.AString("keyrefresh", s.keyrefresh.String()),
	)

	ctx = klog.CtxWithAttrs(ctx, klog.AString("gov.phase", "run"))

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
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to refresh token keys"))
		return
	}
	s.log.Err(ctx, kerrors.WithMsg(err, "Failed max refresh attempts"))
	s.hbfailed = 0
	// clear the cached signer because its secret may be invalid
	m.Stop(ctx)
}

var (
	// ErrSigner is returned when failing to create a signer
	ErrSigner errSigner
	// ErrGenerate is returned when failing to generate a token
	ErrGenerate errGenerate
)

type (
	errSigner   struct{}
	errGenerate struct{}
)

func (e errSigner) Error() string {
	return "Error creating signer"
}

func (e errGenerate) Error() string {
	return "Error generating token"
}

type (
	secretToken struct {
		Secrets    []string `mapstructure:"secrets"`
		ExtKeys    []string `mapstructure:"extkeys"`
		SysPubKeys []string `mapstructure:"syspubkeys"`
	}
)

func (s *Service) getSecrets(ctx context.Context, m *lifecycle.State[tokenSigner]) (*tokenSigner, error) {
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

	s.log.Info(ctx, "Refreshed token keys with new keys",
		klog.AString("hs512kid", signer.hs512id),
		klog.AString("rs256kid", signer.rs256id),
		klog.AString("eddsakid", signer.eddsaid),
		klog.AInt("numjwks", len(jwks)),
		klog.AInt("numtokensigners", signingkeys.Size()),
		klog.AInt("numextsigners", extsigningkeys.Size()),
		klog.AInt("numsysverifiers", sysverifierkeys.Size()),
	)

	m.Store(signer)

	return signer, nil
}

func (s *Service) getTokenSecrets(secrets []string, current *tokenSigner) (*h2signer.SigningKeyring, jose.Signer, string, error) {
	var khs512 h2signer.SigningKey
	signingkeys := h2signer.NewSigningKeyring()
	for _, i := range secrets {
		k, err := h2signer.SigningKeyFromParams(i, s.signingAlgs)
		if err != nil {
			return nil, nil, "", kerrors.WithKind(err, governor.ErrInvalidConfig, "Invalid key param")
		}
		switch k.Alg() {
		case hs512.SigID:
			if khs512 == nil {
				khs512 = k
			}
		}
		if current != nil && khs512 != nil && khs512.ID() == current.hs512id {
			// first signing key matches current signing key, therefore no change in
			// signing keys
			return current.signingkeys, current.signer, current.hs512id, nil
		}
		signingkeys.Register(k)
	}
	if khs512 == nil {
		return nil, nil, "", kerrors.WithKind(nil, governor.ErrInvalidConfig, "No token keys present")
	}
	sig, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.HS512, Key: khs512.Private()}, (&jose.SignerOptions{}).WithType(jwtHeaderJWT).WithHeader(jwtHeaderKid, khs512.ID()))
	if err != nil {
		return nil, nil, "", kerrors.WithKind(err, ErrSigner, "Failed to create new jwt HS512 signer")
	}
	return signingkeys, sig, khs512.ID(), nil
}

func (s *Service) getExtSecrets(secrets []string, current *tokenSigner) (*h2signer.SigningKeyring, jose.Signer, string, []jose.JSONWebKey, error) {
	var krs256 h2signer.SigningKey
	signingkeys := h2signer.NewSigningKeyring()
	var jwks []jose.JSONWebKey
	for _, i := range secrets {
		k, err := h2signer.SigningKeyFromParams(i, s.signingAlgs)
		if err != nil {
			return nil, nil, "", nil, kerrors.WithKind(err, governor.ErrInvalidConfig, "Invalid key param")
		}
		switch k.Alg() {
		case rs256.SigID:
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
		signingkeys.Register(k)
	}
	if krs256 == nil {
		return nil, nil, "", nil, kerrors.WithKind(nil, governor.ErrInvalidConfig, "No token keys present")
	}
	extsig, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: krs256.Private()}, (&jose.SignerOptions{}).WithType(jwtHeaderJWT).WithHeader(jwtHeaderKid, krs256.ID()))
	if err != nil {
		return nil, nil, "", nil, kerrors.WithKind(err, ErrSigner, "Failed to create new jwt RS256 signer")
	}
	return signingkeys, extsig, krs256.ID(), jwks, nil
}

func (s *Service) getSysVerifierSecrets(secrets []string, current *tokenSigner) (*h2signer.VerifierKeyring, string, error) {
	var keddsa h2signer.VerifierKey
	sysverifierkeys := h2signer.NewVerifierKeyring()
	for _, i := range secrets {
		k, err := h2signer.VerifierKeyFromParams(i, s.verifierAlgs)
		if err != nil {
			return nil, "", kerrors.WithKind(err, governor.ErrInvalidConfig, "Invalid key param")
		}
		switch k.Alg() {
		case eddsa.SigID:
			if keddsa == nil {
				keddsa = k
			}
		}
		if current != nil && keddsa != nil && keddsa.ID() == current.eddsaid {
			// first verifier key matches current verifier key, therefore no change
			// in verifier keys
			return current.sysverifierkeys, current.eddsaid, nil
		}
		sysverifierkeys.Register(k)
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
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to stop"))
	}
}

func (s *Service) Setup(ctx context.Context, req governor.ReqSetup) error {
	return nil
}

func (s *Service) Health(ctx context.Context) error {
	if s.lc.Load(ctx) == nil {
		return kerrors.WithKind(nil, governor.ErrInvalidConfig, "Token service not ready")
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
		return "", nil, kerrors.WithKind(err, ErrGenerate, "Failed to generate a new jwt token")
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
		return "", kerrors.WithKind(err, ErrGenerate, "Failed to generate a new jwt token")
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
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to get signer keys"))
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
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to get signer keys"))
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
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to get signer keys"))
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
