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
		mu            *sync.RWMutex
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
		mu:          &sync.RWMutex{},
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
		if err := c.authVault(); err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) ensureValidAuth() error {
	if !c.vaultK8sAuth {
		return nil
	}
	if c.authVaultValid() {
		return nil
	}
	return c.authVault()
}

func (c *Config) authVaultValidLocked() bool {
	return c.vaultExpire-time.Now().Round(0).Unix() > 5
}

func (c *Config) authVaultValid() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.authVaultValidLocked()
}

func (c *Config) authVault() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.authVaultValidLocked() {
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
		SecretReader
	}

	// SecretReader gets values from a secret engine
	SecretReader interface {
		GetSecret(key string) (vaultSecretVal, error)
		InvalidateSecret(key string)
	}

	vaultSecretVal map[string]interface{}

	vaultSecret struct {
		key    string
		value  vaultSecretVal
		expire int64
	}

	configReader struct {
		serviceOpt
		c     *Config
		cache map[string]vaultSecret
		mu    sync.RWMutex
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
	return r.c.config.GetStringMapString(key)
}

func (r *configReader) GetBool(key string) bool {
	return r.c.config.GetBool(r.name + "." + key)
}

func (r *configReader) GetInt(key string) int {
	return r.c.config.GetInt(r.name + "." + key)
}

func (r *configReader) GetStr(key string) string {
	return r.c.config.GetString(r.name + "." + key)
}

func (r *configReader) GetStrSlice(key string) []string {
	return r.c.config.GetStringSlice(r.name + "." + key)
}

func (s *vaultSecret) isValid() bool {
	return s.expire == 0 || s.expire-time.Now().Round(0).Unix() > 5
}

func (r *configReader) GetSecret(key string) (vaultSecretVal, error) {
	kvpath := r.GetStr(key)
	if kvpath == "" {
		return nil, NewError("Invalid secret key", http.StatusInternalServerError, nil)
	}

	if v, ok := r.getCacheSecret(key); ok {
		return v, nil
	}

	if err := r.c.ensureValidAuth(); err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if v, ok := r.getCacheSecretLocked(key); ok {
		return v, nil
	}

	vault := r.c.vault.Logical()
	s, err := vault.Read(kvpath)
	if err != nil {
		return nil, NewError("Failed to read vault secret", http.StatusInternalServerError, err)
	}

	var expire int64
	if s.LeaseDuration > 0 {
		expire = time.Now().Round(0).Unix() + int64(s.LeaseDuration)
	}
	r.setCacheSecretLocked(key, s.Data, expire)

	return s.Data, nil
}

func (r *configReader) InvalidateSecret(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.cache, key)
}

func (r *configReader) getCacheSecretLocked(key string) (vaultSecretVal, bool) {
	s, ok := r.cache[key]
	if !ok {
		return nil, false
	}
	if !s.isValid() {
		return nil, false
	}
	return s.value, true
}

func (r *configReader) getCacheSecret(key string) (vaultSecretVal, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.getCacheSecretLocked(key)
}

func (r *configReader) setCacheSecretLocked(key string, value vaultSecretVal, expire int64) {
	r.cache[key] = vaultSecret{
		key:    key,
		value:  value,
		expire: expire,
	}
}

func (c *Config) reader(opt serviceOpt) ConfigReader {
	return &configReader{
		serviceOpt: opt,
		c:          c,
		cache:      map[string]vaultSecret{},
	}
}
