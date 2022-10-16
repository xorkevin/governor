package governor

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
	"golang.org/x/term"
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
		clients []clientDef
		inj     Injector
		cmds    []*CmdTree
		config  *ClientConfig
		stdin   *bufio.Reader
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

	CLI interface {
		ReadString(delim byte) (string, error)
		ReadPassword() (string, error)
	}

	HTTPClient interface {
		NewRequest(method, path string, body io.Reader) (*http.Request, error)
		NewJSONRequest(method, path string, data interface{}) (*http.Request, error)
		DoRequest(r *http.Request) (*http.Response, error)
		DoRequestJSON(r *http.Request, response interface{}) (*http.Response, bool, error)
	}

	// ServiceClient is a client for a service
	ServiceClient interface {
		Register(inj Injector, r ConfigRegistrar, cr CmdRegistrar)
		Init(gc ClientConfig, r ConfigValueReader, cli CLI, m HTTPClient) error
	}

	cmdRegistrar struct {
		c      *Client
		parent *CmdTree
	}
)

// NewClient creates a new Client
func NewClient(opts Opts, stdin io.Reader) *Client {
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
		inj: newInjector(context.Background()),
		config: &ClientConfig{
			config: v,
		},
		stdin: bufio.NewReader(stdin),
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
		}, c, c); err != nil {
			return kerrors.WithMsg(err, "Init client failed")
		}
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

// NewRequest creates a new request
func (c *Client) NewRequest(method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, c.config.Addr+path, body)
	if err != nil {
		return nil, kerrors.WithKind(err, ErrorInvalidClientReq{}, "Malformed request")
	}
	return req, nil
}

// NewJSONRequest creates a new json request
func (c *Client) NewJSONRequest(method, path string, data interface{}) (*http.Request, error) {
	b, err := kjson.Marshal(data)
	if err != nil {
		return nil, kerrors.WithKind(err, ErrorInvalidClientReq{}, "Failed to encode body to json")
	}
	body := bytes.NewReader(b)
	req, err := c.NewRequest(method, path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Add(headerContentType, "application/json")
	return req, nil
}

// DoRequest sends a request to the server and returns its response
func (c *Client) DoRequest(r *http.Request) (*http.Response, error) {
	res, err := c.httpc.Do(r)
	if err != nil {
		return nil, kerrors.WithKind(err, ErrorInvalidClientReq{}, "Failed request")
	}
	if res.StatusCode >= http.StatusBadRequest {
		defer func() {
			if err := res.Body.Close(); err != nil {
				log.Println(kerrors.WithMsg(err, "Failed to close response body"))
			}
		}()
		var errres ErrorRes
		if err := json.NewDecoder(res.Body).Decode(&errres); err != nil {
			return res, kerrors.WithKind(err, ErrorInvalidServerRes{}, "Failed decoding response")
		}
		return res, kerrors.WithKind(nil, ErrorServerRes{}, errres.Message)
	}
	return res, nil
}

// DoRequestJSON sends a request to the server and decodes response json
func (c *Client) DoRequestJSON(r *http.Request, response interface{}) (*http.Response, bool, error) {
	res, err := c.DoRequest(r)
	if err != nil {
		return res, false, err
	}
	defer func() {
		if err := res.Body.Close(); err != nil {
			log.Println(kerrors.WithMsg(err, "Failed to close response body"))
		}
	}()

	decoded := false
	if response != nil && isStatusDecodable(res.StatusCode) {
		if err := json.NewDecoder(res.Body).Decode(response); err != nil {
			return res, false, kerrors.WithKind(err, ErrorInvalidServerRes{}, "Failed decoding response")
		}
		decoded = true
	}
	return res, decoded, nil
}

func isStatusDecodable(status int) bool {
	return status >= http.StatusOK && status < http.StatusMultipleChoices && status != http.StatusNoContent
}

// Setup sends a setup request to the server
func (c *Client) Setup(secret string) (*ResSetup, error) {
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
	_, decoded, err := c.DoRequestJSON(r, body)
	if err != nil {
		return nil, err
	}
	if !decoded {
		return nil, kerrors.WithKind(nil, ErrorServerRes{}, "Non-decodable response")
	}
	return body, nil
}
