package types

import (
	"errors"
	"fmt"
	"io/fs"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

const (
	PKCEMethodPlain = "plain"
	PKCEMethodS256  = "S256"

	defaultNodeStoreBatchSize = 100
)

var (
	ErrNoPrefixConfigured        = errors.New("no IPv4 or IPv6 prefix configured, minimum one prefix is required")
	ErrInvalidAllocationStrategy = errors.New("invalid prefix allocation strategy")
	errInvalidPKCEMethod         = errors.New("pkce.method must be either 'plain' or 'S256'")
)

type IPAllocationStrategy string

const (
	IPAllocationStrategySequential IPAllocationStrategy = "sequential"
	IPAllocationStrategyRandom     IPAllocationStrategy = "random"
)

type PolicyMode string

const (
	PolicyModeDB   PolicyMode = "database"
	PolicyModeFile PolicyMode = "file"
)

type EphemeralConfig struct {
	InactivityTimeout time.Duration
}

type HARouteConfig struct {
	ProbeInterval time.Duration
	ProbeTimeout  time.Duration
}

type RouteConfig struct {
	HA HARouteConfig
}

type NodeConfig struct {
	Expiry    time.Duration
	Ephemeral EphemeralConfig
	Routes    RouteConfig
}

type Config struct {
	ServerURL           string
	Addr                string
	MetricsAddr         string
	GRPCAddr            string
	GRPCAllowInsecure   bool
	Node                NodeConfig
	PrefixV4            *netip.Prefix
	PrefixV6            *netip.Prefix
	IPAllocation        IPAllocationStrategy
	NoisePrivateKeyPath string
	BaseDomain          string
	Log                 LogConfig
	DisableUpdateCheck  bool

	Database DatabaseConfig
	DERP     DERPConfig
	TLS      TLSConfig

	ACMEURL   string
	ACMEEmail string

	DNSConfig        DNSConfig
	UnixSocket       string
	UnixSocketPerms  fs.FileMode
	OIDC             OIDCConfig
	RandomizeClientPort bool
	Taildrop         TaildropConfig
	CLI              CLIConfig
	Policy           PolicyConfig
	Tuning           TuningConfig
}

type DNSConfig struct {
	MagicDNS         bool
	BaseDomain       string
	OverrideLocalDNS bool
	Nameservers      NameserversConfig
	SearchDomains    []string
	ExtraRecords     []DNSRecord
	ExtraRecordsPath string
}

type NameserversConfig struct {
	Global []string
	Split  map[string][]string
}

type DNSRecord struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

type DatabaseConfig struct {
	Type     string
	Debug    bool
	Gorm     GormConfig
	Sqlite   SqliteConfig
	Postgres PostgresConfig
}

type GormConfig struct {
	Debug                 bool
	SlowThreshold         time.Duration
	SkipErrRecordNotFound bool
	ParameterizedQueries  bool
	PrepareStmt           bool
}

type SqliteConfig struct {
	Path              string
	WriteAheadLog     bool
	WALAutoCheckPoint int
}

type PostgresConfig struct {
	Host                string
	Port                int
	Name                string
	User                string
	Pass                string `json:"-"`
	Ssl                 string
	MaxOpenConnections  int
	MaxIdleConnections  int
	ConnMaxIdleTimeSecs int
}

type DERPConfig struct {
	ServerEnabled                      bool
	AutomaticallyAddEmbeddedDerpRegion bool
	ServerRegionID                     int
	ServerRegionCode                   string
	ServerRegionName                   string
	ServerPrivateKeyPath               string
	ServerVerifyClients                bool
	STUNAddr                           string
	URLs                               []url.URL
	Paths                              []string
	AutoUpdate                         bool
	UpdateFrequency                    time.Duration
	IPv4                               string
	IPv6                               string
}

type TLSConfig struct {
	CertPath   string
	KeyPath    string
	LetsEncrypt LetsEncryptConfig
}

type LetsEncryptConfig struct {
	Listen        string
	Hostname      string
	CacheDir      string
	ChallengeType string
}

type PKCEConfig struct {
	Enabled bool
	Method  string
}

type OIDCConfig struct {
	OnlyStartIfOIDCIsAvailable bool
	Issuer                     string
	ClientID                   string
	ClientSecret               string `json:"-"`
	Scope                      []string
	ExtraParams                map[string]string
	AllowedDomains             []string
	AllowedUsers               []string
	AllowedGroups              []string
	EmailVerifiedRequired      bool
	UseExpiryFromToken         bool
	PKCE                       PKCEConfig
}

type OIDCClaims struct {
	Sub               string          `json:"sub"`
	Iss               string          `json:"iss"`
	Name              string          `json:"name,omitempty"`
	Groups            []string        `json:"groups,omitempty"`
	Email             string          `json:"email,omitempty"`
	EmailVerified     FlexibleBoolean `json:"email_verified,omitempty"`
	ProfilePictureURL string          `json:"picture,omitempty"`
	Username          string          `json:"preferred_username,omitempty"`
}

func (c *OIDCClaims) Identifier() string {
	if c.Iss == "" && c.Sub == "" {
		return ""
	}
	if c.Iss == "" {
		return CleanIdentifier(c.Sub)
	}
	if c.Sub == "" {
		return CleanIdentifier(c.Iss)
	}
	issuer := strings.TrimSuffix(c.Iss, "/")
	subject := strings.TrimPrefix(c.Sub, "/")
	result := issuer + "/" + subject
	return CleanIdentifier(result)
}

type OIDCUserInfo struct {
	Sub               string          `json:"sub"`
	Name              string          `json:"name"`
	GivenName         string          `json:"given_name"`
	FamilyName        string          `json:"family_name"`
	PreferredUsername string          `json:"preferred_username"`
	Email             string          `json:"email"`
	EmailVerified     FlexibleBoolean `json:"email_verified,omitempty"`
	Groups            []string        `json:"groups"`
	Picture           string          `json:"picture"`
}

type FlexibleBoolean bool

func (fb *FlexibleBoolean) UnmarshalJSON(data []byte) error {
	s := strings.ToLower(strings.Trim(string(data), `"`))
	switch s {
	case "true", "1", "yes", "on":
		*fb = true
	case "false", "0", "no", "off", "":
		*fb = false
	default:
		return fmt.Errorf("invalid boolean value: %s", s)
	}
	return nil
}

type TaildropConfig struct {
	Enabled bool
}

type CLIConfig struct {
	Address  string
	APIKey   string `json:"-"`
	Timeout  time.Duration
	Insecure bool
}

type PolicyConfig struct {
	Path string
	Mode PolicyMode
}

func (p *PolicyConfig) IsEmpty() bool {
	return p.Mode == PolicyModeFile && p.Path == ""
}

type LogConfig struct {
	Format string
	Level  string
}

type TuningConfig struct {
	NotifierSendTimeout           time.Duration
	BatchChangeDelay             time.Duration
	NodeMapSessionBufferedChanSize int
	BatcherWorkers               int
	RegisterCacheExpiration      time.Duration
	RegisterCacheMaxEntries      int
	NodeStoreBatchSize           int
	NodeStoreBatchTimeout        time.Duration
}

func validatePKCEMethod(method string) error {
	if method != PKCEMethodPlain && method != PKCEMethodS256 {
		return errInvalidPKCEMethod
	}
	return nil
}

func (c *Config) Domain() string {
	u, err := url.Parse(c.ServerURL)
	if err != nil {
		return c.BaseDomain
	}
	return u.Hostname()
}

func LoadConfig(path string, isFile bool) error {
	if isFile {
		viper.SetConfigFile(path)
	} else {
		viper.SetConfigName("config")
		if path == "" {
			viper.AddConfigPath("/etc/headscale/")
			viper.AddConfigPath("$HOME/.headscale")
			viper.AddConfigPath(".")
		} else {
			viper.AddConfigPath(path)
		}
	}

	envPrefix := "headscale"
	viper.SetEnvPrefix(envPrefix)
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	viper.SetDefault("policy.mode", "file")
	viper.SetDefault("tls_letsencrypt_cache_dir", "/var/www/.cache")
	viper.SetDefault("tls_letsencrypt_challenge_type", "HTTP-01")
	viper.SetDefault("log.level", "info")
	viper.SetDefault("log.format", "text")
	viper.SetDefault("dns.magic_dns", true)
	viper.SetDefault("dns.base_domain", "")
	viper.SetDefault("dns.override_local_dns", true)
	viper.SetDefault("dns.nameservers.global", []string{})
	viper.SetDefault("dns.nameservers.split", map[string]string{})
	viper.SetDefault("dns.search_domains", []string{})
	viper.SetDefault("derp.server.enabled", false)
	viper.SetDefault("derp.server.verify_clients", true)
	viper.SetDefault("derp.server.stun.enabled", true)
	viper.SetDefault("derp.server.automatically_add_embedded_derp_region", true)
	viper.SetDefault("derp.update_frequency", "3h")
	viper.SetDefault("unix_socket", "/var/run/headscale/headscale.sock")
	viper.SetDefault("unix_socket_permission", "0o770")
	viper.SetDefault("grpc_listen_addr", ":50443")
	viper.SetDefault("grpc_allow_insecure", false)
	viper.SetDefault("cli.timeout", "5s")
	viper.SetDefault("cli.insecure", false)
	viper.SetDefault("database.postgres.ssl", "false")
	viper.SetDefault("database.postgres.max_open_conns", 10)
	viper.SetDefault("database.postgres.max_idle_conns", 10)
	viper.SetDefault("database.postgres.conn_max_idle_time_secs", 3600)
	viper.SetDefault("database.sqlite.write_ahead_log", true)
	viper.SetDefault("database.sqlite.wal_autocheckpoint", 1000)
	viper.SetDefault("oidc.scope", []string{"openid", "profile", "email"})
	viper.SetDefault("oidc.only_start_if_oidc_is_available", true)
	viper.SetDefault("oidc.use_expiry_from_token", false)
	viper.SetDefault("oidc.pkce.enabled", false)
	viper.SetDefault("oidc.pkce.method", "S256")
	viper.SetDefault("oidc.email_verified_required", true)
	viper.SetDefault("randomize_client_port", false)
	viper.SetDefault("taildrop.enabled", true)
	viper.SetDefault("node.expiry", "0")
	viper.SetDefault("node.ephemeral.inactivity_timeout", "120s")
	viper.SetDefault("node.routes.ha.probe_interval", "10s")
	viper.SetDefault("node.routes.ha.probe_timeout", "5s")
	viper.SetDefault("tuning.notifier_send_timeout", "800ms")
	viper.SetDefault("tuning.batch_change_delay", "800ms")
	viper.SetDefault("tuning.node_mapsession_buffered_chan_size", 30)
	viper.SetDefault("tuning.node_store_batch_size", defaultNodeStoreBatchSize)
	viper.SetDefault("tuning.node_store_batch_timeout", "500ms")
	viper.SetDefault("prefixes.allocation", string(IPAllocationStrategySequential))

	err := viper.ReadInConfig()
	if err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			return nil
		}
		return fmt.Errorf("fatal error reading config file: %w", err)
	}
	return nil
}

