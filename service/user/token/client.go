package token

import (
	"io"
	"log"
	"os"
	"time"

	"gopkg.in/square/go-jose.v2"
	"gopkg.in/square/go-jose.v2/jwt"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/hunter2"
	"xorkevin.dev/kerrors"
)

type (
	// Client is a token cmd client
	Client struct {
		issuer   string
		audience string
	}
)

// NewClient creates a new [*Client]
func NewClient() *Client {
	return &Client{}
}

func (c *Client) initConfig(r governor.ConfigValueReader) error {
	c.issuer = r.GetStr("issuer")
	if c.issuer == "" {
		return kerrors.WithMsg(nil, "Token issuer is not set")
	}
	c.audience = r.GetStr("audience")
	if c.audience == "" {
		return kerrors.WithMsg(nil, "Token audience is not set")
	}
	return nil
}

func (c *Client) Register(r governor.ConfigRegistrar, cr governor.CmdRegistrar) {
	r.SetDefault("issuer", "governor")
	r.SetDefault("audience", "governor")

	var (
		sysprivkey  string
		syssubject  string
		sysduration string
		sysscope    string
		sysoutput   string
	)

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
				Value:    &sysprivkey,
			},
			{
				Long:     "output",
				Short:    "o",
				Usage:    "token output file",
				Required: false,
				Value:    &sysoutput,
			},
			{
				Long:     "subject",
				Short:    "u",
				Usage:    "token subject",
				Required: true,
				Value:    &syssubject,
			},
			{
				Long:     "expire",
				Short:    "t",
				Usage:    "token expiration",
				Required: false,
				Default:  "1h",
				Value:    &sysduration,
			},
			{
				Long:     "scope",
				Short:    "s",
				Usage:    "token scope",
				Required: false,
				Default:  ScopeAll,
				Value:    &sysscope,
			},
		},
	}, governor.CmdHandlerFunc(func(cc governor.ClientConfig, r governor.ConfigValueReader, args []string) {
		if err := c.initConfig(r); err != nil {
			log.Fatalln(err)
		}
		expiration, err := time.ParseDuration(sysduration)
		if err != nil {
			log.Fatalln(kerrors.WithMsg(err, "Invalid token expiration"))
		}
		if err := c.GenSysToken(sysprivkey, syssubject, expiration, sysscope, sysoutput); err != nil {
			log.Fatalln(err)
		}
	}))
}

func (c *Client) GenSysToken(sysprivkey string, subject string, expiration time.Duration, scope string, outputfile string) error {
	skb, err := func() ([]byte, error) {
		skf, err := os.Open(sysprivkey)
		if err != nil {
			return nil, kerrors.WithMsg(err, "Failed to open private key file")
		}
		defer func() {
			if err := skf.Close(); err != nil {
				log.Println(kerrors.WithMsg(err, "Failed to close private key file"))
			}
		}()
		skb, err := io.ReadAll(skf)
		if err != nil {
			return nil, kerrors.WithMsg(err, "Failed to read private key file")
		}
		return skb, nil
	}()
	if err != nil {
		return err
	}
	key, err := hunter2.SigningKeyFromParams(string(skb), hunter2.DefaultSigningKeyAlgs)
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse private key")
	}
	if key.Alg() != hunter2.SigningAlgEdDSA {
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
			Issuer:    c.issuer,
			Subject:   subject,
			Audience:  []string{c.audience},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Expiry:    jwt.NewNumericDate(now.Add(expiration)),
			ID:        u.Base32(),
		},
		Kind:     KindSystem,
		AuthTime: now.Unix(),
		Scope:    scope,
		Key:      "",
	}
	token, err := jwt.Signed(sig).Claims(claims).CompactSerialize()
	output := os.Stdout
	if outputfile != "" {
		var err error
		output, err = os.Create(outputfile)
		if err != nil {
			return kerrors.WithMsg(err, "Failed to create output file")
		}
		defer func() {
			if err := output.Close(); err != nil {
				log.Println(kerrors.WithMsg(err, "Failed to close output file"))
			}
		}()
	}
	if _, err := io.WriteString(output, token); err != nil {
		return kerrors.WithMsg(err, "Failed to write token output")
	}
	return nil
}
