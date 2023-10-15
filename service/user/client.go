package user

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/gate"
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
		getUserFlags  getUserFlags
	}

	addAdminFlags struct {
		interactive bool
	}

	getUserFlags struct {
		userid string
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
	cr.Register(governor.CmdDesc{
		Usage: "get",
		Short: "gets a user",
		Long:  "gets a user",
		Flags: []governor.CmdFlag{
			{
				Long:     "userid",
				Short:    "i",
				Usage:    "userid",
				Required: true,
				Value:    &c.getUserFlags.userid,
			},
		},
	}, governor.CmdHandlerFunc(c.getUser))
}

func (c *CmdClient) Init(r governor.ClientConfigReader, kit governor.ClientKit) error {
	c.log = klog.NewLevelLogger(kit.Logger)
	c.term = kit.Term
	c.httpc = governor.NewHTTPFetcher(kit.HTTPClient)
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
	if err := c.gate.AddReqToken(r); err != nil {
		return kerrors.WithMsg(err, "Failed to add systoken")
	}
	var body resUserUpdate
	if _, err := c.httpc.DoJSON(context.Background(), r, &body); err != nil {
		return kerrors.WithMsg(err, "Failed adding admin")
	}
	c.log.Info(context.Background(), "Created admin user",
		klog.AString("userid", body.Userid),
		klog.AString("username", body.Username),
	)
	if _, err := io.WriteString(c.term.Stdout(), body.Userid+"\n"); err != nil {
		return kerrors.WithMsg(err, "Failed writing response")
	}
	return nil
}

func (c *CmdClient) getUser(args []string) error {
	r, err := c.httpc.HTTPClient.Req(http.MethodGet, fmt.Sprintf("/user/id/%s", c.getUserFlags.userid), nil)
	if err != nil {
		return kerrors.WithMsg(err, "Failed to create get user request")
	}
	if err := c.gate.AddReqToken(r); err != nil {
		c.log.Err(context.Background(), kerrors.WithMsg(err, "Failed to add systoken"))
	}
	_, body, err := c.httpc.DoBytes(context.Background(), r)
	if err != nil {
		return kerrors.WithMsg(err, "Failed getting user")
	}
	if _, err := c.term.Stdout().Write(append(body, '\n')); err != nil {
		return kerrors.WithMsg(err, "Failed writing response")
	}
	return nil
}