func resolveEphemeralInactivityTimeout() time.Duration {
	if viper.IsSet("node.ephemeral.inactivity_timeout") &&
		viper.GetString("node.ephemeral.inactivity_timeout") != "" {
		return viper.GetDuration("node.ephemeral.inactivity_timeout")
	}
	if viper.IsSet("ephemeral_node_inactivity_timeout") {
		return viper.GetDuration("ephemeral_node_inactivity_timeout")
	}
	return viper.GetDuration("node.ephemeral.inactivity_timeout")
}

func resolveNodeExpiry() time.Duration {
	value := viper.GetString("node.expiry")
	if value == "" || value == "0" {
		return 0
	}
	return viper.GetDuration("node.expiry")
}

func LoadServerConfig() (*Config, error) {
	logLevelStr := viper.GetString("log.level")
	logLevel := parseLogLevel(logLevelStr)

	logFormat := viper.GetString("log.format")
	if logFormat != "json" && logFormat != "text" {
		logFormat = "text"
	}

	var prefix4, prefix6 *netip.Prefix
	prefixV4Str := viper.GetString("prefixes.v4")
	if prefixV4Str != "" {
		p, err := netip.ParsePrefix(prefixV4Str)
		if err != nil {
			return nil, fmt.Errorf("parsing IPv4 prefix: %w", err)
		}
		prefix4 = &p
	}

	prefixV6Str := viper.GetString("prefixes.v6")
	if prefixV6Str != "" {
		p, err := netip.ParsePrefix(prefixV6Str)
		if err != nil {
			return nil, fmt.Errorf("parsing IPv6 prefix: %w", err)
		}
		prefix6 = &p
	}

	if prefix4 == nil && prefix6 == nil {
		return nil, ErrNoPrefixConfigured
	}

	allocStr := viper.GetString("prefixes.allocation")
	var alloc IPAllocationStrategy
	switch allocStr {
	case string(IPAllocationStrategySequential):
		alloc = IPAllocationStrategySequential
	case string(IPAllocationStrategyRandom):
		alloc = IPAllocationStrategyRandom
	default:
		return nil, fmt.Errorf("%w: %q", ErrInvalidAllocationStrategy, allocStr)
	}

	oidcClientSecret := viper.GetString("oidc.client_secret")
	oidcClientSecretPath := viper.GetString("oidc.client_secret_path")
	if oidcClientSecretPath != "" {
		if oidcClientSecret != "" {
			return nil, errors.New("oidc_client_secret and oidc_client_secret_path are mutually exclusive")
		}
		secretBytes, err := os.ReadFile(os.ExpandEnv(oidcClientSecretPath))
		if err != nil {
			return nil, fmt.Errorf("reading OIDC client secret file: %w", err)
		}
		oidcClientSecret = strings.TrimSpace(string(secretBytes))
	}

	derpURLs := viper.GetStringSlice("derp.urls")
	urls := make([]url.URL, 0, len(derpURLs))
	for _, u := range derpURLs {
		if parsed, err := url.Parse(u); err == nil {
			urls = append(urls, *parsed)
		}
	}

	unixSocketPerm := fs.FileMode(0770)
	if permStr := viper.GetString("unix_socket_permission"); permStr != "" {
		if strings.HasPrefix(permStr, "0o") || strings.HasPrefix(permStr, "0") {
			if _, err := fmt.Sscanf(permStr, "%o", &unixSocketPerm); err != nil {
			}
		}
	}

	cfg := &Config{
		ServerURL:         viper.GetString("server_url"),
		Addr:              viper.GetString("listen_addr"),
		MetricsAddr:       viper.GetString("metrics_listen_addr"),
		GRPCAddr:          viper.GetString("grpc_listen_addr"),
		GRPCAllowInsecure: viper.GetBool("grpc_allow_insecure"),
		DisableUpdateCheck: viper.GetBool("disable_check_updates"),
		PrefixV4:          prefix4,
		PrefixV6:          prefix6,
		IPAllocation:      alloc,
		NoisePrivateKeyPath: viper.GetString("noise.private_key_path"),
		BaseDomain:        viper.GetString("dns.base_domain"),
		UnixSocket:        viper.GetString("unix_socket"),
		UnixSocketPerms:   unixSocketPerm,
		RandomizeClientPort: viper.GetBool("randomize_client_port"),
		ACMEEmail:         viper.GetString("acme_email"),
		ACMEURL:           viper.GetString("acme_url"),

		Node: NodeConfig{
			Expiry: resolveNodeExpiry(),
			Ephemeral: EphemeralConfig{
				InactivityTimeout: resolveEphemeralInactivityTimeout(),
			},
			Routes: RouteConfig{
				HA: HARouteConfig{
					ProbeInterval: viper.GetDuration("node.routes.ha.probe_interval"),
					ProbeTimeout:  viper.GetDuration("node.routes.ha.probe_timeout"),
				},
			},
		},

		Database: DatabaseConfig{
			Type:  viper.GetString("database.type"),
			Debug: viper.GetBool("database.debug"),
			Gorm: GormConfig{
				Debug:                 viper.GetBool("database.debug"),
				SlowThreshold:         viper.GetDuration("database.gorm.slow_threshold"),
				SkipErrRecordNotFound: viper.GetBool("database.gorm.skip_err_record_not_found"),
				ParameterizedQueries:  viper.GetBool("database.gorm.parameterized_queries"),
				PrepareStmt:           viper.GetBool("database.gorm.prepare_stmt"),
			},
			Sqlite: SqliteConfig{
				Path:              viper.GetString("database.sqlite.path"),
				WriteAheadLog:     viper.GetBool("database.sqlite.write_ahead_log"),
				WALAutoCheckPoint: viper.GetInt("database.sqlite.wal_autocheckpoint"),
			},
			Postgres: PostgresConfig{
				Host:               viper.GetString("database.postgres.host"),
				Port:               viper.GetInt("database.postgres.port"),
				Name:               viper.GetString("database.postgres.name"),
				User:               viper.GetString("database.postgres.user"),
				Pass:               viper.GetString("database.postgres.pass"),
				Ssl:                viper.GetString("database.postgres.ssl"),
				MaxOpenConnections: viper.GetInt("database.postgres.max_open_conns"),
				MaxIdleConnections: viper.GetInt("database.postgres.max_idle_conns"),
				ConnMaxIdleTimeSecs: viper.GetInt("database.postgres.conn_max_idle_time_secs"),
			},
		},

		DERP: DERPConfig{
			ServerEnabled:                      viper.GetBool("derp.server.enabled"),
			AutomaticallyAddEmbeddedDerpRegion: viper.GetBool("derp.server.automatically_add_embedded_derp_region"),
			ServerRegionID:                    viper.GetInt("derp.server.region_id"),
			ServerRegionCode:                  viper.GetString("derp.server.region_code"),
			ServerRegionName:                 viper.GetString("derp.server.region_name"),
			ServerPrivateKeyPath:             viper.GetString("derp.server.private_key_path"),
			ServerVerifyClients:              viper.GetBool("derp.server.verify_clients"),
			STUNAddr:                         viper.GetString("derp.server.stun_listen_addr"),
			URLs:                             urls,
			Paths:                            viper.GetStringSlice("derp.paths"),
			AutoUpdate:                       viper.GetBool("derp.auto_update_enabled"),
			UpdateFrequency:                  viper.GetDuration("derp.update_frequency"),
			IPv4:                             viper.GetString("derp.server.ipv4"),
			IPv6:                             viper.GetString("derp.server.ipv6"),
		},

		TLS: TLSConfig{
			CertPath: viper.GetString("tls_cert_path"),
			KeyPath:  viper.GetString("tls_key_path"),
			LetsEncrypt: LetsEncryptConfig{
				Listen:        viper.GetString("tls_letsencrypt_listen"),
				Hostname:      viper.GetString("tls_letsencrypt_hostname"),
				CacheDir:     viper.GetString("tls_letsencrypt_cache_dir"),
				ChallengeType: viper.GetString("tls_letsencrypt_challenge_type"),
			},
		},

		DNSConfig: DNSConfig{
			MagicDNS:         viper.GetBool("dns.magic_dns"),
			BaseDomain:       viper.GetString("dns.base_domain"),
			OverrideLocalDNS: viper.GetBool("dns.override_local_dns"),
			Nameservers: NameserversConfig{
				Global: viper.GetStringSlice("dns.nameservers.global"),
				Split:  viper.GetStringMapStringSlice("dns.nameservers.split"),
			},
			SearchDomains:    viper.GetStringSlice("dns.search_domains"),
			ExtraRecordsPath: viper.GetString("dns.extra_records_path"),
		},

		OIDC: OIDCConfig{
			OnlyStartIfOIDCIsAvailable: viper.GetBool("oidc.only_start_if_oidc_is_available"),
			Issuer:                viper.GetString("oidc.issuer"),
			ClientID:              viper.GetString("oidc.client_id"),
			ClientSecret:          oidcClientSecret,
			Scope:                 viper.GetStringSlice("oidc.scope"),
			ExtraParams:           viper.GetStringMapString("oidc.extra_params"),
			AllowedDomains:        viper.GetStringSlice("oidc.allowed_domains"),
			AllowedUsers:          viper.GetStringSlice("oidc.allowed_users"),
			AllowedGroups:         viper.GetStringSlice("oidc.allowed_groups"),
			EmailVerifiedRequired: viper.GetBool("oidc.email_verified_required"),
			UseExpiryFromToken:    viper.GetBool("oidc.use_expiry_from_token"),
			PKCE: PKCEConfig{
				Enabled: viper.GetBool("oidc.pkce.enabled"),
				Method:  viper.GetString("oidc.pkce.method"),
			},
		},

		Taildrop: TaildropConfig{
			Enabled: viper.GetBool("taildrop.enabled"),
		},

		CLI: CLIConfig{
			Address:  viper.GetString("cli.address"),
			APIKey:   viper.GetString("cli.api_key"),
			Timeout:  viper.GetDuration("cli.timeout"),
			Insecure: viper.GetBool("cli.insecure"),
		},

		Policy: PolicyConfig{
			Path: viper.GetString("policy.path"),
			Mode: PolicyMode(viper.GetString("policy.mode")),
		},

		Log: LogConfig{
			Format: logFormat,
			Level:  logLevel,
		},

		Tuning: TuningConfig{
			NotifierSendTimeout:           viper.GetDuration("tuning.notifier_send_timeout"),
			BatchChangeDelay:             viper.GetDuration("tuning.batch_change_delay"),
			NodeMapSessionBufferedChanSize: viper.GetInt("tuning.node_mapsession_buffered_chan_size"),
			BatcherWorkers:               max(1, viper.GetInt("tuning.batcher_workers")),
			RegisterCacheExpiration:      viper.GetDuration("tuning.register_cache_expiration"),
			RegisterCacheMaxEntries:      viper.GetInt("tuning.register_cache_max_entries"),
			NodeStoreBatchSize:           viper.GetInt("tuning.node_store_batch_size"),
			NodeStoreBatchTimeout:        viper.GetDuration("tuning.node_store_batch_timeout"),
		},
	}

	if viper.GetBool("oidc.enabled") {
		if err := validatePKCEMethod(cfg.OIDC.PKCE.Method); err != nil {
			return nil, err
		}
	}

	return cfg, nil
}

