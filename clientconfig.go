package governor

import (
	"io"
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
)

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
