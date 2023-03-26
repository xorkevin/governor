package governor

import (
	"context"
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
		Appname       string
		Version       Version
		Description   string
		DefaultFile   string
		EnvPrefix     string
		ClientDefault string
		ClientPrefix  string
		ConfigReader  io.Reader
		VaultReader   io.Reader
		LogWriter     io.Writer
		HTTPTransport http.RoundTripper
		TermConfig    *TermConfig
	}
)

func (v Version) String() string {
	return v.Num + "-" + v.Hash
}

type (
	// secretsClient is a client that reads secrets
	secretsClient interface {
		Info() string
		Init(ctx context.Context) error
		GetSecret(ctx context.Context, kvpath string) (map[string]interface{}, time.Time, error)
	}

	settings struct {
		v            *viper.Viper
		configReader io.Reader
		vault        secretsClient
		vaultCache   *sync.Map
		vaultReader  io.Reader
		showBanner   bool
		config       Config
		logger       configLogger
		httpServer   configHTTPServer
		middleware   configMiddleware
	}

	configLogger struct {
		level  string
		output string
		writer io.Writer
	}

	configHTTPServer struct {
		maxReqSize    string
		maxHeaderSize string
		maxConnRead   string
		maxConnHeader string
		maxConnWrite  string
		maxConnIdle   string
	}

	configMiddleware struct {
		alloworigins       []string
		allowpaths         []*corsPathRule
		routerewrite       []*rewriteRule
		trustedproxies     []string
		compressibleTypes  []string
		preferredEncodings []string
	}

	// Config is the server configuration including those from a config file and
	// environment variables
	Config struct {
		Appname  string
		Version  Version
		Hostname string
		Instance string
		Addr     string
		BasePath string
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

func newSettings(opts Opts) *settings {
	v := viper.New()
	v.SetDefault("logger.level", "INFO")
	v.SetDefault("logger.output", "STDERR")
	v.SetDefault("banner", true)
	v.SetDefault("http.addr", ":8080")
	v.SetDefault("http.basepath", "/")
	v.SetDefault("http.maxreqsize", "2M")
	v.SetDefault("http.maxheadersize", "1M")
	v.SetDefault("http.maxconnread", "5s")
	v.SetDefault("http.maxconnheader", "2s")
	v.SetDefault("http.maxconnwrite", "5s")
	v.SetDefault("http.maxconnidle", "5s")
	v.SetDefault("cors.alloworigins", []string{})
	v.SetDefault("cors.allowpaths", []string{})
	v.SetDefault("routerewrite", []*rewriteRule{})
	v.SetDefault("trustedproxies", []string{})
	v.SetDefault("compressor.compressibletypes", defaultCompressibleMediaTypes)
	v.SetDefault("compressor.preferredencodings", defaultPreferredEncodings)
	v.SetDefault("vault.filesource", "")
	v.SetDefault("vault.addr", "")
	v.SetDefault("vault.token", "")
	v.SetDefault("vault.k8s.auth", false)
	v.SetDefault("vault.k8s.role", "")
	v.SetDefault("vault.k8s.loginpath", "/auth/kubernetes/login")
	v.SetDefault("vault.k8s.jwtpath", "/var/run/secrets/kubernetes.io/serviceaccount/token")

	v.SetConfigName(opts.DefaultFile)
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("config")
	if cfgdir, err := os.UserConfigDir(); err == nil {
		v.AddConfigPath(filepath.Join(cfgdir, opts.Appname))
	}

	v.SetEnvPrefix(opts.EnvPrefix)
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "__"))

	return &settings{
		v:            v,
		configReader: opts.ConfigReader,
		vaultCache:   &sync.Map{},
		vaultReader:  opts.VaultReader,
		config: Config{
			Appname: opts.Appname,
			Version: opts.Version,
		},
		logger: configLogger{
			writer: opts.LogWriter,
		},
	}
}

// Config errors
var (
	// ErrInvalidConfig is returned when the config is invalid
	ErrInvalidConfig errInvalidConfig
	// ErrVault is returned when failing to contact vault
	ErrVault errVault
)

type (
	errInvalidConfig struct{}
	errVault         struct{}
)

func (e errInvalidConfig) Error() string {
	return "Invalid config"
}

func (e errVault) Error() string {
	return "Failed vault request"
}

const (
	instanceIDRandSize = 8
)

