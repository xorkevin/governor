package user

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/util/ksync"
	"xorkevin.dev/kerrors"
)

type (
	clientConfig struct {
	}

	// CmdClient is a user cmd client
	CmdClient struct {
		gate          gate.Client
		once          *ksync.Once[clientConfig]
		config        governor.ConfigValueReader
		cli           governor.CLI
		http          governor.HTTPClient
		addAdminReq   reqAddAdmin
		addAdminFlags addAdminFlags
	}

	addAdminFlags struct {
		interactive bool
	}
)

// NewCmdClientCtx creates a new [*CmdClient] from a context
func NewCmdClientCtx(inj governor.Injector) *CmdClient {
	return NewCmdClient(
		gate.GetCtxClient(inj),
	)
}

// NewCmdClient creates a new [*CmdClient]
func NewCmdClient(g gate.Client) *CmdClient {
	return &CmdClient{
		gate: g,
		once: ksync.NewOnce[clientConfig](),
	}
}

func (c *CmdClient) Register(inj governor.Injector, r governor.ConfigRegistrar, cr governor.CmdRegistrar) {
	cr.Register(governor.CmdDesc{
		Usage: "add-admin",
		Short: "adds an admin",
		Long:  "adds an admin",
		Flags: []governor.CmdFlag{
			{
				Long:     "username",
				Short:    "u",
				Usage:    "username",
				Required: true,
				Value:    &c.addAdminReq.Username,
			},
			{
				Long:     "password",
				Short:    "p",
				Usage:    "password",
				Required: false,
				Value:    &c.addAdminReq.Password,
			},
			{
				Long:     "email",
				Short:    "m",
				Usage:    "email",
				Required: true,
				Value:    &c.addAdminReq.Email,
			},
			{
				Long:     "firstname",
				Short:    "",
				Usage:    "user first name",
				Required: true,
				Value:    &c.addAdminReq.Firstname,
			},
			{
				Long:     "lastname",
				Short:    "",
				Usage:    "user last name",
				Required: true,
				Value:    &c.addAdminReq.Lastname,
			},
			{
				Long:     "interactive",
				Short:    "",
				Usage:    "show interactive password prompt",
				Required: false,
				Value:    &c.addAdminFlags.interactive,
			},
		},
	}, governor.CmdHandlerFunc(c.addAdmin))
}

func (c *CmdClient) Init(gc governor.ClientConfig, r governor.ConfigValueReader, cli governor.CLI, m governor.HTTPClient) error {
	c.config = r
	c.cli = cli
	c.http = m
	return nil
}

func (c *CmdClient) addAdmin(args []string) error {
	if c.addAdminReq.Password == "-" {
		var err error
		c.addAdminReq.Password, err = c.cli.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return kerrors.WithMsg(err, "Failed reading user password")
		}
	}
	if c.addAdminFlags.interactive && c.addAdminReq.Password == "" {
		fmt.Print("Password: ")
		var err error
		c.addAdminReq.Password, err = c.cli.ReadPassword()
		if err != nil {
			return kerrors.WithMsg(err, "Failed to read password")
		}
		fmt.Print("Verify password: ")
		passwordAgain, err := c.cli.ReadPassword()
		if err != nil {
			return kerrors.WithMsg(err, "Failed to read password")
		}
		if passwordAgain != c.addAdminReq.Password {
			return kerrors.WithMsg(err, "Passwords do not match")
		}
	}
	if err := c.addAdminReq.valid(); err != nil {
		return err
	}
	r, err := c.http.NewJSONRequest(http.MethodPost, "/user/admin", c.addAdminReq)
	if err != nil {
		return kerrors.WithMsg(err, "Failed to create admin request")
	}
	if err := c.gate.AddSysToken(r); err != nil {
		return kerrors.WithMsg(err, "Failed to add systoken")
	}
	var body resUserUpdate
	_, decoded, err := c.http.DoRequestJSON(r, &body)
	if err != nil {
		return kerrors.WithMsg(err, "Failed adding admin")
	}
	if !decoded {
		return kerrors.WithKind(nil, governor.ErrorServerRes, "Non-decodable response")
	}
	log.Printf("Created admin user %s: %s\n", body.Username, body.Userid)
	return nil
}
