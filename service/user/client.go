package user

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	// CmdClient is a user cmd client
	CmdClient struct {
		gate          gate.Client
		log           *klog.LevelLogger
		term          governor.Term
		httpc         *governor.HTTPFetcher
		addAdminReq   reqAddAdmin
		addAdminFlags addAdminFlags
	}

	addAdminFlags struct {
		interactive bool
	}
)

// NewCmdClient creates a new [*CmdClient]
func NewCmdClient(g gate.Client) *CmdClient {
	return &CmdClient{
		gate: g,
	}
}

func (c *CmdClient) Register(r governor.ConfigRegistrar, cr governor.CmdRegistrar) {
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

func (c *CmdClient) Init(r governor.ClientConfigReader, log klog.Logger, term governor.Term, m governor.HTTPClient) error {
	c.log = klog.NewLevelLogger(log)
	c.term = term
	c.httpc = governor.NewHTTPFetcher(m)
	return nil
}

func (c *CmdClient) addAdmin(args []string) error {
	if c.addAdminReq.Password == "-" {
		var err error
		c.addAdminReq.Password, err = c.term.ReadLine()
		if err != nil && !errors.Is(err, io.EOF) {
			return kerrors.WithMsg(err, "Failed reading user password")
		}
	}
	if c.addAdminFlags.interactive && c.addAdminReq.Password == "" {
		fmt.Fprint(c.term.Stderr(), "Password: ")
		var err error
		c.addAdminReq.Password, err = c.term.ReadPassword()
		if err != nil {
			return kerrors.WithMsg(err, "Failed to read password")
		}
		fmt.Fprint(c.term.Stderr(), "Verify password: ")
		passwordAgain, err := c.term.ReadPassword()
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
	r, err := c.httpc.ReqJSON(http.MethodPost, "/user/admin", c.addAdminReq)
	if err != nil {
		return kerrors.WithMsg(err, "Failed to create admin request")
	}
	if err := c.gate.AddSysToken(r); err != nil {
		return kerrors.WithMsg(err, "Failed to add systoken")
	}
	var body resUserUpdate
	_, decoded, err := c.httpc.DoJSON(context.Background(), r, &body)
	if err != nil {
		return kerrors.WithMsg(err, "Failed adding admin")
	}
	if !decoded {
		return kerrors.WithKind(nil, governor.ErrServerRes, "Non-decodable response")
	}
	c.log.Info(context.Background(), "Created admin user",
		klog.AString("userid", body.Userid),
		klog.AString("username", body.Username),
	)
	return nil
}
