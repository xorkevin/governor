package governor

import (
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"io"
)

type (
	// Config is the server configuration
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
		TemplateDir   string
		MaxReqSize    string
		FrontendProxy []string
		Origins       []string
		RouteRewrite  map[string]string
	}
)

func newConfig(defaultConfFile string, appname, version, versionhash, envPrefix string) Config {
	configFilename := pflag.String("config", defaultConfFile, "name of config file in config directory")
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

	v.SetEnvPrefix(envPrefix)
	v.AutomaticEnv()

	return Config{
		config:      v,
		Appname:     appname,
		Version:     version,
		VersionHash: versionhash,
	}
}

func (c *Config) init() error {
	if err := c.config.ReadInConfig(); err != nil {
		return err
	}
	c.LogLevel = envToLevel(c.config.GetString("mode"))
	c.LogOutput = envToLogOutput(c.config.GetString("logoutput"))
	c.Port = c.config.GetString("port")
	c.BaseURL = c.config.GetString("baseurl")
	c.PublicDir = c.config.GetString("publicdir")
	c.TemplateDir = c.config.GetString("templatedir")
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
