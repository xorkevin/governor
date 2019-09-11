package governor

import (
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"io"
	"net/http"
)

type (
	// ConfigOpts is the server base configuration
	ConfigOpts struct {
		DefaultFile string
		Appname     string
		Description string
		Version     string
		VersionHash string
		EnvPrefix   string
	}

	// Config is the complete server configuration including the dynamic
	// (runtime) options
	Config struct {
		config        *viper.Viper
		Appname       string
		Version       string
		VersionHash   string
		LogLevel      int
		LogOutput     io.Writer
		Port          string
		BaseURL       string
		PublicDir     string
		MaxReqSize    string
		FrontendProxy []string
		Origins       []string
		RouteRewrite  map[string]string
	}
)

func newConfig(conf ConfigOpts) Config {
	configFilename := pflag.String("config", conf.DefaultFile, "name of config file in config directory")
	pflag.Parse()

	v := viper.New()
	v.SetDefault("mode", "INFO")
	v.SetDefault("logoutput", "STDOUT")
	v.SetDefault("port", "8080")
	v.SetDefault("baseurl", "/")
	v.SetDefault("publicdir", "public")
	v.SetDefault("templatedir", "templates")
	v.SetDefault("maxreqsize", "2M")
	v.SetDefault("frontendproxy", []string{})
	v.SetDefault("alloworigins", []string{})

	v.SetConfigName(*configFilename)
	v.AddConfigPath("./config")
	v.AddConfigPath(".")
	v.SetConfigType("yaml")

	v.SetEnvPrefix(conf.EnvPrefix)
	v.AutomaticEnv()

	return Config{
		config:      v,
		Appname:     conf.Appname,
		Version:     conf.Version,
		VersionHash: conf.VersionHash,
	}
}

func (c *Config) init() error {
	if err := c.config.ReadInConfig(); err != nil {
		return NewError("Failed to read in config", http.StatusInternalServerError, err)
	}
	c.LogLevel = envToLevel(c.config.GetString("mode"))
	c.LogOutput = envToLogOutput(c.config.GetString("logoutput"))
	c.Port = c.config.GetString("port")
	c.BaseURL = c.config.GetString("baseurl")
	c.PublicDir = c.config.GetString("publicdir")
	c.MaxReqSize = c.config.GetString("maxreqsize")
	c.FrontendProxy = c.config.GetStringSlice("frontendproxy")
	c.Origins = c.config.GetStringSlice("alloworigins")
	c.RouteRewrite = c.config.GetStringMapString("routerewrite")
	return nil
}

// IsDebug returns if the configuration is in debug mode
func (c *Config) IsDebug() bool {
	return c.LogLevel == levelDebug
}

type (
	// ConfigRegistrar sets default values on the config parser
	ConfigRegistrar interface {
		SetDefault(key string, value interface{})
	}

	configRegistrar struct {
		prefix string
		v      *viper.Viper
	}
)

func (r *configRegistrar) SetDefault(key string, value interface{}) {
	r.v.SetDefault(r.prefix+"."+key, value)
}

func (c *Config) registrar(prefix string) ConfigRegistrar {
	return &configRegistrar{
		prefix: prefix,
		v:      c.config,
	}
}

type (
	// ConfigReader gets values from the config parser
	ConfigReader interface {
		Name() string
		URL() string
		GetStrMap(key string) map[string]string
		GetBool(key string) bool
		GetInt(key string) int
		GetStr(key string) string
		GetStrSlice(key string) []string
	}

	configReader struct {
		serviceOpt
		v *viper.Viper
	}
)

func (r *configReader) Name() string {
	return r.name
}

func (r *configReader) URL() string {
	return r.url
}

func (r *configReader) GetStrMap(key string) map[string]string {
	if key == "" {
		key = r.name
	} else {
		key = r.name + "." + key
	}
	return r.v.GetStringMapString(key)
}

func (r *configReader) GetBool(key string) bool {
	return r.v.GetBool(r.name + "." + key)
}

func (r *configReader) GetInt(key string) int {
	return r.v.GetInt(r.name + "." + key)
}

func (r *configReader) GetStr(key string) string {
	return r.v.GetString(r.name + "." + key)
}

func (r *configReader) GetStrSlice(key string) []string {
	return r.v.GetStringSlice(r.name + "." + key)
}

func (c *Config) reader(opt serviceOpt) ConfigReader {
	return &configReader{
		serviceOpt: opt,
		v:          c.config,
	}
}
