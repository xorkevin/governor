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
		gate         gate.Client
		log          *klog.LevelLogger
		term         governor.Term
		httpc        *governor.HTTPFetcher
		reqUserPost  reqUserPost
		addUserFlags addUserFlags
		getUserFlags getUserFlags
		listFlags    listFlags
		useridFlags  useridFlags
		keyFlags     keyFlags
	}

	addUserFlags struct {
		interactive bool
	}

	getUserFlags struct {
		userid  string
		private bool
	}

	listFlags struct {
		amount int
		offset int
	}

	useridFlags struct {
		userid string
	}

	keyFlags struct {
		key         string
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
		Short: "add an admin",
		Long:  "add an admin",
		Flags: []governor.CmdFlag{
			{
				Long:     "username",
				Short:    "u",
				Usage:    "username",
				Required: true,
				Value:    &c.reqUserPost.Username,
			},
			{
				Long:     "password",
				Short:    "p",
				Usage:    "password",
				Required: false,
				Value:    &c.reqUserPost.Password,
			},
			{
				Long:     "email",
				Short:    "m",
				Usage:    "email",
				Required: true,
				Value:    &c.reqUserPost.Email,
			},
			{
				Long:     "firstname",
				Short:    "",
				Usage:    "user first name",
				Required: true,
				Value:    &c.reqUserPost.FirstName,
			},
			{
				Long:     "lastname",
				Short:    "",
				Usage:    "user last name",
				Required: true,
				Value:    &c.reqUserPost.LastName,
			},
			{
				Long:     "interactive",
				Short:    "",
				Usage:    "show interactive password prompt",
				Required: false,
				Value:    &c.addUserFlags.interactive,
			},
		},
	}, governor.CmdHandlerFunc(c.addAdmin))

	cr.Register(governor.CmdDesc{
		Usage: "get",
		Short: "get a user",
		Long:  "get a user",
		Flags: []governor.CmdFlag{
			{
				Long:     "userid",
				Short:    "i",
				Usage:    "userid",
				Required: true,
				Value:    &c.getUserFlags.userid,
			},
			{
				Long:     "private",
				Short:    "p",
				Usage:    "private info",
				Required: false,
				Value:    &c.getUserFlags.private,
			},
		},
	}, governor.CmdHandlerFunc(c.getUser))

	cr.Register(governor.CmdDesc{
		Usage: "create",
		Short: "create a user",
		Long:  "create a user",
		Flags: []governor.CmdFlag{
			{
				Long:     "username",
				Short:    "u",
				Usage:    "username",
				Required: true,
				Value:    &c.reqUserPost.Username,
			},
			{
				Long:     "password",
				Short:    "p",
				Usage:    "password",
				Required: false,
				Value:    &c.reqUserPost.Password,
			},
			{
				Long:     "email",
				Short:    "m",
				Usage:    "email",
				Required: true,
				Value:    &c.reqUserPost.Email,
			},
			{
				Long:     "firstname",
				Short:    "",
				Usage:    "user first name",
				Required: true,
				Value:    &c.reqUserPost.FirstName,
			},
			{
				Long:     "lastname",
				Short:    "",
				Usage:    "user last name",
				Required: true,
				Value:    &c.reqUserPost.LastName,
			},
			{
				Long:     "interactive",
				Short:    "",
				Usage:    "show interactive password prompt",
				Required: false,
				Value:    &c.addUserFlags.interactive,
			},
		},
	}, governor.CmdHandlerFunc(c.createUser))

	cr.Register(governor.CmdDesc{
		Usage: "commit",
		Short: "commits a user",
		Long:  "commits a user",
		Flags: []governor.CmdFlag{
			{
				Long:     "userid",
				Short:    "u",
				Usage:    "username",
				Required: true,
				Value:    &c.useridFlags.userid,
			},
			{
				Long:     "key",
				Short:    "k",
				Usage:    "key",
				Required: false,
				Value:    &c.keyFlags.key,
			},
			{
				Long:     "interactive",
				Short:    "",
				Usage:    "show interactive key prompt",
				Required: false,
				Value:    &c.keyFlags.interactive,
			},
		},
	}, governor.CmdHandlerFunc(c.commitUser))

	approval := cr.Group(governor.CmdDesc{
		Usage: "approval",
		Short: "manage user approvals",
		Long:  "manage user approvals",
	})

	approval.Register(governor.CmdDesc{
		Usage: "list",
		Short: "list user approvals",
		Long:  "list user approvals",
		Flags: []governor.CmdFlag{
			{
				Long:     "amount",
				Short:    "a",
				Usage:    "amount",
				Required: false,
				Value:    &c.listFlags.amount,
			},
			{
				Long:     "offset",
				Short:    "o",
				Usage:    "offset",
				Required: false,
				Value:    &c.listFlags.offset,
			},
		},
	}, governor.CmdHandlerFunc(c.getApprovals))

	approval.Register(governor.CmdDesc{
		Usage: "accept",
		Short: "accept user approvals",
		Long:  "accept user approvals",
		Flags: []governor.CmdFlag{
			{
				Long:     "userid",
				Short:    "i",
				Usage:    "userid",
				Required: true,
				Value:    &c.useridFlags.userid,
			},
		},
	}, governor.CmdHandlerFunc(c.acceptApproval))

	approval.Register(governor.CmdDesc{
		Usage: "deny",
		Short: "deny user approvals",
		Long:  "deny user approvals",
		Flags: []governor.CmdFlag{
			{
				Long:     "userid",
				Short:    "i",
				Usage:    "userid",
				Required: true,
				Value:    &c.useridFlags.userid,
			},
		},
	}, governor.CmdHandlerFunc(c.denyApproval))
}

