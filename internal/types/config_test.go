package types

import (
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestOIDCClaimsIdentifier(t *testing.T) {
	tests := []struct {
		name     string
		claims   OIDCClaims
		expected string
	}{
		{
			name: "standard issuer/subject",
			claims: OIDCClaims{
				Iss: "https://example.com",
				Sub: "user123",
			},
			expected: "https://example.com/user123",
		},
		{
			name: "issuer with trailing slash",
			claims: OIDCClaims{
				Iss: "https://example.com/",
				Sub: "user123",
			},
			expected: "https://example.com/user123",
		},
		{
			name: "subject with leading slash",
			claims: OIDCClaims{
				Iss: "https://example.com",
				Sub: "/user123",
			},
			expected: "https://example.com/user123",
		},
		{
			name: "only subject",
			claims: OIDCClaims{
				Sub: "user123",
			},
			expected: "user123",
		},
		{
			name: "only issuer",
			claims: OIDCClaims{
				Iss: "https://example.com",
			},
			expected: "https://example.com",
		},
		{
			name:     "empty claims",
			claims:   OIDCClaims{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.claims.Identifier()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFlexibleBoolean(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
		hasError bool
	}{
		{"true", true, false},
		{"false", false, false},
		{"1", true, false},
		{"0", false, false},
		{"yes", true, false},
		{"no", false, false},
		{"on", true, false},
		{"off", false, false},
		{"", false, false},
		{"TRUE", true, false},
		{"FALSE", false, false},
		{"invalid", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var fb FlexibleBoolean
			err := fb.UnmarshalJSON([]byte(tt.input))

			if tt.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, bool(fb))
			}
		})
	}
}

func TestPolicyConfigIsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		config   PolicyConfig
		expected bool
	}{
		{
			name:     "empty file mode",
			config:   PolicyConfig{Mode: PolicyModeFile, Path: ""},
			expected: true,
		},
		{
			name:     "file mode with path",
			config:   PolicyConfig{Mode: PolicyModeFile, Path: "/etc/policy.hujson"},
			expected: false,
		},
		{
			name:     "database mode",
			config:   PolicyConfig{Mode: PolicyModeDB, Path: ""},
			expected: false,
		},
		{
			name:     "database mode with path ignored",
			config:   PolicyConfig{Mode: PolicyModeDB, Path: "/ignored"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.config.IsEmpty())
		})
	}
}

func TestConfigDomain(t *testing.T) {
	tests := []struct {
		name        string
		serverURL   string
		baseDomain  string
		expected    string
	}{
		{
			name:       "standard URL",
			serverURL:  "https://headscale.example.com",
			baseDomain: "example.net",
			expected:   "headscale.example.com",
		},
		{
			name:       "URL with port",
			serverURL:  "https://headscale.example.com:8443",
			baseDomain: "example.net",
			expected:   "headscale.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				ServerURL:  tt.serverURL,
				BaseDomain: tt.baseDomain,
			}
			assert.Equal(t, tt.expected, cfg.Domain())
		})
	}
}

func TestIPAllocationStrategyValidation(t *testing.T) {
	validStrategies := []IPAllocationStrategy{
		IPAllocationStrategySequential,
		IPAllocationStrategyRandom,
	}

	for _, strategy := range validStrategies {
		t.Run(string(strategy), func(t *testing.T) {
			assert.Contains(t, validStrategies, strategy)
		})
	}
}

func TestNodeConfigDefaults(t *testing.T) {
	cfg := NodeConfig{
		Ephemeral: EphemeralConfig{
			InactivityTimeout: 120 * time.Second,
		},
		Routes: RouteConfig{
			HA: HARouteConfig{
				ProbeInterval: 10 * time.Second,
				ProbeTimeout:  5 * time.Second,
			},
		},
	}

	assert.Equal(t, 120*time.Second, cfg.Ephemeral.InactivityTimeout)
	assert.Equal(t, 10*time.Second, cfg.Routes.HA.ProbeInterval)
	assert.Equal(t, 5*time.Second, cfg.Routes.HA.ProbeTimeout)
}

