package governor

import (
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type (
	// Config is the server configuration
	Config struct {
		config    *viper.Viper
		Appname   string
		Version   string
		LogLevel  int
		Port      string
		BaseURL   string
		PublicDir string
		Origins   []string
	}
)

// IsDebug returns if the configuration is in debug mode
func (c *Config) IsDebug() bool {
	return c.LogLevel == levelDebug
}

// Conf returns the config instance
func (c *Config) Conf() *viper.Viper {
	return c.config
}

// Init reads in the config
func (c *Config) Init() error {
	if err := c.config.ReadInConfig(); err != nil {
		return err
	}
	c.Appname = c.config.GetString("appname")
	c.Version = c.config.GetString("version")
	c.LogLevel = envToLevel(c.config.GetString("mode"))
	c.Port = c.config.GetString("port")
	c.BaseURL = c.config.GetString("baseurl")
	c.PublicDir = c.config.GetString("publicdir")
	origins := c.config.GetStringSlice("allow_origins")
	if len(origins) == 0 {
		// this effectively will not match any origin
		c.Origins = []string{""}
	} else {
		c.Origins = origins
	}
	return nil
}

// NewConfig creates a new server configuration
func NewConfig(confFilenameDefault string) (Config, error) {
	configFilename := pflag.String("config", confFilenameDefault, "name of config file in config directory")
	pflag.Parse()

	v := viper.New()
	v.SetDefault("appname", "governor")
	v.SetDefault("version", "version")
	v.SetDefault("mode", "INFO")
	v.SetDefault("port", "8080")
	v.SetDefault("baseurl", "/")
	v.SetDefault("publicdir", "public")
	v.SetDefault("allow_origins", []string{""})

	v.SetConfigName(*configFilename)
	v.AddConfigPath("./config")
	v.AddConfigPath(".")
	v.SetConfigType("yaml")

	v.SetEnvPrefix("gov")
	v.AutomaticEnv()

	return Config{
		config: v,
	}, nil
}
