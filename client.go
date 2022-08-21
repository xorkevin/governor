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
	"xorkevin.dev/kerrors"
)

type (
	// ClientFlags are flags for the client cmd
	ClientFlags struct {
		ConfigFile string
	}

	// Client is a server client
	Client struct {
		config *viper.Viper
		httpc  *http.Client
		flags  ClientFlags
		addr   string
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
		config: v,
		httpc: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// SetFlags sets Client flags
func (c *Client) SetFlags(flags ClientFlags) {
	c.flags = flags
}

// Init initializes the Client by reading a config
func (c *Client) Init() error {
	if file := c.flags.ConfigFile; file != "" {
		c.config.SetConfigFile(file)
	}
	if err := c.config.ReadInConfig(); err != nil {
		return kerrors.WithKind(err, ErrInvalidConfig{}, "Failed to read in config")
	}
	c.addr = c.config.GetString("addr")
	if t, err := time.ParseDuration(c.config.GetString("timeout")); err == nil {
		c.httpc.Timeout = t
	}
	return nil
}

type (
	// ErrInvalidClientReq is returned when the client request could not be made
	ErrInvalidClientReq struct{}
	// ErrInvalidServerRes is returned on an invalid server response
	ErrInvalidServerRes struct{}
	// ErrServerRes is a returned server error
	ErrServerRes struct{}
)

func (e ErrInvalidClientReq) Error() string {
	return "Invalid client request"
}

func (e ErrInvalidServerRes) Error() string {
	return "Invalid server response"
}

func (e ErrServerRes) Error() string {
	return "Error server response"
}

// Request sends a request to the server
func (c *Client) Request(method, path string, data interface{}, response interface{}) (int, error) {
	var body io.Reader
	if data != nil {
		b := &bytes.Buffer{}
		if err := json.NewEncoder(b).Encode(data); err != nil {
			return 0, kerrors.WithKind(err, ErrInvalidClientReq{}, "Failed to encode body to json")
		}
		body = b
	}
	req, err := http.NewRequest(method, c.addr+path, body)
	if body != nil {
		req.Header.Add("Content-Type", "application/json")
	}
	if err != nil {
		return 0, kerrors.WithKind(err, ErrInvalidClientReq{}, "Malformed request")
	}
	res, err := c.httpc.Do(req)
	if err != nil {
		return 0, kerrors.WithKind(err, ErrInvalidClientReq{}, "Failed request")
	}
	defer res.Body.Close()
	if res.StatusCode >= http.StatusBadRequest {
		var errres ErrorRes
		if err := json.NewDecoder(res.Body).Decode(&errres); err != nil {
			return 0, kerrors.WithKind(err, ErrInvalidServerRes{}, "Failed decoding response")
		}
		return res.StatusCode, kerrors.WithKind(nil, ErrServerRes{}, errres.Message)
	} else if err := json.NewDecoder(res.Body).Decode(response); err != nil {
		return 0, kerrors.WithKind(err, ErrInvalidServerRes{}, "Failed decoding response")
	}
	return res.StatusCode, nil
}

func isStatusOK(status int) bool {
	return status >= http.StatusOK && status < http.StatusMultipleChoices
}

// Setup sends a setup request to the server
func (c *Client) Setup(req ReqSetup) (*ResponseSetup, error) {
	res := &ResponseSetup{}
	if status, err := c.Request("POST", "/setupz", req, res); err != nil {
		return nil, err
	} else if !isStatusOK(status) {
		return nil, kerrors.WithKind(nil, ErrServerRes{}, "Non success response")
	}
	return res, nil
}