func (s *settings) init(ctx context.Context, flags Flags) error {
	if flags.ConfigFile != "" {
		s.v.SetConfigFile(flags.ConfigFile)
	}

	var err error
	s.config.Hostname, err = os.Hostname()
	if err != nil {
		return kerrors.WithMsg(err, "Failed to get hostname")
	}
	u, err := uid.NewSnowflake(instanceIDRandSize)
	if err != nil {
		return err
	}
	s.config.Instance = u.Base32()

	if s.configReader != nil {
		if err := s.v.ReadConfig(s.configReader); err != nil {
			return kerrors.WithKind(err, ErrInvalidConfig, "Failed to read in config")
		}
	} else {
		if err := s.v.ReadInConfig(); err != nil {
			return kerrors.WithKind(err, ErrInvalidConfig, "Failed to read in config")
		}
	}

	s.showBanner = s.v.GetBool("banner")
	s.logger.level = s.v.GetString("logger.level")
	s.logger.output = s.v.GetString("logger.output")
	s.config.Addr = s.v.GetString("http.addr")
	s.config.BasePath = s.v.GetString("http.basepath")
	s.httpServer.maxReqSize = s.v.GetString("http.maxreqsize")
	s.httpServer.maxHeaderSize = s.v.GetString("http.maxheadersize")
	s.httpServer.maxConnRead = s.v.GetString("http.maxconnread")
	s.httpServer.maxConnHeader = s.v.GetString("http.maxconnheader")
	s.httpServer.maxConnWrite = s.v.GetString("http.maxconnwrite")
	s.httpServer.maxConnIdle = s.v.GetString("http.maxconnidle")
	s.middleware.alloworigins = s.v.GetStringSlice("cors.alloworigins")
	allowPathPatterns := s.v.GetStringSlice("cors.allowpaths")
	s.middleware.allowpaths = make([]*corsPathRule, 0, len(allowPathPatterns))
	for _, i := range allowPathPatterns {
		s.middleware.allowpaths = append(s.middleware.allowpaths, &corsPathRule{
			pattern: i,
		})
	}
	routerewrite := []*rewriteRule{}
	if err := s.v.UnmarshalKey("routerewrite", &routerewrite); err != nil {
		return err
	}
	s.middleware.routerewrite = routerewrite
	s.middleware.trustedproxies = s.v.GetStringSlice("trustedproxies")
	s.middleware.compressibleTypes = s.v.GetStringSlice("compressor.compressibletypes")
	s.middleware.preferredEncodings = s.v.GetStringSlice("compressor.preferredencodings")
	if err := s.initsecrets(ctx); err != nil {
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
func newSecretsFileSource(s string, r io.Reader) (secretsClient, error) {
	var b []byte
	if r != nil {
		var err error
		b, err = io.ReadAll(r)
		if err != nil {
			return nil, kerrors.WithKind(err, ErrInvalidConfig, "Failed to read secrets file source")
		}
		s = "io.Reader"
	} else {
		var err error
		b, err = os.ReadFile(s)
		if err != nil {
			return nil, kerrors.WithKind(err, ErrInvalidConfig, "Failed to read secrets file source")
		}
	}
	data := secretsFileData{}
	if err := yaml.Unmarshal(b, &data); err != nil {
		return nil, kerrors.WithKind(err, ErrInvalidConfig, "Invalid secrets file source file")
	}
	return &secretsFileSource{
		path: s,
		data: data,
	}, nil
}

func (s *secretsFileSource) Info() string {
	return fmt.Sprintf("file source; path: %s", s.path)
}

func (s *secretsFileSource) Init(ctx context.Context) error {
	return nil
}

func (s *secretsFileSource) GetSecret(ctx context.Context, kvpath string) (map[string]interface{}, time.Time, error) {
	data, ok := s.data.Data[kvpath]
	if !ok {
		return nil, time.Time{}, kerrors.WithKind(nil, ErrVault, "Failed to read vault secret")
	}
	return data, time.Time{}, nil
}

type (
	// secretsVaultSourceConfig is a vault secrets client config
	secretsVaultSourceConfig struct {
		Addr         string
		AuthToken    string
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
		vaultExpire time.Time
		mu          *sync.RWMutex
	}
)

func newSecretsVaultSource(config secretsVaultSourceConfig) (secretsClient, error) {
	vconfig := vaultapi.DefaultConfig()
	if err := vconfig.Error; err != nil {
		return nil, kerrors.WithKind(err, ErrInvalidConfig, "Failed to create vault default config")
	}
	vconfig.Address = config.Addr
	vault, err := vaultapi.NewClient(vconfig)
	if err != nil {
		return nil, kerrors.WithKind(err, ErrInvalidConfig, "Failed to create vault client")
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

func (s *secretsVaultSource) Init(ctx context.Context) error {
	if s.config.AuthToken != "" {
		s.vault.SetToken(s.config.AuthToken)
		return nil
	}
	if !s.config.K8SAuth {
		return nil
	}
	if s.authVaultValid() {
		return nil
	}
	return s.authVault(ctx)
}

func (s *secretsVaultSource) authVaultValidLocked() bool {
	return time.Now().Round(0).Add(5 * time.Second).Before(s.vaultExpire)
}

func (s *secretsVaultSource) authVaultValid() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.authVaultValidLocked()
}

func (s *secretsVaultSource) authVault(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.authVaultValidLocked() {
		return nil
	}

	authsecret, err := s.vault.Logical().WriteWithContext(ctx, s.config.K8SLoginPath, map[string]interface{}{
		"jwt":  s.config.K8SJWT,
		"role": s.config.K8SRole,
	})
	if err != nil {
		return kerrors.WithKind(err, ErrVault, "Failed to auth with vault k8s")
	}
	s.vaultExpire = time.Now().Round(0).Add(time.Duration(authsecret.Auth.LeaseDuration) * time.Second)
	s.vault.SetToken(authsecret.Auth.ClientToken)
	return nil
}

func (s *secretsVaultSource) GetSecret(ctx context.Context, kvpath string) (map[string]interface{}, time.Time, error) {
	if err := s.Init(ctx); err != nil {
		return nil, time.Time{}, err
	}

	secret, err := s.vault.Logical().ReadWithContext(ctx, kvpath)
	if err != nil {
		return nil, time.Time{}, kerrors.WithKind(err, ErrVault, "Failed to read vault secret")
	}
	data := secret.Data
	if v, ok := data["data"].(map[string]interface{}); ok {
		data = v
	}
	var expire time.Time
	if secret.LeaseDuration > 0 {
		expire = time.Now().Round(0).Add(time.Duration(secret.LeaseDuration) * time.Second)
		k := s.vaultExpire
		if expire.After(k) {
			expire = k
		}
	}
	return data, expire, nil
}

func (s *settings) initsecrets(ctx context.Context) error {
	if vsource := s.v.GetString("vault.filesource"); s.vaultReader != nil || vsource != "" {
		client, err := newSecretsFileSource(vsource, s.vaultReader)
		if err != nil {
			return err
		}
		s.vault = client
		return nil
	}
	config := secretsVaultSourceConfig{}
	if vaddr := s.v.GetString("vault.addr"); vaddr != "" {
		config.Addr = vaddr
	}
	if token := s.v.GetString("vault.token"); token != "" {
		config.AuthToken = token
	} else if s.v.GetBool("vault.k8s.auth") {
		config.K8SAuth = true

		config.K8SRole = s.v.GetString("vault.k8s.role")
		config.K8SLoginPath = s.v.GetString("vault.k8s.loginpath")
		jwtpath := s.v.GetString("vault.k8s.jwtpath")
		if config.K8SRole == "" {
			return kerrors.WithKind(nil, ErrInvalidConfig, "No vault role set")
		}
		if config.K8SLoginPath == "" {
			return kerrors.WithKind(nil, ErrInvalidConfig, "No vault k8s login path set")
		}
		if jwtpath == "" {
			return kerrors.WithKind(nil, ErrInvalidConfig, "No path for vault k8s service account jwt auth")
		}
		jwtbytes, err := os.ReadFile(jwtpath)
		if err != nil {
			return kerrors.WithKind(err, ErrInvalidConfig, "Failed to read vault k8s service account jwt")
		}
		config.K8SJWT = string(jwtbytes)
	}
	vault, err := newSecretsVaultSource(config)
	if err != nil {
		return err
	}
	if err := vault.Init(ctx); err != nil {
		return err
	}
	s.vault = vault
	return nil
}

func (s *settings) getSecret(ctx context.Context, key string, cacheDuration time.Duration, target interface{}) error {
	if v, ok := s.vaultCache.Load(key); ok {
		s := v.(vaultSecret)
		if s.isValid() {
			if err := mapstructure.Decode(s.value, target); err != nil {
				return kerrors.WithKind(err, ErrInvalidConfig, "Failed decoding secret")
			}
			return nil
		}
	}

	kvpath := s.v.GetString(key)
	if kvpath == "" {
		return kerrors.WithKind(nil, ErrInvalidConfig, "Empty secret key "+key)
	}

	data, expire, err := s.vault.GetSecret(ctx, kvpath)
	if err != nil {
		return err
	}
	if expire.IsZero() && cacheDuration != 0 {
		expire = time.Now().Round(0).Add(cacheDuration)
	}
	s.vaultCache.Store(key, vaultSecret{
		key:    key,
		value:  data,
		expire: expire,
	})

	if err := mapstructure.Decode(data, target); err != nil {
		return kerrors.WithKind(err, ErrInvalidConfig, "Failed decoding secret")
	}
	return nil
}

func (s *settings) invalidateSecret(key string) {
	s.vaultCache.Delete(key)
}

type (
	// ConfigRegistrar sets default values on the config parser
	ConfigRegistrar interface {
		Name() string
		SetDefault(key string, value interface{})
	}

	configRegistrar struct {
		prefix string
		v      *viper.Viper
	}
)

func (r *configRegistrar) Name() string {
	return r.prefix
}

func (r *configRegistrar) SetDefault(key string, value interface{}) {
	r.v.SetDefault(r.prefix+"."+key, value)
}

func (s *settings) registrar(prefix string) ConfigRegistrar {
	return &configRegistrar{
		prefix: prefix,
		v:      s.v,
	}
}

type (
	ConfigValueReader interface {
		Name() string
		URL() string
		GetBool(key string) bool
		GetInt(key string) int
		GetDuration(key string) (time.Duration, error)
		GetStr(key string) string
		GetStrSlice(key string) []string
		Unmarshal(key string, val interface{}) error
	}

	// ConfigReader gets values from the config parser
	ConfigReader interface {
		Config() Config
		ConfigValueReader
		SecretReader
	}

	// SecretReader gets values from a secret engine
	SecretReader interface {
		GetSecret(ctx context.Context, key string, cacheDuration time.Duration, target interface{}) error
		InvalidateSecret(key string)
	}

	vaultSecretVal map[string]interface{}

	vaultSecret struct {
		key    string
		value  vaultSecretVal
		expire time.Time
	}

	configReader struct {
		s *settings
		v *configValueReader
	}

	configValueReader struct {
		opt serviceOpt
		v   *viper.Viper
	}
)

func (r *configValueReader) Name() string {
	return r.opt.name
}

func (r *configValueReader) URL() string {
	return r.opt.url
}

func (r *configValueReader) GetBool(key string) bool {
	return r.v.GetBool(r.opt.name + "." + key)
}

func (r *configValueReader) GetInt(key string) int {
	return r.v.GetInt(r.opt.name + "." + key)
}

func (r *configValueReader) GetDuration(key string) (time.Duration, error) {
	return time.ParseDuration(r.GetStr(key))
}

func (r *configValueReader) GetStr(key string) string {
	return r.v.GetString(r.opt.name + "." + key)
}

func (r *configValueReader) GetStrSlice(key string) []string {
	return r.v.GetStringSlice(r.opt.name + "." + key)
}

func (r *configValueReader) Unmarshal(key string, val interface{}) error {
	return r.v.UnmarshalKey(r.opt.name+"."+key, val)
}

func (r *configReader) Config() Config {
	return r.s.config
}

func (r *configReader) Name() string {
	return r.v.Name()
}

func (r *configReader) URL() string {
	return r.v.URL()
}

func (r *configReader) GetBool(key string) bool {
	return r.v.GetBool(key)
}

func (r *configReader) GetInt(key string) int {
	return r.v.GetInt(key)
}

func (r *configReader) GetDuration(key string) (time.Duration, error) {
	return r.v.GetDuration(key)
}

func (r *configReader) GetStr(key string) string {
	return r.v.GetStr(key)
}

func (r *configReader) GetStrSlice(key string) []string {
	return r.v.GetStrSlice(key)
}

func (r *configReader) Unmarshal(key string, val interface{}) error {
	return r.v.Unmarshal(key, val)
}

func (s *vaultSecret) isValid() bool {
	return s.expire.IsZero() || time.Now().Round(0).Add(5*time.Second).Before(s.expire)
}

func (r *configReader) GetSecret(ctx context.Context, key string, cacheDuration time.Duration, target interface{}) error {
	return r.s.getSecret(ctx, r.v.Name()+"."+key, cacheDuration, target)
}

func (r *configReader) InvalidateSecret(key string) {
	r.s.invalidateSecret(r.v.Name() + "." + key)
}

func (s *settings) reader(opt serviceOpt) ConfigReader {
	return &configReader{
		s: s,
		v: &configValueReader{
			opt: opt,
			v:   s.v,
		},
	}
}
