package governor

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
	"golang.org/x/term"
	"xorkevin.dev/governor/util/kjson"
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
		clients []clientDef
		inj     Injector
		cmds    []*cmdTree
		config  *ClientConfig
		log     *klog.LevelLogger
		stdout  io.Writer
		stdin   *bufio.Reader
		httpc   *http.Client
		flags   ClientFlags
	}

	// ClientConfig is the client config
	ClientConfig struct {
		config    *viper.Viper
		logLevel  string
		logOutput string
		logWriter io.Writer
		Addr      string
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

	CLI interface {
		Stdout() io.Writer
		Stdin() io.Reader
		ReadFile(name string) ([]byte, error)
		WriteFile(name string, data []byte, mode fs.FileMode) error
		ReadString(delim byte) (string, error)
		ReadPassword() (string, error)
	}

	HTTPClient interface {
		NewRequest(method, path string, body io.Reader) (*http.Request, error)
		NewJSONRequest(method, path string, data interface{}) (*http.Request, error)
		DoRequest(ctx context.Context, r *http.Request) (*http.Response, error)
		DoRequestNoContent(ctx context.Context, r *http.Request) (*http.Response, error)
		DoRequestJSON(ctx context.Context, r *http.Request, response interface{}) (*http.Response, bool, error)
	}

	// ServiceClient is a client for a service
	ServiceClient interface {
		Register(inj Injector, r ConfigRegistrar, cr CmdRegistrar)
		Init(gc ClientConfig, r ConfigValueReader, log klog.Logger, cli CLI, m HTTPClient) error
	}

	cmdRegistrar struct {
		c      *Client
		parent *cmdTree
	}
)

// NewClient creates a new Client
func NewClient(opts Opts, stdout io.Writer, stdin io.Reader) *Client {
	v := viper.New()
	v.SetDefault("addr", "http://localhost:8080/api")
	v.SetDefault("timeout", "5s")
	v.SetDefault("loglevel", "INFO")
	v.SetDefault("logoutput", "STDERR")

	v.SetConfigName(opts.ClientDefault)
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath(filepath.Join(".", "config"))
	if cfgdir, err := os.UserConfigDir(); err == nil {
		v.AddConfigPath(filepath.Join(cfgdir, opts.Appname))
	}

	v.SetEnvPrefix(opts.EnvPrefix)
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "__"))

	return &Client{
		inj: newInjector(context.Background()),
		config: &ClientConfig{
			config: v,
		},
		stdout: stdout,
		stdin:  bufio.NewReader(stdin),
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
		v:      c.config.config,
	}, cr)
}

// GetCmds returns registered cmds
func (c *Client) GetCmds() []*cmdTree {
	return c.cmds
}

// Init initializes the Client by reading a config
func (c *Client) Init() error {
	if file := c.flags.ConfigFile; file != "" {
		c.config.config.SetConfigFile(file)
	}
	if err := c.config.config.ReadInConfig(); err != nil {
		return kerrors.WithKind(err, ErrorInvalidConfig, "Failed to read in config")
	}

	c.config.Addr = c.config.config.GetString("addr")
	if t, err := time.ParseDuration(c.config.config.GetString("timeout")); err == nil {
		c.httpc.Timeout = t
	} else {
		return kerrors.WithKind(err, ErrorInvalidConfig, "Invalid http client timeout")
	}

	c.config.logLevel = c.config.config.GetString("loglevel")
	c.config.logOutput = c.config.config.GetString("logoutput")
	c.log = newPlaintextLogger(*c.config)

	for _, i := range c.clients {
		l := klog.Sub(c.log.Logger, i.opt.name, nil)
		if err := i.r.Init(*c.config, &configValueReader{
			opt: i.opt,
			v:   c.config.config,
		}, l, c, c); err != nil {
			return kerrors.WithMsg(err, "Init client failed")
		}
	}
	return nil
}

func (c *Client) Stdout() io.Writer {
	return c.stdout
}

func (c *Client) Stdin() io.Reader {
	return c.stdin
}

func (c *Client) ReadFile(name string) ([]byte, error) {
	b, err := os.ReadFile(name)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to read file")
	}
	return b, nil
}

func (c *Client) WriteFile(name string, data []byte, mode fs.FileMode) error {
	if err := os.WriteFile(name, data, mode); err != nil {
		return kerrors.WithMsg(err, "Failed to write file")
	}
	return nil
}

func (c *Client) ReadString(delim byte) (string, error) {
	s, err := c.stdin.ReadString(delim)
	if err != nil && !errors.Is(err, io.EOF) {
		err = kerrors.WithMsg(err, "Failed to read stdin")
	}
	return s, err
}

func (c *Client) ReadPassword() (string, error) {
	s, err := term.ReadPassword(0)
	if err != nil {
		return "", kerrors.WithMsg(err, "Failed to read password")
	}
	fmt.Println()
	return string(s), nil
}

