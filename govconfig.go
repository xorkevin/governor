package governor

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	vaultapi "github.com/hashicorp/vault/api"
	"github.com/spf13/viper"
)

type (
	// Version is the app version
	Version struct {
		Num  string
		Hash string
	}

	// Opts is the static server configuration
	Opts struct {
		Version
		Appname       string
		Description   string
		DefaultFile   string
		ClientDefault string
		ClientPrefix  string
		EnvPrefix     string
	}
)

func (v Version) String() string {
	return v.Num + "-" + v.Hash
}

type (
	// Config is the server configuration including those from a config file and
	// environment variables
	Config struct {
		config         *viper.Viper
		vault          *vaultapi.Client
		vaultK8sAuth   bool
		vaultRole      string
		vaultJWT       string
		vaultLoginPath string
		vaultExpire    int64
		vaultCache     map[string]vaultSecret
		mu             *sync.RWMutex
		appname        string
		version        Version
		showBanner     bool
		logLevel       int
		logOutput      io.Writer
		maxReqSize     string
		maxHeaderSize  string
		maxConnRead    string
		maxConnHeader  string
		maxConnWrite   string
		maxConnIdle    string
		origins        []string
		allowpaths     []*corsPathRule
		rewrite        []*rewriteRule
		Port           string
		BaseURL        string
	}

	corsPathRule struct {
		pattern string
		regex   *regexp.Regexp
	}

	rewriteRule struct {
		Host      string   `mapstructure:"host"`
		Methods   []string `mapstructure:"methods"`
		Pattern   string   `mapstructure:"pattern"`
		Replace   string   `mapstructure:"replace"`
		regex     *regexp.Regexp
		methodset map[string]struct{}
	}
)

func (r *corsPathRule) init() error {
	k, err := regexp.Compile(r.pattern)
	if err != nil {
		return err
	}
	r.regex = k
	return nil
}

func (r *corsPathRule) match(req *http.Request) bool {
	return r.regex.MatchString(req.URL.Path)
}

func (r *rewriteRule) init() error {
	k, err := regexp.Compile(r.Pattern)
	if err != nil {
		return err
	}
	r.regex = k
	s := make(map[string]struct{}, len(r.Methods))
	for _, i := range r.Methods {
		s[i] = struct{}{}
	}
	r.methodset = s
	return nil
}

func (r *rewriteRule) match(req *http.Request) bool {
	if r.Host != "" && req.Host != r.Host {
		return false
	}
	if len(r.methodset) != 0 {
		if _, ok := r.methodset[req.Method]; !ok {
			return false
		}
	}
	return true
}

func (r *rewriteRule) replace(src string) string {
	return r.regex.ReplaceAllString(src, r.Replace)
}

func (r rewriteRule) String() string {
	return fmt.Sprintf("Host: %s, Methods: %s, Pattern: %s, Replace: %s", r.Host, strings.Join(r.Methods, " "), r.Pattern, r.Replace)
}

func newConfig(opts Opts) *Config {
	v := viper.New()
	v.SetDefault("mode", "INFO")
	v.SetDefault("logoutput", "STDOUT")
	v.SetDefault("banner", true)
	v.SetDefault("port", "8080")
	v.SetDefault("baseurl", "/")
	v.SetDefault("templatedir", "templates")
	v.SetDefault("maxreqsize", "2M")
	v.SetDefault("maxheadersize", "1M")
	v.SetDefault("maxconnread", "5s")
	v.SetDefault("maxconnheader", "2s")
	v.SetDefault("maxconnwrite", "5s")
	v.SetDefault("maxconnidle", "5s")
	v.SetDefault("alloworigins", []string{})
	v.SetDefault("allowpaths", []string{})
	v.SetDefault("routerewrite", []*rewriteRule{})
	v.SetDefault("vault.addr", "")
	v.SetDefault("vault.k8s.auth", false)
	v.SetDefault("vault.k8s.role", "")
	v.SetDefault("vault.k8s.loginpath", "/auth/kubernetes/login")
	v.SetDefault("vault.k8s.jwtpath", "/var/run/secrets/kubernetes.io/serviceaccount/token")

	v.SetConfigName(opts.DefaultFile)
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath(filepath.Join(".", "config"))
	if cfgdir, err := os.UserConfigDir(); err == nil {
		v.AddConfigPath(filepath.Join(cfgdir, opts.Appname))
	}

	v.SetEnvPrefix(opts.EnvPrefix)
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "__"))

	return &Config{
		config:     v,
		mu:         &sync.RWMutex{},
		appname:    opts.Appname,
		version:    opts.Version,
		vaultCache: map[string]vaultSecret{},
	}
}

func (c *Config) setConfigFile(file string) {
	c.config.SetConfigFile(file)
}

type (
	// ErrInvalidConfig is returned when the config is invalid
	ErrInvalidConfig struct{}
	// ErrVault is returned when failing to contact vault
	ErrVault struct{}
)

func (e ErrInvalidConfig) Error() string {
	return "Invalid config"
}

func (e ErrVault) Error() string {
	return "Failed vault request"
}

