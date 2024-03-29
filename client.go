package governor

import (
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
		cmds       []*CmdTree
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

	// CmdTree is a tree of client cmds
	CmdTree struct {
		Desc     CmdDesc
		Handler  CmdHandler
		Children []*CmdTree
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
		Register(r ConfigRegistrar, cr CmdRegistrar)
		Init(r ClientConfigReader, kit ClientKit) error
	}

	ClientKit struct {
		Logger     klog.Logger
		Term       Term
		HTTPClient HTTPClient
	}

	cmdRegistrar struct {
		c      *Client
		parent *CmdTree
	}
)

// NewClient creates a new Client
func NewClient(opts Opts, clientOpts *ClientOpts) *Client {
	if clientOpts == nil {
		clientOpts = &ClientOpts{}
	}
	return &Client{
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

func (c *Client) addCmd(cmd *CmdTree) {
	c.cmds = append(c.cmds, cmd)
}

func (t *CmdTree) addCmd(cmd *CmdTree) {
	t.Children = append(t.Children, cmd)
}

func (r *cmdRegistrar) addCmd(cmd *CmdTree) {
	if r.parent == nil {
		r.c.addCmd(cmd)
	} else {
		r.parent.addCmd(cmd)
	}
}

func (r *cmdRegistrar) Register(cmd CmdDesc, handler CmdHandler) {
	r.addCmd(&CmdTree{
		Desc:    cmd,
		Handler: handler,
	})
}

func (r *cmdRegistrar) Group(cmd CmdDesc) CmdRegistrar {
	t := &CmdTree{
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
	r.Register(&configRegistrar{
		prefix: name,
		v:      c.settings.v,
	}, cr)
}

// GetCmds returns registered cmds
func (c *Client) GetCmds() []*CmdTree {
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
	c.term = newTermClient(c.configTerm)
	c.httpc = NewHTTPFetcher(newHTTPClient(c.settings.httpClient))

	for _, i := range c.clients {
		if err := i.r.Init(
			c.settings.reader(i.opt),
			ClientKit{
				Logger:     c.log.Logger.Sublogger(i.opt.name),
				Term:       c.term,
				HTTPClient: c.httpc.HTTPClient.Subclient(i.opt.url),
			},
		); err != nil {
			return kerrors.WithMsg(err, "Init client failed")
		}
	}
	return nil
}

func (c *Client) HTTPFetcher() *HTTPFetcher {
	return c.httpc
}