func (c *CmdClient) Init(r governor.ClientConfigReader, kit governor.ClientKit) error {
	c.log = klog.NewLevelLogger(kit.Logger)
	c.term = kit.Term
	c.httpc = governor.NewHTTPFetcher(kit.HTTPClient)
	return nil
}

func (c *CmdClient) addAdmin(args []string) error {
	if c.reqUserPost.Password == "-" {
		var err error
		c.reqUserPost.Password, err = c.term.ReadLine()
		if err != nil && !errors.Is(err, io.EOF) {
			return kerrors.WithMsg(err, "Failed reading user password")
		}
	}
	if c.addUserFlags.interactive && c.reqUserPost.Password == "" {
		fmt.Fprint(c.term.Stderr(), "Password: ")
		var err error
		c.reqUserPost.Password, err = c.term.ReadPassword()
		if err != nil {
			return kerrors.WithMsg(err, "Failed to read password")
		}
		fmt.Fprint(c.term.Stderr(), "Verify password: ")
		passwordAgain, err := c.term.ReadPassword()
		if err != nil {
			return kerrors.WithMsg(err, "Failed to read password")
		}
		if passwordAgain != c.reqUserPost.Password {
			return kerrors.WithMsg(err, "Passwords do not match")
		}
	}
	if err := c.reqUserPost.valid(); err != nil {
		return err
	}
	r, err := c.httpc.ReqJSON(http.MethodPost, "/user/admin", c.reqUserPost)
	if err != nil {
		return kerrors.WithMsg(err, "Failed to create admin request")
	}
	if err := c.gate.AddReqToken(r); err != nil {
		return kerrors.WithMsg(err, "Failed to add token")
	}
	var body resUserUpdate
	if _, err := c.httpc.DoJSON(context.Background(), r, &body); err != nil {
		return kerrors.WithMsg(err, "Failed adding admin")
	}
	if _, err := io.WriteString(c.term.Stdout(), body.Userid+"\n"); err != nil {
		return kerrors.WithMsg(err, "Failed writing response")
	}
	return nil
}

