package governor

import (
	"io"
	"strings"
	"time"

	"github.com/spf13/viper"
	"xorkevin.dev/kerrors"
)

type (
	clientSettings struct {
		v            *viper.Viper
		configReader io.Reader
		config       ClientConfig
		logger       configLogger
		httpClient   configHTTPClient
	}

	// ClientConfig is the client config
	ClientConfig struct {
		BaseURL string
	}
)

func newClientSettings(opts Opts) *clientSettings {
	v := viper.New()
	v.SetDefault("logger.level", "INFO")
	v.SetDefault("http.baseurl", "http://localhost:8080/api")
	v.SetDefault("http.timeout", "15s")

	v.SetConfigName(opts.ClientDefault)
	v.AddConfigPath(".")

	v.SetEnvPrefix(opts.ClientPrefix)
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "__"))

	return &clientSettings{
		v:            v,
		configReader: opts.ConfigReader,
		httpClient: configHTTPClient{
			transport: opts.HTTPTransport,
		},
	}
}

func (s *clientSettings) init(flags ClientFlags) error {
	if file := flags.ConfigFile; file != "" {
		s.v.SetConfigFile(file)
	}
	if s.configReader != nil {
		s.v.SetConfigType("json")
		if err := s.v.ReadConfig(s.configReader); err != nil {
			return kerrors.WithKind(err, ErrInvalidConfig, "Failed to read in config")
		}
	} else {
		if err := s.v.ReadInConfig(); err != nil {
			return kerrors.WithKind(err, ErrInvalidConfig, "Failed to read in config")
		}
	}

	s.logger.level = s.v.GetString("logger.level")

	s.config.BaseURL = s.v.GetString("http.baseurl")
	s.httpClient.baseurl = s.config.BaseURL
	if t, err := time.ParseDuration(s.v.GetString("http.timeout")); err == nil {
		s.httpClient.timeout = t
	} else {
		return kerrors.WithKind(err, ErrInvalidConfig, "Invalid http client timeout")
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