func TestDatabaseConfig(t *testing.T) {
	tests := []struct {
		name   string
		config DatabaseConfig
	}{
		{
			name: "sqlite config",
			config: DatabaseConfig{
				Type: "sqlite",
				Sqlite: SqliteConfig{
					Path:          "/var/lib/headscale/db.sqlite",
					WriteAheadLog: true,
				},
			},
		},
		{
			name: "postgres config",
			config: DatabaseConfig{
				Type: "postgres",
				Postgres: PostgresConfig{
					Host:     "localhost",
					Port:     5432,
					Name:     "headscale",
					User:     "headscale",
					Pass:     "secret",
					Ssl:      "disable",
					MaxOpenConnections: 10,
					MaxIdleConnections: 5,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotEmpty(t, tt.config.Type)
		})
	}
}

func TestDERPConfig(t *testing.T) {
	cfg := DERPConfig{
		ServerEnabled:       true,
		ServerRegionID:     900,
		ServerRegionCode:    "headscale",
		ServerRegionName:    "Headscale Embedded DERP",
		ServerVerifyClients: true,
		STUNAddr:           "0.0.0.0:3478",
		AutoUpdate:         true,
		UpdateFrequency:    3 * time.Hour,
	}

	assert.True(t, cfg.ServerEnabled)
	assert.Equal(t, 900, cfg.ServerRegionID)
	assert.Equal(t, "headscale", cfg.ServerRegionCode)
	assert.True(t, cfg.ServerVerifyClients)
	assert.Equal(t, "0.0.0.0:3478", cfg.STUNAddr)
	assert.True(t, cfg.AutoUpdate)
	assert.Equal(t, 3*time.Hour, cfg.UpdateFrequency)
}

func TestDNSConfig(t *testing.T) {
	cfg := DNSConfig{
		MagicDNS:         true,
		BaseDomain:       "headscale.net",
		OverrideLocalDNS: true,
		Nameservers: NameserversConfig{
			Global: []string{"1.1.1.1", "8.8.8.8"},
			Split: map[string][]string{
				"internal.example.com": {"10.0.0.1"},
			},
		},
		SearchDomains: []string{"example.com"},
	}

	assert.True(t, cfg.MagicDNS)
	assert.Equal(t, "headscale.net", cfg.BaseDomain)
	assert.True(t, cfg.OverrideLocalDNS)
	assert.Len(t, cfg.Nameservers.Global, 2)
	assert.Contains(t, cfg.Nameservers.Split, "internal.example.com")
	assert.Len(t, cfg.SearchDomains, 1)
}

func TestTuningConfigDefaults(t *testing.T) {
	cfg := TuningConfig{
		NotifierSendTimeout:            800 * time.Millisecond,
		BatchChangeDelay:              800 * time.Millisecond,
		NodeMapSessionBufferedChanSize: 30,
		BatcherWorkers:                4,
		RegisterCacheExpiration:       5 * time.Minute,
		RegisterCacheMaxEntries:       1000,
		NodeStoreBatchSize:            100,
		NodeStoreBatchTimeout:         500 * time.Millisecond,
	}

	assert.Equal(t, 800*time.Millisecond, cfg.NotifierSendTimeout)
	assert.Equal(t, 800*time.Millisecond, cfg.BatchChangeDelay)
	assert.Equal(t, 30, cfg.NodeMapSessionBufferedChanSize)
	assert.Equal(t, 4, cfg.BatcherWorkers)
	assert.Equal(t, 5*time.Minute, cfg.RegisterCacheExpiration)
	assert.Equal(t, 1000, cfg.RegisterCacheMaxEntries)
	assert.Equal(t, 100, cfg.NodeStoreBatchSize)
	assert.Equal(t, 500*time.Millisecond, cfg.NodeStoreBatchTimeout)
}

func TestTLSConfig(t *testing.T) {
	cfg := TLSConfig{
		CertPath: "/etc/ssl/cert.pem",
		KeyPath:  "/etc/ssl/key.pem",
		LetsEncrypt: LetsEncryptConfig{
			Listen:        ":80",
			Hostname:      "headscale.example.com",
			CacheDir:     "/var/lib/headscale/acme",
			ChallengeType: "HTTP-01",
		},
	}

	assert.Equal(t, "/etc/ssl/cert.pem", cfg.CertPath)
	assert.Equal(t, "/etc/ssl/key.pem", cfg.KeyPath)
	assert.Equal(t, ":80", cfg.LetsEncrypt.Listen)
	assert.Equal(t, "headscale.example.com", cfg.LetsEncrypt.Hostname)
}

func TestOIDCConfig(t *testing.T) {
	cfg := OIDCConfig{
		OnlyStartIfOIDCIsAvailable: true,
		Issuer:                    "https://accounts.google.com",
		ClientID:                  "my-client-id",
		ClientSecret:              "my-secret",
		Scope:                     []string{"openid", "profile", "email"},
		AllowedDomains:           []string{"example.com"},
		AllowedUsers:              []string{"admin@example.com"},
		AllowedGroups:             []string{"admins"},
		EmailVerifiedRequired:     true,
		UseExpiryFromToken:        false,
		PKCE: PKCEConfig{
			Enabled: true,
			Method:  "S256",
		},
	}

	assert.True(t, cfg.OnlyStartIfOIDCIsAvailable)
	assert.Equal(t, "https://accounts.google.com", cfg.Issuer)
	assert.Equal(t, "my-client-id", cfg.ClientID)
	assert.Len(t, cfg.Scope, 3)
	assert.True(t, cfg.EmailVerifiedRequired)
	assert.True(t, cfg.PKCE.Enabled)
	assert.Equal(t, "S256", cfg.PKCE.Method)
}

func TestCleanIdentifier(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"user123", "user123"},
		{"  spaced  ", "spaced"},
		{"a/b/c", "a/b/c"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := CleanIdentifier(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidatePKCEMethod(t *testing.T) {
	tests := []struct {
		method   string
		hasError bool
	}{
		{"plain", false},
		{"S256", false},
		{"invalid", true},
		{"", true},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			err := validatePKCEMethod(tt.method)
			if tt.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConfigPrefixValidation(t *testing.T) {
	tests := []struct {
		name      string
		prefixV4  string
		prefixV6  string
		expectErr bool
	}{
		{
			name:      "valid IPv4 only",
			prefixV4:  "100.64.0.0/10",
			expectErr: false,
		},
		{
			name:      "valid IPv6 only",
			prefixV6:  "fd7a:115c:a1e0::/48",
			expectErr: false,
		},
		{
			name:      "both prefixes",
			prefixV4:  "100.64.0.0/10",
			prefixV6:  "fd7a:115c:a1e0::/48",
			expectErr: false,
		},
		{
			name:      "no prefixes",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{}
			if tt.prefixV4 != "" {
				p := netip.MustParsePrefix(tt.prefixV4)
				cfg.PrefixV4 = &p
			}
			if tt.prefixV6 != "" {
				p := netip.MustParsePrefix(tt.prefixV6)
				cfg.PrefixV6 = &p
			}

			hasV4 := cfg.PrefixV4 != nil
			hasV6 := cfg.PrefixV6 != nil
			hasAny := hasV4 || hasV6

			if tt.expectErr {
				assert.False(t, hasAny)
			} else {
				assert.True(t, hasAny)
			}
		})
	}
}

func TestUserFromClaim(t *testing.T) {
	tests := []struct {
		name               string
		claims             OIDCClaims
		emailVerified      bool
		expectedEmail      string
		expectedName       string
	}{
		{
			name: "standard claims",
			claims: OIDCClaims{
				Sub:               "123",
				Iss:               "https://example.com",
				Username:          "testuser",
				Email:             "test@example.com",
				EmailVerified:     true,
				Name:              "Test User",
				ProfilePictureURL: "https://example.com/photo.jpg",
			},
			emailVerified: true,
			expectedEmail: "test@example.com",
			expectedName:  "Test User",
		},
		{
			name: "email not verified",
			claims: OIDCClaims{
				Sub:           "123",
				Email:         "test@example.com",
				EmailVerified: false,
			},
			emailVerified: true,
			expectedEmail: "",
		},
		{
			name: "email not verified but requirement disabled",
			claims: OIDCClaims{
				Sub:           "123",
				Email:         "test@example.com",
				EmailVerified: false,
			},
			emailVerified: false,
			expectedEmail: "test@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &User{}
			user.FromClaim(&tt.claims, tt.emailVerified)

			if tt.expectedEmail != "" {
				assert.Equal(t, tt.expectedEmail, user.Email)
			}
			if tt.expectedName != "" {
				assert.Equal(t, tt.expectedName, user.DisplayName)
			}
			assert.Equal(t, "oidc", user.Provider, "provider should be oidc")
		})
	}
}