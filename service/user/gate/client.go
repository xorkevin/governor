package gate

import (
	"io/fs"
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/token"
	"xorkevin.dev/governor/util/ksync"
	"xorkevin.dev/kerrors"
)

type (
	// Client is a gate client
	Client interface {
		GetSysToken() (string, error)
		AddSysToken(r *http.Request) error
	}

	clientConfig struct {
		systokenfile string
	}

	// CmdClient is a gate cmd client
	CmdClient struct {
		once         *ksync.Once[clientConfig]
		systokenonce *ksync.Once[string]
		config       governor.ConfigValueReader
		term         governor.Term
	}
)

// NewCmdClient creates a new [*CmdClient]
func NewCmdClient() *CmdClient {
	return &CmdClient{
		once:         ksync.NewOnce[clientConfig](),
		systokenonce: ksync.NewOnce[string](),
	}
}

func (c *CmdClient) Register(r governor.ConfigRegistrar, cr governor.CmdRegistrar) {
	r.SetDefault("systokenfile", "")
}

func (c *CmdClient) Init(r governor.ClientConfigReader, kit governor.ClientKit) error {
	c.config = r
	c.term = kit.Term
	return nil
}

func (c *CmdClient) initConfig() (*clientConfig, error) {
	return c.once.Do(func() (*clientConfig, error) {
		return &clientConfig{
			systokenfile: c.config.GetStr("systokenfile"),
		}, nil
	})
}

func (c *clientConfig) getSysTokenFile() (string, error) {
	if c.systokenfile == "" {
		return "", kerrors.WithKind(nil, governor.ErrInvalidConfig, "Systoken file is not set")
	}
	return c.systokenfile, nil
}

func (c *CmdClient) GetSysToken() (string, error) {
	s, err := c.systokenonce.Do(func() (*string, error) {
		cc, err := c.initConfig()
		if err != nil {
			return nil, err
		}
		fp, err := cc.getSysTokenFile()
		if err != nil {
			return nil, err
		}
		b, err := fs.ReadFile(c.term.FS(), fp)
		if err != nil {
			return nil, kerrors.WithMsg(err, "Failed to read systoken file")
		}
		s := string(b)
		return &s, nil
	})
	if err != nil {
		return "", err
	}
	return *s, nil
}

func (c *CmdClient) AddSysToken(r *http.Request) error {
	s, err := c.GetSysToken()
	if err != nil {
		return err
	}
	r.SetBasicAuth(token.KeyIDSystem, s)
	return nil
}