func (c *Config) init() error {
	if err := c.config.ReadInConfig(); err != nil {
		return ErrWithKind(err, ErrInvalidConfig{}, "Failed to read in config")
	}
	c.showBanner = c.config.GetBool("banner")
	c.logLevel = envToLevel(c.config.GetString("mode"))
	c.logOutput = envToLogOutput(c.config.GetString("logoutput"))
	c.maxReqSize = c.config.GetString("maxreqsize")
	c.maxHeaderSize = c.config.GetString("maxheadersize")
	c.maxConnRead = c.config.GetString("maxconnread")
	c.maxConnHeader = c.config.GetString("maxconnheader")
	c.maxConnWrite = c.config.GetString("maxconnwrite")
	c.maxConnIdle = c.config.GetString("maxconnidle")
	c.origins = c.config.GetStringSlice("alloworigins")
	allowPathPatterns := c.config.GetStringSlice("allowpaths")
	c.allowpaths = make([]*corsPathRule, 0, len(allowPathPatterns))
	for _, i := range allowPathPatterns {
		c.allowpaths = append(c.allowpaths, &corsPathRule{
			pattern: i,
		})
	}
	rewrite := []*rewriteRule{}
	if err := c.config.UnmarshalKey("routerewrite", &rewrite); err != nil {
		return err
	}
	c.rewrite = rewrite
	c.Port = c.config.GetString("port")
	c.BaseURL = c.config.GetString("baseurl")
	if err := c.initvault(); err != nil {
		return err
	}
	return nil
}

func (c *Config) initvault() error {
	vaultconfig := vaultapi.DefaultConfig()
	if err := vaultconfig.Error; err != nil {
		return err
	}
	if vaddr := c.config.GetString("vault.addr"); vaddr != "" {
		vaultconfig.Address = vaddr
	}
	vault, err := vaultapi.NewClient(vaultconfig)
	if err != nil {
		return ErrWithKind(err, ErrInvalidConfig{}, "Failed to create vault client")
	}
	c.vault = vault
	if c.config.GetBool("vault.k8s.auth") {
		c.vaultK8sAuth = true

		role := c.config.GetString("vault.k8s.role")
		loginpath := c.config.GetString("vault.k8s.loginpath")
		jwtpath := c.config.GetString("vault.k8s.jwtpath")
		if role == "" {
			return ErrWithKind(nil, ErrInvalidConfig{}, "No vault role set")
		}
		if loginpath == "" {
			return ErrWithKind(nil, ErrInvalidConfig{}, "No vault k8s login path set")
		}
		if jwtpath == "" {
			return ErrWithKind(nil, ErrInvalidConfig{}, "No path for vault k8s service account jwt auth")
		}
		jwtbytes, err := os.ReadFile(jwtpath)
		if err != nil {
			return ErrWithKind(err, ErrInvalidConfig{}, "Failed to read vault k8s service account jwt")
		}
		jwt := string(jwtbytes)
		c.vaultRole = role
		c.vaultLoginPath = loginpath
		c.vaultJWT = jwt

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
	authsecret, err := vault.Write(c.vaultLoginPath, map[string]interface{}{
		"jwt":  c.vaultJWT,
		"role": c.vaultRole,
	})
	if err != nil {
		return ErrWithKind(err, ErrVault{}, "Failed to auth with vault k8s")
	}
	c.vaultExpire = time.Now().Round(0).Unix() + int64(authsecret.Auth.LeaseDuration)
	c.vault.SetToken(authsecret.Auth.ClientToken)
	return nil
}

func (c *Config) getSecret(key string) (vaultSecretVal, error) {
	if s, ok := c.vaultCache[key]; ok && s.isValid() {
		return s.value, nil
	}

	kvpath := c.config.GetString(key)
	if kvpath == "" {
		return nil, ErrWithKind(nil, ErrInvalidConfig{}, "Empty secret key "+key)
	}

	if err := c.ensureValidAuth(); err != nil {
		return nil, err
	}

	vault := c.vault.Logical()
	s, err := vault.Read(kvpath)
	if err != nil {
		return nil, ErrWithKind(err, ErrVault{}, "Failed to read vault secret")
	}

	data := s.Data
	if v, ok := data["data"].(map[string]interface{}); ok {
		data = v
	}

	var expire int64
	if s.LeaseDuration > 0 {
		expire = time.Now().Round(0).Unix() + int64(s.LeaseDuration)
		k := c.vaultExpire
		if expire > k {
			expire = k
		}
	}
	c.vaultCache[key] = vaultSecret{
		key:    key,
		value:  data,
		expire: expire,
	}

	return data, nil
}

func (c *Config) invalidateSecret(key string) {
	delete(c.vaultCache, key)
}

// IsDebug returns if the configuration is in debug mode
func (c *Config) IsDebug() bool {
	return c.logLevel == levelDebug
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
		GetBool(key string) bool
		GetInt(key string) int
		GetStr(key string) string
		GetStrSlice(key string) []string
		GetStrMap(key string) map[string]string
		Unmarshal(key string, val interface{}) error
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
		c *Config
	}
)

func (r *configReader) Name() string {
	return r.name
}

func (r *configReader) URL() string {
	return r.url
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

func (r *configReader) GetStrMap(key string) map[string]string {
	if key == "" {
		key = r.name
	} else {
		key = r.name + "." + key
	}
	return r.c.config.GetStringMapString(key)
}

func (r *configReader) Unmarshal(key string, val interface{}) error {
	return r.c.config.UnmarshalKey(r.name+"."+key, val)
}

func (s *vaultSecret) isValid() bool {
	return s.expire == 0 || s.expire-time.Now().Round(0).Unix() > 5
}

func (r *configReader) GetSecret(key string) (vaultSecretVal, error) {
	return r.c.getSecret(r.name + "." + key)
}

func (r *configReader) InvalidateSecret(key string) {
	r.c.invalidateSecret(r.name + "." + key)
}

func (c *Config) reader(opt serviceOpt) ConfigReader {
	return &configReader{
		serviceOpt: opt,
		c:          c,
	}
}
