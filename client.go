package governor

import (
	"context"
	"errors"
	"io"
	"net/http"

	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	// ClientFlags are flags for the client cmd
	ClientFlags struct {
		ConfigFile string
	}

	// Client is a server client
	Client struct {
		clients    []clientDef
		inj        Injector
		cmds       []*cmdTree
		settings   *clientSettings
		configTerm *TermConfig
		term       Term
		log        *klog.LevelLogger
		httpc      *HTTPFetcher
		flags      ClientFlags
	}

	clientDef struct {
		opt serviceOpt
		r   ServiceClient
	}

	// cmdTree is a tree of client cmds
	cmdTree struct {
		Desc     CmdDesc
		Handler  CmdHandler
		Children []*cmdTree
	}

	// CmdFlag describes a client flag
	CmdFlag struct {
		Long     string
		Short    string
		Usage    string
		Required bool
		Default  interface{}
		Value    interface{}
	}

	// CmdDesc describes a client cmd
	CmdDesc struct {
		Usage string
		Short string
		Long  string
		Flags []CmdFlag
	}

	// CmdHandler handles a client cmd
	CmdHandler interface {
		Handle(args []string) error
	}

	// CmdHandlerFunc implements CmdHandler for a function
	CmdHandlerFunc func(args []string) error

	// CmdRegistrar registers cmd handlers on a client
	CmdRegistrar interface {
		Register(cmd CmdDesc, handler CmdHandler)
		Group(cmd CmdDesc) CmdRegistrar
	}

	// ServiceClient is a client for a service
	ServiceClient interface {
		Register(inj Injector, r ConfigRegistrar, cr CmdRegistrar)
		Init(r ClientConfigReader, log klog.Logger, term Term, m HTTPClient) error
	}

	cmdRegistrar struct {
		c      *Client
		parent *cmdTree
	}
)

// NewClient creates a new Client
func NewClient(opts Opts, clientOpts *ClientOpts) *Client {
	if clientOpts == nil {
		clientOpts = &ClientOpts{}
	}
	return &Client{
		inj:        newInjector(context.Background()),
		settings:   newClientSettings(opts, *clientOpts),
		configTerm: clientOpts.TermConfig,
	}
}

// Handle implements [CmdHandler]
func (f CmdHandlerFunc) Handle(args []string) error {
	return f(args)
}

// SetFlags sets Client flags
func (c *Client) SetFlags(flags ClientFlags) {
	c.flags = flags
}

func (c *Client) addCmd(cmd *cmdTree) {
	c.cmds = append(c.cmds, cmd)
}

func (t *cmdTree) addCmd(cmd *cmdTree) {
	t.Children = append(t.Children, cmd)
}

func (r *cmdRegistrar) addCmd(cmd *cmdTree) {
	if r.parent == nil {
		r.c.addCmd(cmd)
	} else {
		r.parent.addCmd(cmd)
	}
}

func (r *cmdRegistrar) Register(cmd CmdDesc, handler CmdHandler) {
	r.addCmd(&cmdTree{
		Desc:    cmd,
		Handler: handler,
	})
}

func (r *cmdRegistrar) Group(cmd CmdDesc) CmdRegistrar {
	t := &cmdTree{
		Desc: cmd,
	}
	r.addCmd(t)
	return &cmdRegistrar{
		parent: t,
	}
}

// Register registers a service client
func (c *Client) Register(name string, url string, cmd *CmdDesc, r ServiceClient) {
	c.clients = append(c.clients, clientDef{
		opt: serviceOpt{
			name: name,
			url:  url,
		},
		r: r,
	})
	var cr CmdRegistrar = &cmdRegistrar{
		c: c,
	}
	if cmd != nil {
		cr = cr.Group(*cmd)
	}
	r.Register(c.inj, &configRegistrar{
		prefix: name,
		v:      c.settings.v,
	}, cr)
}

// GetCmds returns registered cmds
func (c *Client) GetCmds() []*cmdTree {
	return c.cmds
}

// Init initializes the Client by reading a config
func (c *Client) Init(flags ClientFlags, log klog.Logger) error {
	if err := c.settings.init(c.flags); err != nil {
		return err
	}

	c.log = klog.NewLevelLogger(klog.New(
		klog.OptHandler(log.Handler()),
		klog.OptMinLevelStr(c.settings.logger.level),
	))
	c.term = newTermClient(c.configTerm, c.log.Logger)
	httpc := newHTTPClient(c.settings.httpClient, c.log.Logger)
	c.httpc = NewHTTPFetcher(httpc)

	for _, i := range c.clients {
		l := c.log.Logger.Sublogger(i.opt.name)
		if err := i.r.Init(c.settings.reader(i.opt), l, c.term, httpc.subclient(i.opt.url, l)); err != nil {
			return kerrors.WithMsg(err, "Init client failed")
		}
	}
	return nil
}

// Setup sends a setup request to the server
func (c *Client) Setup(ctx context.Context, secret string) (*ResSetup, error) {
	if secret == "-" {
		var err error
		secret, err = c.term.ReadLine()
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, kerrors.WithMsg(err, "Failed reading setup secret")
		}
	}
	if err := setupSecretValid(secret); err != nil {
		return nil, err
	}
	body := &ResSetup{}
	r, err := c.httpc.Req(http.MethodPost, "/setupz", nil)
	if err != nil {
		return nil, err
	}
	r.SetBasicAuth("setup", secret)
	_, decoded, err := c.httpc.DoJSON(ctx, r, body)
	if err != nil {
		return nil, err
	}
	if !decoded {
		return nil, kerrors.WithKind(nil, ErrServerRes, "Non-decodable response")
	}
	c.log.Info(ctx, "Successfully setup governor", klog.AString("version", body.Version))
	return body, nil
}
