package governor

import (
	"context"
	"encoding/json"
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
	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/kerrors"
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
	// secretsClient is a client that reads secrets
	secretsClient interface {
		Info() string
		Init() error
		GetSecret(ctx context.Context, kvpath string) (map[string]interface{}, int64, error)
	}

	// System event channels
	SysChannels struct {
		GC string
	}

	// SysEventTimestampProps
	SysEventTimestampProps struct {
		Timestamp int64 `json:"timestamp"`
	}

	// Config is the server configuration including those from a config file and
	// environment variables
	Config struct {
		config        *viper.Viper
		vault         secretsClient
		vaultCache    map[string]vaultSecret
		appname       string
		version       Version
		showBanner    bool
		logLevel      string
		logOutput     string
		logWriter     io.Writer
		maxReqSize    string
		maxHeaderSize string
		maxConnRead   string
		maxConnHeader string
		maxConnWrite  string
		maxConnIdle   string
		origins       []string
		allowpaths    []*corsPathRule
		rewrite       []*rewriteRule
		proxies       []string
		Port          string
		BaseURL       string
		Hostname      string
		Instance      string
		SysChannels   SysChannels
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
	v.SetDefault("loglevel", "INFO")
	v.SetDefault("logoutput", "STDOUT")
	v.SetDefault("banner", true)
	v.SetDefault("port", "8080")
	v.SetDefault("baseurl", "/")
	v.SetDefault("maxreqsize", "2M")
	v.SetDefault("maxheadersize", "1M")
	v.SetDefault("maxconnread", "5s")
	v.SetDefault("maxconnheader", "2s")
	v.SetDefault("maxconnwrite", "5s")
	v.SetDefault("maxconnidle", "5s")
	v.SetDefault("alloworigins", []string{})
	v.SetDefault("allowpaths", []string{})
	v.SetDefault("routerewrite", []*rewriteRule{})
	v.SetDefault("proxies", []string{})
	v.SetDefault("vault.filesource", "")
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
		appname:    opts.Appname,
		version:    opts.Version,
		vaultCache: map[string]vaultSecret{},
		SysChannels: SysChannels{
			GC: opts.Appname + "." + "sys.gc",
		},
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

const (
	instanceIDRandSize = 8
)

func (c *Config) init() error {
	if err := c.config.ReadInConfig(); err != nil {
		return kerrors.WithKind(err, ErrInvalidConfig{}, "Failed to read in config")
	}
	c.showBanner = c.config.GetBool("banner")
	c.logLevel = c.config.GetString("loglevel")
	c.logOutput = c.config.GetString("logoutput")
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
	c.proxies = c.config.GetStringSlice("proxies")
	c.Port = c.config.GetString("port")
	c.BaseURL = c.config.GetString("baseurl")
	var err error
	c.Hostname, err = os.Hostname()
	if err != nil {
		return kerrors.WithMsg(err, "Failed to get hostname")
	}
	u, err := uid.NewSnowflake(instanceIDRandSize)
	if err != nil {
		return err
	}
	c.Instance = u.Base32()
	if err := c.initsecrets(); err != nil {
		return err
	}
	return nil
}

type (
	// secretsFileSource is a secretsClient reading from a static file
	secretsFileSource struct {
		path string
		data secretsFileData
	}

	secretsFileData struct {
		Data map[string]map[string]interface{} `yaml:"data"`
	}
)

// newSecretsFileSource creates a new secretsFileSource
func newSecretsFileSource(s string) (secretsClient, error) {
	b, err := os.ReadFile(s)
	if err != nil {
		return nil, kerrors.WithKind(err, ErrInvalidConfig{}, "Failed to read secrets file source")
	}
	data := secretsFileData{}
	if err := yaml.Unmarshal(b, &data); err != nil {
		return nil, kerrors.WithKind(err, ErrInvalidConfig{}, "Invalid secrets file source file")
	}
	return &secretsFileSource{
		path: s,
		data: data,
	}, nil
}

func (s *secretsFileSource) Info() string {
	return fmt.Sprintf("file source; path: %s", s.path)
}

func (s *secretsFileSource) Init() error {
	return nil
}

func (s *secretsFileSource) GetSecret(ctx context.Context, kvpath string) (map[string]interface{}, int64, error) {
	data, ok := s.data.Data[kvpath]
	if !ok {
		return nil, 0, kerrors.WithKind(nil, ErrVault{}, "Failed to read vault secret")
	}
	return data, 0, nil
}

type (
	// secretsVaultSourceConfig is a vault secrets client config
	secretsVaultSourceConfig struct {
		Addr         string
		K8SAuth      bool
		K8SRole      string
		K8SLoginPath string
		K8SJWT       string
	}

	// secretsVaultSource is a secretsClient reading from vault
	secretsVaultSource struct {
		address     string
		vault       *vaultapi.Client
		config      secretsVaultSourceConfig
		vaultExpire int64
		mu          *sync.RWMutex
	}
)

// NewSecretsVaultSource creates a new secretsVaultSource
func NewSecretsVaultSource(config secretsVaultSourceConfig) (secretsClient, error) {
	vconfig := vaultapi.DefaultConfig()
	if err := vconfig.Error; err != nil {
		return nil, kerrors.WithKind(err, ErrInvalidConfig{}, "Failed to create vault default config")
	}
	vconfig.Address = config.Addr
	vault, err := vaultapi.NewClient(vconfig)
	if err != nil {
		return nil, kerrors.WithKind(err, ErrInvalidConfig{}, "Failed to create vault client")
	}
	return &secretsVaultSource{
		address: config.Addr,
		vault:   vault,
		config:  config,
		mu:      &sync.RWMutex{},
	}, nil
}

func (s *secretsVaultSource) Info() string {
	return fmt.Sprintf("vault source; addr: %s", s.address)
}

func (s *secretsVaultSource) Init() error {
	if !s.config.K8SAuth {
		return nil
	}
	if s.authVaultValid() {
		return nil
	}
	return s.authVault()
}

func (s *secretsVaultSource) authVaultValidLocked() bool {
	return s.vaultExpire-time.Now().Round(0).Unix() > 5
}

func (s *secretsVaultSource) authVaultValid() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.authVaultValidLocked()
}

func (s *secretsVaultSource) authVault() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.authVaultValidLocked() {
		return nil
	}

	vault := s.vault.Logical()
	authsecret, err := vault.Write(s.config.K8SLoginPath, map[string]interface{}{
		"jwt":  s.config.K8SJWT,
		"role": s.config.K8SRole,
	})
	if err != nil {
		return kerrors.WithKind(err, ErrVault{}, "Failed to auth with vault k8s")
	}
	s.vaultExpire = time.Now().Round(0).Unix() + int64(authsecret.Auth.LeaseDuration)
	s.vault.SetToken(authsecret.Auth.ClientToken)
	return nil
}

