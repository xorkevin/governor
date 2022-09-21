package governor

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
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
			Timeout: 15 * time.Second,
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
		return kerrors.WithKind(err, ErrorInvalidConfig{}, "Failed to read in config")
	}
	c.addr = c.config.GetString("addr")
	if t, err := time.ParseDuration(c.config.GetString("timeout")); err == nil {
		c.httpc.Timeout = t
	} else {
		log.Println("Invalid http client timeout:", err)
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
	req, err := http.NewRequest(method, c.addr+path, body)
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
