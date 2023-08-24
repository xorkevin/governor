package gate

import (
	"context"
	"crypto"
	"time"

	"github.com/go-jose/go-jose/v3"
	"github.com/go-jose/go-jose/v3/jwt"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/util/ksync"
	"xorkevin.dev/governor/util/lifecycle"
	"xorkevin.dev/hunter2/h2signer"
	"xorkevin.dev/hunter2/h2signer/eddsa"
	"xorkevin.dev/hunter2/h2signer/rs256"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

const (
	// CookieNameAccessToken is the name of the access token cookie
	CookieNameAccessToken = "access_token"
)

const (
	// ScopeAll grants all scopes to a token
	ScopeAll = "all"
	// ScopeForbidden denies all access
	ScopeForbidden = "forbidden"
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
	// kindOpenID is an openid id token kind
	kindOpenID = "openid"
)

const (
	jwtHeaderKid = "kid"
	jwtHeaderJWT = "JWT"
)

type (
	Claims struct {
		Issuer    string   `json:"iss,omitempty"`
		Subject   string   `json:"sub,omitempty"`
		Audience  []string `json:"aud,omitempty"`
		Expiry    int64    `json:"exp,omitempty"`
		NotBefore int64    `json:"nbf,omitempty"`
		IssuedAt  int64    `json:"iat,omitempty"`
		ID        string   `json:"jti,omitempty"`
		// Custom fields
		Kind      Kind   `json:"k,omitempty"`
		SessionID string `json:"sid,omitempty"`
		AuthAt    int64  `json:"aat,omitempty"`
		Scope     string `json:"scope,omitempty"`
		Key       string `json:"key,omitempty"`
	}

	ctxKeyUserid struct{}
	ctxKeyClaims struct{}
)

// GetCtxUserid returns a userid from the context
func GetCtxUserid(c *governor.Context) string {
	v := c.Get(ctxKeyUserid{})
	if v == nil {
		return ""
	}
	return v.(string)
}

func setCtxUserid(c *governor.Context, userid string) {
	c.Set(ctxKeyUserid{}, userid)
	c.LogAttrs(klog.AString("gate.userid", userid))
}

// GetCtxClaims returns token claims from the context
func GetCtxClaims(c *governor.Context) *Claims {
	v := c.Get(ctxKeyClaims{})
	if v == nil {
		return nil
	}
	return v.(*Claims)
}

func setCtxClaims(c *governor.Context, claims *Claims) {
	c.Set(ctxKeyUserid{}, claims.Subject)
	c.Set(ctxKeyClaims{}, claims)
	c.LogAttrs(
		klog.AString("gate.userid", claims.Subject),
		klog.AString("gate.sessionid", claims.ID),
	)
}

type (
	Gate interface{}

	tokenSigner struct {
		signer         jose.Signer
		extsigner      jose.Signer
		signingkeys    *h2signer.SigningKeyring
		extsigningkeys *h2signer.SigningKeyring
		jwks           []jose.JSONWebKey
		eddsaid        string
		rs256id        string
	}

	Service struct {
		lc           *lifecycle.Lifecycle[tokenSigner]
		issuer       string
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
	eddsa.RegisterSigner(signingAlgs)
	rs256.Register(signingAlgs)
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
	r.SetDefault("hbinterval", "5s")
	r.SetDefault("hbmaxfail", 6)
	r.SetDefault("keyrefresh", "1m")
}

func (s *Service) Init(ctx context.Context, r governor.ConfigReader, kit governor.ServiceKit) error {
	s.log = klog.NewLevelLogger(kit.Logger)
	s.config = r

	s.issuer = r.GetStr("issuer")
	if s.issuer == "" {
		return kerrors.WithMsg(nil, "Token issuer is not set")
	}

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
		klog.AString("issuer", s.issuer),
		klog.AString("hbinterval", hbinterval.String()),
		klog.AInt("hbmaxfail", s.hbmaxfail),
		klog.AString("keyrefresh", s.keyrefresh.String()),
	)

	ctx = klog.CtxWithAttrs(ctx, klog.AString("gov.phase", "run"))

	s.lc = lifecycle.New(
		ctx,
		s.getKeys,
		s.closeKeys,
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
		Keys    []string `mapstructure:"keys"`
		ExtKeys []string `mapstructure:"extkeys"`
	}
)

func (s *Service) getKeys(ctx context.Context, m *lifecycle.State[tokenSigner]) (*tokenSigner, error) {
	currentSigner := m.Load(ctx)
	var tokenSecrets secretToken
	if err := s.config.GetSecret(ctx, "tokensecret", s.keyrefresh, &tokenSecrets); err != nil {
		return nil, kerrors.WithMsg(err, "Invalid token secret")
	}
	signingkeys, sig, eddsaid, err := s.getSigningKeys(tokenSecrets.Keys, currentSigner)
	if err != nil {
		return nil, err
	}
	extsigningkeys, extsig, rs256id, jwks, err := s.getExtSigningKeys(tokenSecrets.ExtKeys, currentSigner)
	if err != nil {
		return nil, err
	}
	if currentSigner != nil && eddsaid == currentSigner.eddsaid && rs256id == currentSigner.rs256id {
		return currentSigner, nil
	}

	m.Stop(ctx)

	signer := &tokenSigner{
		signer:         sig,
		extsigner:      extsig,
		signingkeys:    signingkeys,
		extsigningkeys: extsigningkeys,
		jwks:           jwks,
		eddsaid:        eddsaid,
		rs256id:        rs256id,
	}

	s.log.Info(ctx, "Refreshed token keys with new keys",
		klog.AString("eddsakid", signer.eddsaid),
		klog.AString("rs256kid", signer.rs256id),
		klog.AInt("numjwks", len(jwks)),
		klog.AInt("numtokensigners", signingkeys.Size()),
		klog.AInt("numextsigners", extsigningkeys.Size()),
	)

	m.Store(signer)

	return signer, nil
}

func (s *Service) getSigningKeys(keys []string, current *tokenSigner) (*h2signer.SigningKeyring, jose.Signer, string, error) {
	var keddsa h2signer.SigningKey
	signingkeys := h2signer.NewSigningKeyring()
	for _, i := range keys {
		k, err := h2signer.SigningKeyFromParams(i, s.signingAlgs)
		if err != nil {
			return nil, nil, "", kerrors.WithKind(err, governor.ErrInvalidConfig, "Invalid key param")
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
			return current.signingkeys, current.signer, current.eddsaid, nil
		}
		signingkeys.Register(k)
	}
	sig, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.EdDSA, Key: keddsa.Private()}, (&jose.SignerOptions{}).WithType(jwtHeaderJWT).WithHeader(jwtHeaderKid, keddsa.ID()))
	if err != nil {
		return nil, nil, "", kerrors.WithKind(err, ErrSigner, "Failed to create new jwt HS512 signer")
	}
	return signingkeys, sig, keddsa.ID(), nil
}