func (c *CmdClient) getUser(args []string) error {
	var u string
	if c.getUserFlags.private {
		u = fmt.Sprintf("/user/id/%s/private", c.getUserFlags.userid)
	} else {
		u = fmt.Sprintf("/user/id/%s", c.getUserFlags.userid)
	}
	r, err := c.httpc.HTTPClient.Req(http.MethodGet, u, nil)
	if err != nil {
		return kerrors.WithMsg(err, "Failed to create get user request")
	}
	if c.getUserFlags.private {
		if err := c.gate.AddReqToken(r); err != nil {
			return kerrors.WithMsg(err, "Failed to add token")
		}
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

func (c *CmdClient) createUser(args []string) error {
	if c.reqUserPost.Password == "-" {
		var err error
		c.reqUserPost.Password, err = c.term.ReadLine()
		if err != nil && !errors.Is(err, io.EOF) {
			return kerrors.WithMsg(err, "Failed reading user password")
		}
	}
	if c.addUserFlags.interactive && c.reqUserPost.Password == "" {
		fmt.Fprint(c.term.Stderr(), "Password: ")
		var err error
		c.reqUserPost.Password, err = c.term.ReadPassword()
		if err != nil {
			return kerrors.WithMsg(err, "Failed to read password")
		}
		fmt.Fprint(c.term.Stderr(), "Verify password: ")
		passwordAgain, err := c.term.ReadPassword()
		if err != nil {
			return kerrors.WithMsg(err, "Failed to read password")
		}
		if passwordAgain != c.reqUserPost.Password {
			return kerrors.WithMsg(err, "Passwords do not match")
		}
	}
	if err := c.reqUserPost.valid(); err != nil {
		return err
	}
	r, err := c.httpc.ReqJSON(http.MethodPost, "/user", c.reqUserPost)
	if err != nil {
		return kerrors.WithMsg(err, "Failed to create user request")
	}
	var body resUserUpdate
	if _, err := c.httpc.DoJSON(context.Background(), r, &body); err != nil {
		return kerrors.WithMsg(err, "Failed creating user")
	}
	if !body.Created {
		c.log.Info(context.Background(), "Created user pending approval",
			klog.AString("userid", body.Userid),
			klog.AString("username", body.Username),
		)
	}
	if _, err := io.WriteString(c.term.Stdout(), body.Userid+"\n"); err != nil {
		return kerrors.WithMsg(err, "Failed writing response")
	}
	return nil
}

func (c *CmdClient) commitUser(args []string) error {
	if c.keyFlags.key == "-" {
		var err error
		c.keyFlags.key, err = c.term.ReadLine()
		if err != nil && !errors.Is(err, io.EOF) {
			return kerrors.WithMsg(err, "Failed reading key")
		}
	}
	if c.keyFlags.interactive && c.keyFlags.key == "" {
		fmt.Fprint(c.term.Stderr(), "Key: ")
		var err error
		c.keyFlags.key, err = c.term.ReadPassword()
		if err != nil {
			return kerrors.WithMsg(err, "Failed to read key")
		}
	}
	r, err := c.httpc.ReqJSON(http.MethodPost, "/user/confirm", reqUserPostConfirm{
		Userid: c.useridFlags.userid,
		Key:    c.keyFlags.key,
	})
	if err != nil {
		return kerrors.WithMsg(err, "Failed to create commit user request")
	}
	var body resUserUpdate
	if _, err := c.httpc.DoJSON(context.Background(), r, &body); err != nil {
		return kerrors.WithMsg(err, "Failed committing user")
	}
	if _, err := io.WriteString(c.term.Stdout(), body.Userid+"\n"); err != nil {
		return kerrors.WithMsg(err, "Failed writing response")
	}
	return nil
}

func (c *CmdClient) getApprovals(args []string) error {
	r, err := c.httpc.HTTPClient.Req(
		http.MethodGet,
		fmt.Sprintf("/user/approvals?amount=%d&offset=%d", c.listFlags.amount, c.listFlags.offset),
		nil,
	)
	if err != nil {
		return kerrors.WithMsg(err, "Failed to create get user approvals request")
	}
	if err := c.gate.AddReqToken(r); err != nil {
		return kerrors.WithMsg(err, "Failed to add token")
	}
	_, body, err := c.httpc.DoBytes(context.Background(), r)
	if err != nil {
		return kerrors.WithMsg(err, "Failed getting user approvals")
	}
	if _, err := c.term.Stdout().Write(append(body, '\n')); err != nil {
		return kerrors.WithMsg(err, "Failed writing response")
	}
	return nil
}

func (c *CmdClient) acceptApproval(args []string) error {
	r, err := c.httpc.HTTPClient.Req(
		http.MethodPost,
		fmt.Sprintf("/user/approvals/id/%s", c.useridFlags.userid),
		nil,
	)
	if err != nil {
		return kerrors.WithMsg(err, "Failed to create accept user approval request")
	}
	if err := c.gate.AddReqToken(r); err != nil {
		return kerrors.WithMsg(err, "Failed to add token")
	}
	if _, err := c.httpc.DoNoContent(context.Background(), r); err != nil {
		return kerrors.WithMsg(err, "Failed approving user")
	}
	return nil
}

func (c *CmdClient) denyApproval(args []string) error {
	r, err := c.httpc.HTTPClient.Req(
		http.MethodDelete,
		fmt.Sprintf("/user/approvals/id/%s", c.useridFlags.userid),
		nil,
	)
	if err != nil {
		return kerrors.WithMsg(err, "Failed to create deny user approval request")
	}
	if err := c.gate.AddReqToken(r); err != nil {
		return kerrors.WithMsg(err, "Failed to add token")
	}
	if _, err := c.httpc.DoNoContent(context.Background(), r); err != nil {
		return kerrors.WithMsg(err, "Failed denying user approval")
	}
	return nil
}
