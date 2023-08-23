package token

import (
	"bufio"
	"io/fs"
	"time"

	"gopkg.in/square/go-jose.v2"
	"gopkg.in/square/go-jose.v2/jwt"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/util/ksync"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/hunter2/h2signer"
	"xorkevin.dev/hunter2/h2signer/eddsa"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/kfs"
	"xorkevin.dev/klog"
)

type (
	clientConfig struct {
		issuer   string
		audience string
	}

	// CmdClient is a token cmd client
	CmdClient struct {
		once          *ksync.Once[clientConfig]
		signingAlgs   h2signer.SigningKeyAlgs
		config        governor.ConfigValueReader
		log           *klog.LevelLogger
		term          governor.Term
		sysTokenFlags sysTokenFlags
	}

	sysTokenFlags struct {
		privkey   string
		subject   string
		expirestr string
		scope     string
		output    string
	}
)

// NewCmdClient creates a new [*CmdClient]
func NewCmdClient() *CmdClient {
	signingAlgs := h2signer.NewSigningKeysMap()
	eddsa.RegisterSigner(signingAlgs)
	return &CmdClient{
		once:        ksync.NewOnce[clientConfig](),
		signingAlgs: signingAlgs,
	}
}

func (c *CmdClient) Register(r governor.ConfigRegistrar, cr governor.CmdRegistrar) {
	r.SetDefault("issuer", "governor")
	r.SetDefault("audience", "governor")

	cr.Register(governor.CmdDesc{
		Usage: "gen-sys",
		Short: "generates a system token",
		Long:  "generates a system token",
		Flags: []governor.CmdFlag{
			{
				Long:     "key",
				Short:    "i",
				Usage:    "token private key",
				Required: true,
				Value:    &c.sysTokenFlags.privkey,
			},
			{
				Long:     "output",
				Short:    "o",
				Usage:    "token output file",
				Required: false,
				Value:    &c.sysTokenFlags.output,
			},
			{
				Long:     "subject",
				Short:    "u",
				Usage:    "token subject",
				Required: true,
				Value:    &c.sysTokenFlags.subject,
			},
			{
				Long:     "expire",
				Short:    "t",
				Usage:    "token expiration",
				Required: false,
				Default:  "1h",
				Value:    &c.sysTokenFlags.expirestr,
			},
			{
				Long:     "scope",
				Short:    "s",
				Usage:    "token scope",
				Required: false,
				Default:  ScopeAll,
				Value:    &c.sysTokenFlags.scope,
			},
		},
	}, governor.CmdHandlerFunc(c.genSysToken))
}

func (c *CmdClient) Init(r governor.ClientConfigReader, log klog.Logger, term governor.Term, m governor.HTTPClient) error {
	c.config = r
	c.log = klog.NewLevelLogger(log)
	c.term = term
	return nil
}

func (c *CmdClient) initConfig() (*clientConfig, error) {
	return c.once.Do(func() (*clientConfig, error) {
		cc := &clientConfig{
			issuer:   c.config.GetStr("issuer"),
			audience: c.config.GetStr("audience"),
		}
		if cc.issuer == "" {
			return nil, kerrors.WithKind(nil, governor.ErrInvalidConfig, "Token issuer is not set")
		}
		if cc.audience == "" {
			return nil, kerrors.WithKind(nil, governor.ErrInvalidConfig, "Token audience is not set")
		}
		return cc, nil
	})
}

func (c *CmdClient) genSysToken(args []string) error {
	cc, err := c.initConfig()
	if err != nil {
		return err
	}
	expire, err := time.ParseDuration(c.sysTokenFlags.expirestr)
	if err != nil {
		return kerrors.WithMsg(err, "Invalid token expiration")
	}
	skb, err := fs.ReadFile(c.term.FS(), c.sysTokenFlags.privkey)
	if err != nil {
		return kerrors.WithMsg(err, "Failed to read private key file")
	}
	key, err := h2signer.SigningKeyFromParams(string(skb), c.signingAlgs)
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse private key")
	}
	if key.Alg() != eddsa.SigID {
		return kerrors.WithMsg(nil, "Invalid private key signature algorithm")
	}
	sig, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.EdDSA, Key: key.Private()}, (&jose.SignerOptions{}).WithType(jwtHeaderJWT).WithHeader(jwtHeaderKid, key.ID()))
	if err != nil {
		return kerrors.WithMsg(err, "Failed to create signer")
	}
	u, err := uid.NewSnowflake(8)
	if err != nil {
		return kerrors.WithMsg(err, "Failed to generate token id")
	}
	now := time.Now().Round(0).UTC()
	claims := Claims{
		Claims: jwt.Claims{
			Issuer:    cc.issuer,
			Subject:   c.sysTokenFlags.subject,
			Audience:  []string{cc.audience},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Expiry:    jwt.NewNumericDate(now.Add(expire)),
			ID:        u.Base32(),
		},
		Kind:     KindSystem,
		AuthTime: now.Unix(),
		Scope:    c.sysTokenFlags.scope,
		Key:      "",
	}
	token, err := jwt.Signed(sig).Claims(claims).CompactSerialize()
	if c.sysTokenFlags.output != "" {
		if err := kfs.WriteFile(c.term.FS(), c.sysTokenFlags.output, []byte(token+"\n"), 0o600); err != nil {
			return kerrors.WithMsg(err, "Failed to write token output to file")
		}
		return nil
	}
	out := bufio.NewWriter(c.term.Stdout())
	if _, err := out.WriteString(token + "\n"); err != nil {
		return kerrors.WithMsg(err, "Failed to write token output")
	}
	if err := out.Flush(); err != nil {
		return kerrors.WithMsg(err, "Failed to write token output")
	}
	return nil
}