func (s *Service) getExtSigningKeys(keys []string, current *tokenSigner) (*h2signer.SigningKeyring, jose.Signer, string, []jose.JSONWebKey, error) {
	var krs256 h2signer.SigningKey
	signingkeys := h2signer.NewSigningKeyring()
	var jwks []jose.JSONWebKey
	for _, i := range keys {
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

func (s *Service) closeKeys(ctx context.Context, signer *tokenSigner) {
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
func (s *Service) Generate(ctx context.Context, claims Claims, duration time.Duration) (string, error) {
	if claims.Kind != KindAccess && claims.Kind != KindRefresh {
		return "", kerrors.WithKind(nil, ErrGenerate, "Invalid token kind")
	}
	signer, err := s.getSigner(ctx)
	if err != nil {
		return "", err
	}
	now := time.Now().Round(0).UTC()
	claims.IssuedAt = now.Unix()
	claims.Expiry = now.Add(duration).Unix()
	token, err := jwt.Signed(signer.signer).Claims(claims).CompactSerialize()
	if err != nil {
		return "", kerrors.WithKind(err, ErrGenerate, "Failed to generate a new jwt token")
	}
	return token, nil
}

// GenerateExt creates a new id token
func (s *Service) GenerateExt(ctx context.Context, baseClaims Claims, duration time.Duration, otherClaims interface{}) (string, error) {
	signer, err := s.getSigner(ctx)
	if err != nil {
		return "", err
	}
	now := time.Now().Round(0).UTC()
	baseClaims.Kind = kindOpenID
	baseClaims.Issuer = s.issuer
	baseClaims.IssuedAt = now.Unix()
	baseClaims.Expiry = now.Add(duration).Unix()
	token, err := jwt.Signed(signer.extsigner).Claims(baseClaims).Claims(otherClaims).CompactSerialize()
	if err != nil {
		return "", kerrors.WithKind(err, ErrGenerate, "Failed to generate a new jwt token")
	}
	return token, nil
}

func (s *tokenSigner) getPubKey(kind Kind, keyid string) (crypto.PublicKey, bool) {
	switch kind {
	case KindAccess, KindRefresh:
		if key, ok := s.signingkeys.Get(keyid); ok {
			return key.Public(), true
		}
	case kindOpenID:
		if key, ok := s.extsigningkeys.Get(keyid); ok {
			return key.Public(), true
		}
	}
	return nil, false
}

func (s *Service) Validate(ctx context.Context, kind Kind, tokenString string) (bool, *Claims) {
	if kind == kindOpenID {
		return false, nil
	}
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
	var jwtclaims jwt.Claims
	if err := token.Claims(pubkey, claims, &jwtclaims); err != nil {
		return false, nil
	}
	if claims.Kind != kind {
		return false, nil
	}
	now := time.Now().Round(0).UTC()
	if err := jwtclaims.ValidateWithLeeway(jwt.Expected{
		Time: now,
	}, 0); err != nil {
		return false, nil
	}
	return true, claims
}

func (s *Service) ValidateExt(ctx context.Context, tokenString string, otherClaims interface{}) (bool, *Claims) {
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
	pubkey, ok := signer.getPubKey(kindOpenID, token.Headers[0].KeyID)
	if !ok {
		return false, nil
	}
	claims := &Claims{}
	var jwtclaims jwt.Claims
	if otherClaims == nil {
		var empty struct{}
		otherClaims = &empty
	}
	if err := token.Claims(pubkey, claims, &jwtclaims, otherClaims); err != nil {
		return false, nil
	}
	if claims.Kind != kindOpenID {
		return false, nil
	}
	now := time.Now().Round(0).UTC()
	if err := jwtclaims.ValidateWithLeeway(jwt.Expected{
		Issuer: s.issuer,
		Time:   now,
	}, 0); err != nil {
		return false, nil
	}
	return true, claims
}
