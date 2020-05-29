package governor

import (
	"context"
	vaultapi "github.com/hashicorp/vault/api"
	"github.com/spf13/viper"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"sync"
	"time"
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
		vault         *vaultapi.Client
		vaultK8sAuth  bool
		vaultExpire   int64
		mu            sync.RWMutex
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

func newConfig(conf ConfigOpts) *Config {
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
	v.SetDefault("vault.addr", "")
	v.SetDefault("vault.k8s.auth", false)
	v.SetDefault("vault.k8s.role", "")
	v.SetDefault("vault.k8s.loginpath", "/auth/kubernetes/login")
	v.SetDefault("vault.k8s.jwtpath", "/var/run/secrets/kubernetes.io/serviceaccount/token")

	v.SetConfigName(conf.DefaultFile)
	v.AddConfigPath(".")
	v.AddConfigPath(path.Join(".", "config"))
	if cfgdir, err := os.UserConfigDir(); err == nil {
		v.AddConfigPath(path.Join(cfgdir, conf.Appname))
	}
	v.SetConfigType("yaml")

	v.SetEnvPrefix(conf.EnvPrefix)
	v.AutomaticEnv()

	return &Config{
		config:      v,
		Appname:     conf.Appname,
		Version:     conf.Version,
		VersionHash: conf.VersionHash,
	}
}

func (c *Config) setConfigFile(file string) {
	c.config.SetConfigFile(file)
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

func (c *Config) initvault(ctx context.Context, l Logger) error {
	vconfig := c.config.GetStringMapString("vault")
	vaultconfig := vaultapi.DefaultConfig()
	if err := vaultconfig.Error; err != nil {
		l.Warn("error creating vault config", map[string]string{
			"phase":      "init",
			"error":      err.Error(),
			"actiontype": "vaultdefaultconfig",
		})
	}
	if vaddr := vconfig["addr"]; vaddr != "" {
		vaultconfig.Address = vaddr
	}
	vault, err := vaultapi.NewClient(vaultconfig)
	if err != nil {
		return NewError("Failed to create vault client", http.StatusInternalServerError, err)
	}
	c.vault = vault
	if c.config.GetBool("vault.k8s.auth") {
		c.vaultK8sAuth = true
		if err := c.authk8s(); err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) ensureValidAuth() error {
	if !c.vaultK8sAuth {
		return nil
	}
	if c.authk8sValid() {
		return nil
	}
	return c.authk8s()
}

func (c *Config) authk8sValidLocked() bool {
	return c.vaultExpire-time.Now().Round(0).Unix() > 5
}

func (c *Config) authk8sValid() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.authk8sValidLocked()
}

func (c *Config) authk8s() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.authk8sValidLocked() {
		return nil
	}

	vault := c.vault.Logical()
	kconfig := c.config.GetStringMapString("vault.k8s")
	role := kconfig["role"]
	loginpath := kconfig["loginpath"]
	jwtpath := kconfig["jwtpath"]
	if role == "" {
		return NewError("No vault role set", http.StatusBadRequest, nil)
	}
	if loginpath == "" {
		return NewError("No vault k8s login path set", http.StatusBadRequest, nil)
	}
	if jwtpath == "" {
		return NewError("No path for vault k8s service account jwt auth", http.StatusBadRequest, nil)
	}
	jwtbytes, err := ioutil.ReadFile(jwtpath)
	if err != nil {
		return NewError("Failed to read vault k8s service account jwt", http.StatusInternalServerError, err)
	}
	jwt := string(jwtbytes)
	authsecret, err := vault.Write(loginpath, map[string]interface{}{
		"jwt":  jwt,
		"role": role,
	})
	if err != nil {
		return NewError("Failed to auth with vault k8s", http.StatusInternalServerError, err)
	}
	c.vaultExpire = time.Now().Round(0).Unix() + int64(authsecret.Auth.LeaseDuration)
	c.vault.SetToken(authsecret.Auth.ClientToken)
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
		GetSecret(key string) (string, error)
	}

	configReader struct {
		serviceOpt
		v     *viper.Viper
		vault *vaultapi.Client
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

func (r *configReader) GetSecret(key string) (string, error) {
	kvpath := r.GetStr(key)
	if kvpath == "" {
		return "", NewError("Invalid secret key", http.StatusInternalServerError, nil)
	}
	return "", nil
}

func (c *Config) reader(opt serviceOpt) ConfigReader {
	return &configReader{
		serviceOpt: opt,
		v:          c.config,
		vault:      c.vault,
	}
}
