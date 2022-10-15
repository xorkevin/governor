package governor

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
	"xorkevin.dev/governor/util/kjson"
	"xorkevin.dev/kerrors"
)

type (
	// ClientFlags are flags for the client cmd
	ClientFlags struct {
		ConfigFile string
	}

	// Client is a server client
	Client struct {
		config  *ClientConfig
		clients []clientDef
		cmds    []*CmdTree
		httpc   *http.Client
		flags   ClientFlags
	}

	// ClientConfig is the client config
	ClientConfig struct {
		config *viper.Viper
		Addr   string
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
		Init(gc ClientConfig, r ConfigValueReader) error
	}

	cmdRegistrar struct {
		c      *Client
		parent *CmdTree
	}
)

// NewClient creates a new Client
func NewClient(opts Opts) *Client {
	v := viper.New()
	v.SetDefault("addr", "http://localhost:8080/api")
	v.SetDefault("timeout", "5s")

	v.SetConfigName(opts.ClientDefault)
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath(filepath.Join(".", "config"))
	if cfgdir, err := os.UserConfigDir(); err == nil {
		v.AddConfigPath(filepath.Join(cfgdir, opts.Appname))
	}

	v.SetEnvPrefix(opts.EnvPrefix)
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	return &Client{
		config: &ClientConfig{
			config: v,
		},
		httpc: &http.Client{
			Timeout: 15 * time.Second,
		},
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

// GetConfig returns the client config
func (c *Client) GetConfig() ClientConfig {
	return *c.config
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
func (c *Client) Register(name string, url string, cmd CmdDesc, r ServiceClient) {
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
	if cmd.Usage != "" {
		cr = cr.Group(cmd)
	}
	r.Register(&configRegistrar{
		prefix: name,
		v:      c.config.config,
	}, cr)
}

// GetCmds returns registered cmds
func (c *Client) GetCmds() []*CmdTree {
	return c.cmds
}

// Init initializes the Client by reading a config
func (c *Client) Init() error {
	if file := c.flags.ConfigFile; file != "" {
		c.config.config.SetConfigFile(file)
	}
	if err := c.config.config.ReadInConfig(); err != nil {
		return kerrors.WithKind(err, ErrorInvalidConfig{}, "Failed to read in config")
	}
	c.config.Addr = c.config.config.GetString("addr")
	if t, err := time.ParseDuration(c.config.config.GetString("timeout")); err == nil {
		c.httpc.Timeout = t
	} else {
		return kerrors.WithKind(err, ErrorInvalidConfig{}, "Invalid http client timeout")
	}
	for _, i := range c.clients {
		if err := i.r.Init(*c.config, &configValueReader{
			opt: i.opt,
			v:   c.config.config,
		}); err != nil {
			return kerrors.WithMsg(err, "Init client failed")
		}
	}
	return nil
}

type (
	// ErrorInvalidClientReq is returned when the client request could not be made
	ErrorInvalidClientReq struct{}
	// ErrorInvalidServerRes is returned on an invalid server response
	ErrorInvalidServerRes struct{}
	// ErrorServerRes is a returned server error
	ErrorServerRes struct{}
)

func (e ErrorInvalidClientReq) Error() string {
	return "Invalid client request"
}

func (e ErrorInvalidServerRes) Error() string {
	return "Invalid server response"
}

func (e ErrorServerRes) Error() string {
	return "Error server response"
}

// Request sends a request to the server
func (c *Client) Request(method, path string, data interface{}, response interface{}) (int, error) {
	var body io.Reader
	if data != nil {
		b, err := kjson.Marshal(data)
		if err != nil {
			return 0, kerrors.WithKind(err, ErrorInvalidClientReq{}, "Failed to encode body to json")
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.config.Addr+path, body)
	if body != nil {
		req.Header.Add("Content-Type", "application/json")
	}
	if err != nil {
		return 0, kerrors.WithKind(err, ErrorInvalidClientReq{}, "Malformed request")
	}
	res, err := c.httpc.Do(req)
	if err != nil {
		return 0, kerrors.WithKind(err, ErrorInvalidClientReq{}, "Failed request")
	}
	defer res.Body.Close()
	if res.StatusCode >= http.StatusBadRequest {
		var errres ErrorRes
		if err := json.NewDecoder(res.Body).Decode(&errres); err != nil {
			return 0, kerrors.WithKind(err, ErrorInvalidServerRes{}, "Failed decoding response")
		}
		return res.StatusCode, kerrors.WithKind(nil, ErrorServerRes{}, errres.Message)
	} else if err := json.NewDecoder(res.Body).Decode(response); err != nil {
		return 0, kerrors.WithKind(err, ErrorInvalidServerRes{}, "Failed decoding response")
	}
	return res.StatusCode, nil
}

func isStatusOK(status int) bool {
	return status >= http.StatusOK && status < http.StatusMultipleChoices
}

// Setup sends a setup request to the server
func (c *Client) Setup(req ReqSetup) (*ResSetup, error) {
	res := &ResSetup{}
	if status, err := c.Request("POST", "/setupz", req, res); err != nil {
		return nil, err
	} else if !isStatusOK(status) {
		return nil, kerrors.WithKind(nil, ErrorServerRes{}, "Non success response")
	}
	return res, nil
}
