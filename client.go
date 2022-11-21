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
		clients  []clientDef
		inj      Injector
		cmds     []*cmdTree
		settings *clientSettings
		stdout   io.Writer
		stdin    *bufio.Reader
		log      *klog.LevelLogger
		httpc    *HTTPFetcher
		flags    ClientFlags
	}

	clientSettings struct {
		v            *viper.Viper
		configReader io.Reader
		config       ClientConfig
		logger       configLogger
		httpClient   configHTTPClient
	}

	configHTTPClient struct {
		baseurl string
		timeout time.Duration
	}

	// ClientConfig is the client config
	ClientConfig struct {
		BaseURL string
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
		Req(method, path string, body io.Reader) (*http.Request, error)
		Do(ctx context.Context, r *http.Request) (*http.Response, error)
		Log() klog.Logger
	}

	// ServiceClient is a client for a service
	ServiceClient interface {
		Register(inj Injector, r ConfigRegistrar, cr CmdRegistrar)
		Init(r ClientConfigReader, log klog.Logger, cli CLI, m HTTPClient) error
	}

	cmdRegistrar struct {
		c      *Client
		parent *cmdTree
	}
)

// NewClient creates a new Client
func NewClient(opts Opts, stdout io.Writer, stdin io.Reader) *Client {
	v := viper.New()
	v.SetDefault("logger.level", "INFO")
	v.SetDefault("logger.output", "STDERR")
	v.SetDefault("http.addr", "http://localhost:8080/api")
	v.SetDefault("http.timeout", "15s")

	v.SetConfigName(opts.ClientDefault)
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("config")
	if cfgdir, err := os.UserConfigDir(); err == nil {
		v.AddConfigPath(filepath.Join(cfgdir, opts.Appname))
	}

	v.SetEnvPrefix(opts.ClientPrefix)
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "__"))

	return &Client{
		inj: newInjector(context.Background()),
		settings: &clientSettings{
			v: v,
		},
		stdout: stdout,
		stdin:  bufio.NewReader(stdin),
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
func (c *Client) Init() error {
	if err := c.settings.init(c.flags); err != nil {
		return err
	}

	c.log = newPlaintextLogger(c.settings.logger)

	httpc := newHTTPClient(c.settings.httpClient, c.log.Logger)
	c.httpc = NewHTTPFetcher(httpc)

	for _, i := range c.clients {
		l := klog.Sub(c.log.Logger, i.opt.name, nil)
		if err := i.r.Init(c.settings.reader(i.opt), l, c, httpc.subclient(i.opt.url, l)); err != nil {
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

type (
	httpClient struct {
		log   *klog.LevelLogger
		httpc *http.Client
		base  string
	}
)

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

func newHTTPClient(c configHTTPClient, l klog.Logger) *httpClient {
	return &httpClient{
		log: klog.NewLevelLogger(klog.Sub(l, "httpc", nil)),
		httpc: &http.Client{
			Timeout: c.timeout,
		},
		base: c.baseurl,
	}
}

func (c *httpClient) subclient(path string, l klog.Logger) HTTPClient {
	return &httpClient{
		log:   klog.NewLevelLogger(klog.Sub(l, "httpc", nil)),
		httpc: c.httpc,
		base:  c.base + path,
	}
}

// Req creates a new request
func (c *httpClient) Req(method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, c.base+path, body)
	if err != nil {
		return nil, kerrors.WithKind(err, ErrorInvalidClientReq, "Malformed request")
	}
	return req, nil
}

// Do sends a request to the server and returns its response
func (c *httpClient) Do(ctx context.Context, r *http.Request) (*http.Response, error) {
	ctx = klog.WithFields(ctx, klog.Fields{
		"gov.httpc.url": r.URL.String(),
	})
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

func (c *httpClient) Log() klog.Logger {
	return c.log.Logger
}

type (
	// HTTPFetcher provides convenience HTTP client functionality
	HTTPFetcher struct {
		HTTPClient HTTPClient
		log        *klog.LevelLogger
	}
)

// NewHTTPFetcher creates a new [*HTTPFetcher]
func NewHTTPFetcher(c HTTPClient) *HTTPFetcher {
	return &HTTPFetcher{
		HTTPClient: c,
		log:        klog.NewLevelLogger(c.Log()),
	}
}

// Req calls [HTTPClient] Req
func (c *HTTPFetcher) Req(method, path string, body io.Reader) (*http.Request, error) {
	return c.HTTPClient.Req(method, path, body)
}

// Do calls [HTTPClient] Do
func (c *HTTPFetcher) Do(ctx context.Context, r *http.Request) (*http.Response, error) {
	return c.HTTPClient.Do(ctx, r)
}

func (c *HTTPFetcher) Log() klog.Logger {
	return c.log.Logger
}

// ReqJSON creates a new json request
func (c *HTTPFetcher) ReqJSON(method, path string, data interface{}) (*http.Request, error) {
	b, err := kjson.Marshal(data)
	if err != nil {
		return nil, kerrors.WithKind(err, ErrorInvalidClientReq, "Failed to encode body to json")
	}
	body := bytes.NewReader(b)
	req, err := c.HTTPClient.Req(method, path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set(headerContentType, "application/json")
	return req, nil
}

// DoNoContent sends a request to the server and discards the response body
func (c *HTTPFetcher) DoNoContent(ctx context.Context, r *http.Request) (*http.Response, error) {
	ctx = klog.WithFields(ctx, klog.Fields{
		"gov.httpc.url": r.URL.String(),
	})
	res, err := c.HTTPClient.Do(ctx, r)
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

// DoJSON sends a request to the server and decodes response json
func (c *HTTPFetcher) DoJSON(ctx context.Context, r *http.Request, response interface{}) (*http.Response, bool, error) {
	ctx = klog.WithFields(ctx, klog.Fields{
		"gov.httpc.url": r.URL.String(),
	})
	res, err := c.HTTPClient.Do(ctx, r)
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
		return nil, kerrors.WithKind(nil, ErrorServerRes, "Non-decodable response")
	}
	c.log.Info(ctx, "Successfully setup governor", klog.Fields{
		"version": body.Version,
	})
	return body, nil
}

func (s *clientSettings) init(flags ClientFlags) error {
	if file := flags.ConfigFile; file != "" {
		s.v.SetConfigFile(file)
	}
	if s.configReader != nil {
		if err := s.v.ReadConfig(s.configReader); err != nil {
			return kerrors.WithKind(err, ErrorInvalidConfig, "Failed to read in config")
		}
	} else {
		if err := s.v.ReadInConfig(); err != nil {
			return kerrors.WithKind(err, ErrorInvalidConfig, "Failed to read in config")
		}
	}

	s.logger.level = s.v.GetString("logger.level")
	s.logger.output = s.v.GetString("logger.output")

	s.config.BaseURL = s.v.GetString("http.baseurl")
	s.httpClient.baseurl = s.config.BaseURL
	if t, err := time.ParseDuration(s.v.GetString("http.timeout")); err == nil {
		s.httpClient.timeout = t
	} else {
		return kerrors.WithKind(err, ErrorInvalidConfig, "Invalid http client timeout")
	}

	return nil
}

type (
	ClientConfigReader interface {
		Config() ClientConfig
		ConfigValueReader
	}

	clientConfigReader struct {
		s *clientSettings
		v *configValueReader
	}
)

func (r *clientConfigReader) Config() ClientConfig {
	return r.s.config
}

func (r *clientConfigReader) Name() string {
	return r.v.Name()
}

func (r *clientConfigReader) URL() string {
	return r.v.URL()
}

func (r *clientConfigReader) GetBool(key string) bool {
	return r.v.GetBool(key)
}

func (r *clientConfigReader) GetInt(key string) int {
	return r.v.GetInt(key)
}

func (r *clientConfigReader) GetDuration(key string) (time.Duration, error) {
	return r.v.GetDuration(key)
}

func (r *clientConfigReader) GetStr(key string) string {
	return r.v.GetStr(key)
}

func (r *clientConfigReader) GetStrSlice(key string) []string {
	return r.v.GetStrSlice(key)
}

func (r *clientConfigReader) Unmarshal(key string, val interface{}) error {
	return r.v.Unmarshal(key, val)
}

func (s *clientSettings) reader(opt serviceOpt) ClientConfigReader {
	return &clientConfigReader{
		s: s,
		v: &configValueReader{
			opt: opt,
			v:   s.v,
		},
	}
}
