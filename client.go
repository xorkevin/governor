package governor

import (
	"bytes"
	"encoding/json"
	"github.com/spf13/viper"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

type (
	ClientFlags struct {
		ConfigFile string
	}

	Client struct {
		config *viper.Viper
		httpc  *http.Client
		flags  ClientFlags
		addr   string
	}
)

func NewClient(opts Opts) *Client {
	v := viper.New()
	v.SetDefault("addr", "http://localhost:8080")
	v.SetDefault("timeout", "5s")

	v.SetConfigName(opts.ClientDefault)
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath(path.Join(".", "config"))
	if cfgdir, err := os.UserConfigDir(); err == nil {
		v.AddConfigPath(path.Join(cfgdir, opts.Appname))
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

func (c *Client) SetFlags(flags ClientFlags) {
	c.flags = flags
}

func (c *Client) Init() error {
	if file := c.flags.ConfigFile; file != "" {
		c.config.SetConfigFile(file)
	}
	if err := c.config.ReadInConfig(); err != nil {
		return NewError("Failed to read in config", http.StatusInternalServerError, err)
	}
	c.addr = c.config.GetString("addr")
	if t, err := time.ParseDuration(c.config.GetString("timeout")); err == nil {
		c.httpc.Timeout = t
	}
	return nil
}

func (c *Client) Request(method, path string, data interface{}, response interface{}) (interface{}, error) {
	var body io.Reader
	if data != nil {
		b := &bytes.Buffer{}
		if err := json.NewEncoder(b).Encode(data); err != nil {
			return nil, NewError("Failed to encode body to json", http.StatusBadRequest, err)
		}
		body = b
	}
	req, err := http.NewRequest(method, c.addr+path, body)
	if err != nil {
		return nil, NewError("Malformed request", http.StatusBadRequest, err)
	}
	res, err := c.httpc.Do(req)
	if err != nil {
		return nil, NewError("Failed request", http.StatusInternalServerError, err)
	}
	defer res.Body.Close()
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		return nil, NewError("Failed decoding response", http.StatusInternalServerError, err)
	}
	return response, nil
}