func LoadCLIConfig() (*Config, error) {
	logLevelStr := viper.GetString("log.level")
	logLevel := parseLogLevel(logLevelStr)

	return &Config{
		DisableUpdateCheck: viper.GetBool("disable_check_updates"),
		UnixSocket:        viper.GetString("unix_socket"),
		CLI: CLIConfig{
			Address:  viper.GetString("cli.address"),
			APIKey:   viper.GetString("cli.api_key"),
			Timeout:  viper.GetDuration("cli.timeout"),
			Insecure: viper.GetBool("cli.insecure"),
		},
		Log: LogConfig{
			Format: viper.GetString("log.format"),
			Level:  logLevel,
		},
	}, nil
}

func (u *User) FromClaim(claims *OIDCClaims, emailVerifiedRequired bool) {
	if claims.Username != "" {
		u.Name = claims.Username
	}
	if claims.EmailVerified || !FlexibleBoolean(emailVerifiedRequired) {
		u.Email = claims.Email
	}
	identifier := claims.Identifier()
	if claims.Iss == "" && !strings.HasPrefix(identifier, "/") {
		identifier = "/" + identifier
	}
	u.DisplayName = claims.Name
	u.ProfileURL = claims.ProfilePictureURL
	u.Provider = "oidc"
	_ = identifier
}

func max[T int](a, b T) T {
	if a > b {
		return a
	}
	return b
}

func parseLogLevel(level string) string {
	switch strings.ToLower(level) {
	case "trace", "debug", "info", "warn", "error":
		return strings.ToLower(level)
	default:
		return "info"
	}
}