func (s *secretsVaultSource) GetSecret(ctx context.Context, kvpath string) (map[string]interface{}, int64, error) {
	if err := s.Init(); err != nil {
		return nil, 0, err
	}

	vault := s.vault.Logical()
	secret, err := vault.ReadWithContext(ctx, kvpath)
	if err != nil {
		return nil, 0, kerrors.WithKind(err, ErrVault{}, "Failed to read vault secret")
	}
	data := secret.Data
	if v, ok := data["data"].(map[string]interface{}); ok {
		data = v
	}
	var expire int64
	if secret.LeaseDuration > 0 {
		expire = time.Now().Round(0).Unix() + int64(secret.LeaseDuration)
		k := s.vaultExpire
		if expire > k {
			expire = k
		}
	}
	return data, expire, nil
}

func (c *Config) initsecrets() error {
	if vsource := c.config.GetString("vault.filesource"); vsource != "" {
		client, err := newSecretsFileSource(vsource)
		if err != nil {
			return err
		}
		c.vault = client
		return nil
	}
	config := secretsVaultSourceConfig{}
	if vaddr := c.config.GetString("vault.addr"); vaddr != "" {
		config.Addr = vaddr
	}
	if c.config.GetBool("vault.k8s.auth") {
		config.K8SAuth = true

		config.K8SRole = c.config.GetString("vault.k8s.role")
		config.K8SLoginPath = c.config.GetString("vault.k8s.loginpath")
		jwtpath := c.config.GetString("vault.k8s.jwtpath")
		if config.K8SRole == "" {
			return kerrors.WithKind(nil, ErrInvalidConfig{}, "No vault role set")
		}
		if config.K8SLoginPath == "" {
			return kerrors.WithKind(nil, ErrInvalidConfig{}, "No vault k8s login path set")
		}
		if jwtpath == "" {
			return kerrors.WithKind(nil, ErrInvalidConfig{}, "No path for vault k8s service account jwt auth")
		}
		jwtbytes, err := os.ReadFile(jwtpath)
		if err != nil {
			return kerrors.WithKind(err, ErrInvalidConfig{}, "Failed to read vault k8s service account jwt")
		}
		config.K8SJWT = string(jwtbytes)
	}
	vault, err := NewSecretsVaultSource(config)
	if err != nil {
		return err
	}
	if err := vault.Init(); err != nil {
		return err
	}
	c.vault = vault
	return nil
}

func (c *Config) getSecret(ctx context.Context, key string, seconds int64, target interface{}) error {
	if s, ok := c.vaultCache[key]; ok && s.isValid() {
		if err := mapstructure.Decode(s.value, target); err != nil {
			return kerrors.WithKind(err, ErrInvalidConfig{}, "Failed decoding secret")
		}
		return nil
	}

	kvpath := c.config.GetString(key)
	if kvpath == "" {
		return kerrors.WithKind(nil, ErrInvalidConfig{}, "Empty secret key "+key)
	}

	data, expire, err := c.vault.GetSecret(ctx, kvpath)
	if err != nil {
		return err
	}
	if expire == 0 && seconds != 0 {
		expire = time.Now().Round(0).Unix() + seconds
	}
	c.vaultCache[key] = vaultSecret{
		key:    key,
		value:  data,
		expire: expire,
	}

	if err := mapstructure.Decode(data, target); err != nil {
		return kerrors.WithKind(err, ErrInvalidConfig{}, "Failed decoding secret")
	}
	return nil
}

func (c *Config) invalidateSecret(key string) {
	delete(c.vaultCache, key)
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
		GetSecret(ctx context.Context, key string, seconds int64, target interface{}) error
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
	return s.expire == 0 || time.Now().Round(0).Unix()+5 < s.expire
}

func (r *configReader) GetSecret(ctx context.Context, key string, seconds int64, target interface{}) error {
	return r.c.getSecret(ctx, r.name+"."+key, seconds, target)
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

// DecodeSysEventTimestampProps unmarshals json encoded sys event timestamp props
func DecodeSysEventTimestampProps(msgdata []byte) (*SysEventTimestampProps, error) {
	m := &SysEventTimestampProps{}
	if err := json.Unmarshal(msgdata, m); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to decode sys event timestamp props")
	}
	return m, nil
}
