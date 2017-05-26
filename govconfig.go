package governor

import (
	"fmt"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"strings"
)

type (
	// Config is the server configuration
	Config struct {
		Version     string
		LogLevel    int
		Port        string
		PostgresURL string
	}
)

// IsDebug returns if the configuration is in debug mode
func (c *Config) IsDebug() bool {
	return c.LogLevel == levelDebug
}

// NewConfig creates a new server configuration
// It requires ENV vars:
//   VERSION
//   MODE
//   POSTGRES_URL
func NewConfig(confFilenameDefault string) (Config, error) {
	configFilename := pflag.String("config", confFilenameDefault, "name of config file in config directory")
	pflag.Parse()

	v := viper.New()
	v.SetDefault("version", "version")
	v.SetDefault("mode", "INFO")
	v.SetDefault("port", "8080")
	v.SetDefault("postgres.user", "postgres")
	v.SetDefault("postgres.password", "admin")
	v.SetDefault("postgres.dbname", "governor")
	v.SetDefault("postgres.host", "localhost")
	v.SetDefault("postgres.port", "5432")
	v.SetDefault("postgres.sslmode", "disable")

	v.SetConfigName(*configFilename)
	v.AddConfigPath("./config")
	v.AddConfigPath(".")
	v.SetConfigType("yaml")

	if err := v.ReadInConfig(); err != nil {
		fmt.Println(err)
		return Config{}, err
	}

	pgconf := v.GetStringMapString("postgres")

	pgarr := []string{}

	for k, v := range pgconf {
		pgarr = append(pgarr, k+"="+v)
	}

	return Config{
		Version:     v.GetString("version"),
		LogLevel:    envToLevel(v.GetString("mode")),
		Port:        v.GetString("port"),
		PostgresURL: strings.Join(pgarr, " "),
	}, nil
}
