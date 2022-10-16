package user

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"golang.org/x/term"
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
		addAdminFlags reqAddAdmin
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
				Value:    &c.addAdminFlags.Username,
			},
			{
				Long:     "password",
				Short:    "p",
				Usage:    "password",
				Required: false,
				Value:    &c.addAdminFlags.Password,
			},
			{
				Long:     "email",
				Short:    "m",
				Usage:    "email",
				Required: true,
				Value:    &c.addAdminFlags.Email,
			},
			{
				Long:     "firstname",
				Short:    "",
				Usage:    "user first name",
				Required: true,
				Value:    &c.addAdminFlags.Firstname,
			},
			{
				Long:     "lastname",
				Short:    "",
				Usage:    "user last name",
				Required: true,
				Value:    &c.addAdminFlags.Lastname,
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
	if c.addAdminFlags.Password == "-" {
		var err error
		c.addAdminFlags.Password, err = c.cli.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return kerrors.WithMsg(err, "Failed reading user password")
		}
	}
	if err := c.addAdminFlags.valid(); err != nil {
		return err
	}
	r, err := c.http.NewJSONRequest(http.MethodPost, "/user/admin", c.addAdminFlags)
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
		return kerrors.WithKind(nil, governor.ErrorServerRes{}, "Non-decodable response")
	}
	log.Printf("Created admin user %s: %s\n", body.Username, body.Userid)
	return nil
}

func getAdminPromptReq() (*reqAddAdmin, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("First name: ")
	firstname, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}

	fmt.Print("Last name: ")
	lastname, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}

	fmt.Print("Username: ")
	username, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}

	fmt.Print("Email: ")
	email, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}

	fmt.Print("Password: ")
	passwordBytes, err := term.ReadPassword(0)
	if err != nil {
		return nil, err
	}
	fmt.Println()
	password := string(passwordBytes)

	fmt.Print("Verify password: ")
	passwordVerifyBytes, err := term.ReadPassword(0)
	if err != nil {
		return nil, err
	}
	fmt.Println()
	passwordVerify := string(passwordVerifyBytes)
	if password != passwordVerify {
		return nil, errors.New("Passwords do not match")
	}

	return &reqAddAdmin{
		Username:  strings.TrimSpace(username),
		Password:  password,
		Email:     strings.TrimSpace(email),
		Firstname: strings.TrimSpace(firstname),
		Lastname:  strings.TrimSpace(lastname),
	}, nil
}