// Client http errors
var (
	// ErrorInvalidClientReq is returned when the client request could not be made
	ErrorInvalidClientReq errorInvalidClientReq
	// ErrorInvalidServerRes is returned on an invalid server response
	ErrorInvalidServerRes errorInvalidServerRes
	// ErrorServerRes is a returned server error
	ErrorServerRes errorServerRes
)

type (
	errorInvalidClientReq struct{}
	errorInvalidServerRes struct{}
	errorServerRes        struct{}
)

func (e errorInvalidClientReq) Error() string {
	return "Invalid client request"
}

func (e errorInvalidServerRes) Error() string {
	return "Invalid server response"
}

func (e errorServerRes) Error() string {
	return "Error server response"
}

// NewRequest creates a new request
func (c *Client) NewRequest(method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, c.config.Addr+path, body)
	if err != nil {
		return nil, kerrors.WithKind(err, ErrorInvalidClientReq, "Malformed request")
	}
	return req, nil
}

// NewJSONRequest creates a new json request
func (c *Client) NewJSONRequest(method, path string, data interface{}) (*http.Request, error) {
	b, err := kjson.Marshal(data)
	if err != nil {
		return nil, kerrors.WithKind(err, ErrorInvalidClientReq, "Failed to encode body to json")
	}
	body := bytes.NewReader(b)
	req, err := c.NewRequest(method, path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set(headerContentType, "application/json")
	return req, nil
}

// DoRequest sends a request to the server and returns its response
func (c *Client) DoRequest(ctx context.Context, r *http.Request) (*http.Response, error) {
	res, err := c.httpc.Do(r)
	if err != nil {
		return nil, kerrors.WithKind(err, ErrorInvalidClientReq, "Failed request")
	}
	if res.StatusCode >= http.StatusBadRequest {
		defer func() {
			if err := res.Body.Close(); err != nil {
				c.log.Err(ctx, kerrors.WithMsg(err, "Failed to close http response body"), nil)
			}
		}()
		defer func() {
			if _, err := io.Copy(io.Discard, res.Body); err != nil {
				c.log.Err(ctx, kerrors.WithMsg(err, "Failed to discard http response body"), nil)
			}
		}()
		var errres ErrorRes
		if err := json.NewDecoder(res.Body).Decode(&errres); err != nil {
			return res, kerrors.WithKind(err, ErrorInvalidServerRes, "Failed decoding response")
		}
		return res, kerrors.WithKind(nil, ErrorServerRes, errres.Message)
	}
	return res, nil
}

// DoRequestNoContent sends a request to the server and discards the response body
func (c *Client) DoRequestNoContent(ctx context.Context, r *http.Request) (*http.Response, error) {
	res, err := c.DoRequest(ctx, r)
	if err != nil {
		return res, err
	}
	defer func() {
		if err := res.Body.Close(); err != nil {
			c.log.Err(ctx, kerrors.WithMsg(err, "Failed to close http response body"), nil)
		}
	}()
	defer func() {
		if _, err := io.Copy(io.Discard, res.Body); err != nil {
			c.log.Err(ctx, kerrors.WithMsg(err, "Failed to discard http response body"), nil)
		}
	}()
	return res, nil
}

// DoRequestJSON sends a request to the server and decodes response json
func (c *Client) DoRequestJSON(ctx context.Context, r *http.Request, response interface{}) (*http.Response, bool, error) {
	res, err := c.DoRequest(ctx, r)
	if err != nil {
		return res, false, err
	}
	defer func() {
		if err := res.Body.Close(); err != nil {
			c.log.Err(ctx, kerrors.WithMsg(err, "Failed to close http response body"), nil)
		}
	}()
	defer func() {
		if _, err := io.Copy(io.Discard, res.Body); err != nil {
			c.log.Err(ctx, kerrors.WithMsg(err, "Failed to discard http response body"), nil)
		}
	}()

	decoded := false
	if response != nil && isStatusDecodable(res.StatusCode) {
		if err := json.NewDecoder(res.Body).Decode(response); err != nil {
			return res, false, kerrors.WithKind(err, ErrorInvalidServerRes, "Failed decoding response")
		}
		decoded = true
	}
	return res, decoded, nil
}

func isStatusDecodable(status int) bool {
	return status >= http.StatusOK && status < http.StatusMultipleChoices && status != http.StatusNoContent
}

// Setup sends a setup request to the server
func (c *Client) Setup(ctx context.Context, secret string) (*ResSetup, error) {
	if secret == "-" {
		var err error
		secret, err = c.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, kerrors.WithMsg(err, "Failed reading setup secret")
		}
	}
	if err := setupSecretValid(secret); err != nil {
		return nil, err
	}
	body := &ResSetup{}
	r, err := c.NewRequest(http.MethodPost, "/setupz", nil)
	if err != nil {
		return nil, err
	}
	r.SetBasicAuth("setup", secret)
	_, decoded, err := c.DoRequestJSON(ctx, r, body)
	if err != nil {
		return nil, err
	}
	if !decoded {
		return nil, kerrors.WithKind(nil, ErrorServerRes, "Non-decodable response")
	}
	c.log.Info(ctx, "Successfully setup governor", klog.Fields{
		"version": body.Version,
	})
	return body, nil
}
