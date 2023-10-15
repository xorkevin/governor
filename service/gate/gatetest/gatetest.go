package gatetest

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"net/http"
	"time"

	"github.com/go-jose/go-jose/v3"
	"github.com/go-jose/go-jose/v3/jwt"
	"xorkevin.dev/governor/service/gate"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/hunter2/h2signer"
	"xorkevin.dev/hunter2/h2signer/eddsa"
	"xorkevin.dev/hunter2/h2signer/rsasig"
	"xorkevin.dev/kerrors"
)

type (
	Client struct {
		Key       h2signer.SigningKey
		KeyStr    string
		Token     string
		ExtKeyStr string
	}
)

func NewClient() (*Client, error) {
	cfg, err := eddsa.NewConfig()
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to generate key")
	}
	cfgstr, err := cfg.String()
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to generate key")
	}
	key, err := eddsa.New(*cfg)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to generate key")
	}
	rsakey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to generate rsa key")
	}
	rsaconfig := rsasig.Config{
		Key: rsakey,
	}
	rsastr, err := rsaconfig.String()
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to generate rsa key")
	}
	return &Client{
		Key:       key,
		KeyStr:    cfgstr,
		ExtKeyStr: rsastr,
	}, nil
}

func (c *Client) GenToken(subject string, expire time.Duration, scope string) (string, error) {
	sig, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.EdDSA, Key: c.Key.Private()},
		(&jose.SignerOptions{}).WithType(gate.JWTHeaderJWT).WithHeader(gate.JWTHeaderKid, c.Key.Verifier().ID()),
	)
	if err != nil {
		return "", kerrors.WithMsg(err, "Failed to create signer")
	}
	u, err := uid.New()
	if err != nil {
		return "", kerrors.WithMsg(err, "Failed to generate token id")
	}
	now := time.Now().Round(0).UTC()
	claims := gate.Claims{
		Subject:   subject,
		Expiry:    now.Add(expire).Unix(),
		ID:        u.Base64(),
		SessionID: u.Base64(),
		Scope:     scope,
	}
	token, err := jwt.Signed(sig).Claims(claims).CompactSerialize()
	if err != nil {
		return "", kerrors.WithMsg(err, "Failed to generate token")
	}
	return token, nil
}

func (c *Client) GetToken() (string, error) {
	return c.Token, nil
}

func (c *Client) AddReqToken(r *http.Request) error {
	s, err := c.GetToken()
	if err != nil {
		return err
	}
	r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s))
	return nil
}
