package gate

import (
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"time"

	"github.com/go-jose/go-jose/v3"
	"github.com/go-jose/go-jose/v3/jwt"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/util/ksync"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/hunter2/h2signer"
	"xorkevin.dev/hunter2/h2signer/eddsa"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/kfs"
)

type (
	// Client is a gate client
	Client interface {
		GetToken() (string, error)
		AddReqToken(r *http.Request) error
	}

	clientConfig struct {
		keyfile   string
		tokenfile string
	}

	// CmdClient is a gate cmd client
	CmdClient struct {
		once        *ksync.Once[clientConfig]
		tokenonce   *ksync.Once[string]
		signingAlgs h2signer.SigningKeyAlgs
		config      governor.ConfigValueReader
		term        governor.Term
		tokenFlags  tokenFlags
		keyFlags    keyFlags
	}

	tokenFlags struct {
		privkey   string
		subject   string
		expirestr string
		scope     string
		output    string
	}

	keyFlags struct {
		privkey string
	}
)

// NewCmdClient creates a new [*CmdClient]
func NewCmdClient() *CmdClient {
	signingAlgs := h2signer.NewSigningKeysMap()
	eddsa.RegisterSigner(signingAlgs)
	return &CmdClient{
		once:        ksync.NewOnce[clientConfig](),
		tokenonce:   ksync.NewOnce[string](),
		signingAlgs: signingAlgs,
	}
}

func (c *CmdClient) Register(r governor.ConfigRegistrar, cr governor.CmdRegistrar) {
	r.SetDefault("keyfile", "")
	r.SetDefault("tokenfile", "")

	cr.Register(governor.CmdDesc{
		Usage: "gen-key",
		Short: "generates a key",
		Long:  "generates a key",
		Flags: []governor.CmdFlag{
			{
				Long:     "output",
				Short:    "o",
				Usage:    "key output file",
				Required: false,
				Value:    &c.keyFlags.privkey,
			},
		},
	}, governor.CmdHandlerFunc(c.genKey))

	cr.Register(governor.CmdDesc{
		Usage: "gen-token",
		Short: "generates a token",
		Long:  "generates a token",
		Flags: []governor.CmdFlag{
			{
				Long:     "key",
				Short:    "i",
				Usage:    "token private key",
				Required: false,
				Value:    &c.tokenFlags.privkey,
			},
			{
				Long:     "output",
				Short:    "o",
				Usage:    "token output file",
				Required: false,
				Value:    &c.tokenFlags.output,
			},
			{
				Long:     "subject",
				Short:    "u",
				Usage:    "token subject",
				Required: false,
				Default:  KeySubSystem,
				Value:    &c.tokenFlags.subject,
			},
			{
				Long:     "expire",
				Short:    "t",
				Usage:    "token expiration",
				Required: false,
				Default:  "1h",
				Value:    &c.tokenFlags.expirestr,
			},
			{
				Long:     "scope",
				Short:    "s",
				Usage:    "token scope",
				Required: false,
				Default:  ScopeAll,
				Value:    &c.tokenFlags.scope,
			},
		},
	}, governor.CmdHandlerFunc(c.genToken))
}

func (c *CmdClient) Init(r governor.ClientConfigReader, kit governor.ClientKit) error {
	c.config = r
	c.term = kit.Term
	return nil
}

func (c *CmdClient) initConfig() (*clientConfig, error) {
	return c.once.Do(func() (*clientConfig, error) {
		return &clientConfig{
			keyfile:   c.config.GetStr("keyfile"),
			tokenfile: c.config.GetStr("tokenfile"),
		}, nil
	})
}

func (c *clientConfig) getTokenFile() (string, error) {
	if c.tokenfile == "" {
		return "", kerrors.WithKind(nil, governor.ErrInvalidConfig, "Token file is not set")
	}
	return c.tokenfile, nil
}

func (c *CmdClient) GetToken() (string, error) {
	s, err := c.tokenonce.Do(func() (*string, error) {
		cc, err := c.initConfig()
		if err != nil {
			return nil, err
		}
		fp, err := cc.getTokenFile()
		if err != nil {
			return nil, err
		}
		b, err := fs.ReadFile(c.term.FS(), fp)
		if err != nil {
			return nil, kerrors.WithMsg(err, "Failed to read token file")
		}
		s := string(b)
		return &s, nil
	})
	if err != nil {
		return "", err
	}
	return *s, nil
}

func (c *CmdClient) AddReqToken(r *http.Request) error {
	s, err := c.GetToken()
	if err != nil {
		return err
	}
	r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s))
	return nil
}

func (c *CmdClient) genKey(args []string) error {
	cc, err := c.initConfig()
	if err != nil {
		return err
	}

	keyfile := c.keyFlags.privkey
	if keyfile == "" {
		keyfile = cc.keyfile
	}
	if keyfile == "" {
		return kerrors.WithMsg(err, "Invalid key output")
	}

	cfg, err := eddsa.NewConfig()
	if err != nil {
		return kerrors.WithMsg(err, "Failed to generate key")
	}
	cfgstr, err := cfg.String()
	if err != nil {
		return kerrors.WithMsg(err, "Failed to serialize key")
	}

	if keyfile == "-" {
		if _, err := io.WriteString(c.term.Stdout(), cfgstr+"\n"); err != nil {
			return kerrors.WithMsg(err, "Failed to write key to stdout")
		}
		return nil
	}
	if err := kfs.WriteFile(c.term.FS(), keyfile, []byte(cfgstr+"\n"), 0o600); err != nil {
		return kerrors.WithMsg(err, "Failed to write key to file")
	}
	return nil
}

func (c *CmdClient) genToken(args []string) error {
	cc, err := c.initConfig()
	if err != nil {
		return err
	}

	output := c.tokenFlags.output
	if output == "" {
		output = cc.tokenfile
	}
	if output == "" {
		return kerrors.WithMsg(err, "Invalid token output")
	}

	expire, err := time.ParseDuration(c.tokenFlags.expirestr)
	if err != nil {
		return kerrors.WithMsg(err, "Invalid token expiration")
	}
	skb, err := fs.ReadFile(c.term.FS(), c.tokenFlags.privkey)
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
	sig, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.EdDSA, Key: key.Private()}, (&jose.SignerOptions{}).WithType(jwtHeaderJWT).WithHeader(jwtHeaderKid, key.Verifier().ID()))
	if err != nil {
		return kerrors.WithMsg(err, "Failed to create signer")
	}
	u, err := uid.New()
	if err != nil {
		return kerrors.WithMsg(err, "Failed to generate token id")
	}
	now := time.Now().Round(0).UTC()
	claims := Claims{
		Subject:   c.tokenFlags.subject,
		Expiry:    now.Add(expire).Unix(),
		ID:        u.Base64(),
		Kind:      KindAccess,
		SessionID: u.Base64(),
		Scope:     c.tokenFlags.scope,
	}
	token, err := jwt.Signed(sig).Claims(claims).CompactSerialize()

	if output == "-" {
		if _, err := io.WriteString(c.term.Stdout(), token+"\n"); err != nil {
			return kerrors.WithMsg(err, "Failed to write token to stdout")
		}
		return nil
	}
	if err := kfs.WriteFile(c.term.FS(), output, []byte(token+"\n"), 0o600); err != nil {
		return kerrors.WithMsg(err, "Failed to write token to file")
	}
	return nil
}
