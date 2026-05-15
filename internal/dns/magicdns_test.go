package dns

import (
	"net/netip"
	"testing"

	"github.com/juanfont/headscale-v2/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateMagicDNSConfig_Disabled(t *testing.T) {
	cfg := &types.DNSConfig{
		MagicDNS: false,
	}

	result := GenerateMagicDNSConfig(cfg, "headscale.net", nil, nil)
	assert.Nil(t, result)
}

func TestGenerateMagicDNSConfig_NoBaseDomain(t *testing.T) {
	cfg := &types.DNSConfig{
		MagicDNS: true,
	}

	result := GenerateMagicDNSConfig(cfg, "", nil, nil)
	assert.Nil(t, result)
}

func TestGenerateMagicDNSConfig_Basic(t *testing.T) {
	cfg := &types.DNSConfig{
		MagicDNS:         true,
		BaseDomain:       "headscale.net",
		OverrideLocalDNS: true,
		Nameservers: types.NameserversConfig{
			Global: []string{"1.1.1.1", "8.8.8.8"},
		},
		SearchDomains: []string{"example.com"},
	}

	prefix4 := netip.MustParsePrefix("100.64.0.0/10")
	prefix6 := netip.MustParsePrefix("fd7a:115c:a1e0::/48")

	result := GenerateMagicDNSConfig(cfg, "headscale.net", &prefix4, &prefix6)

	require.NotNil(t, result)
	assert.True(t, result.Proxied)
	assert.Len(t, result.Resolvers, 2)
	assert.Contains(t, result.Domains, "headscale.net")
	assert.Contains(t, result.Domains, "example.com")
}

func TestGenerateMagicDNSConfig_Fallback(t *testing.T) {
	cfg := &types.DNSConfig{
		MagicDNS:         true,
		BaseDomain:       "headscale.net",
		OverrideLocalDNS: false,
		Nameservers: types.NameserversConfig{
			Global: []string{"1.1.1.1"},
		},
	}

	result := GenerateMagicDNSConfig(cfg, "headscale.net", nil, nil)

	require.NotNil(t, result)
	assert.Len(t, result.FallbackResolvers, 1)
	assert.Empty(t, result.Resolvers)
}

func TestGenerateMagicDNSConfig_SplitDNS(t *testing.T) {
	cfg := &types.DNSConfig{
		MagicDNS:   true,
		BaseDomain: "headscale.net",
		Nameservers: types.NameserversConfig{
			Split: map[string][]string{
				"internal.example.com": {"10.0.0.1"},
			},
		},
	}

	result := GenerateMagicDNSConfig(cfg, "headscale.net", nil, nil)

	require.NotNil(t, result)
	assert.Contains(t, result.Routes, "internal.example.com")
	assert.Len(t, result.Routes["internal.example.com"], 1)
}

func TestGlobalResolvers(t *testing.T) {
	tests := []struct {
		name      string
		input     []string
		expected  int
	}{
		{
			name:     "ip addresses",
			input:    []string{"1.1.1.1", "8.8.8.8"},
			expected: 2,
		},
		{
			name:     "http resolver",
			input:    []string{"https://dns.example.com/dns-query"},
			expected: 1,
		},
		{
			name:     "mixed",
			input:    []string{"1.1.1.1", "https://dns.example.com/dns-query"},
			expected: 2,
		},
		{
			name:     "empty",
			input:    []string{},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := globalResolvers(tt.input)
			assert.Len(t, result, tt.expected)
		})
	}
}

func TestSplitResolvers(t *testing.T) {
	input := map[string][]string{
		"internal.example.com": {"10.0.0.1", "10.0.0.2"},
		"external.example.com": {"https://dns.example.com/dns-query"},
	}

	result := splitResolvers(input)

	assert.Len(t, result, 2)
	assert.Len(t, result["internal.example.com"], 2)
	assert.Len(t, result["external.example.com"], 1)
}

func TestGenerateIPv4DNSRootDomain(t *testing.T) {
	prefix := netip.MustParsePrefix("100.64.0.0/10")

	fqdns := GenerateIPv4DNSRootDomain(prefix)

	assert.NotEmpty(t, fqdns)
}

func TestGenerateIPv6DNSRootDomain(t *testing.T) {
	prefix := netip.MustParsePrefix("fd7a:115c:a1e0::/48")

	fqdns := GenerateIPv6DNSRootDomain(prefix)

	assert.NotEmpty(t, fqdns)
}

func TestGenerateMagicDNSRootDomains(t *testing.T) {
	prefix4 := netip.MustParsePrefix("100.64.0.0/10")
	prefix6 := netip.MustParsePrefix("fd7a:115c:a1e0::/48")

	domains := GenerateMagicDNSRootDomains("headscale.net", &prefix4, &prefix6)

	assert.NotEmpty(t, domains)
}

func TestGenerateMagicDNSRootDomains_NoBaseDomain(t *testing.T) {
	prefix4 := netip.MustParsePrefix("100.64.0.0/10")

	domains := GenerateMagicDNSRootDomains("", &prefix4, nil)

	assert.NotEmpty(t, domains)
}

func TestDNSRootForUser(t *testing.T) {
	prefix4 := netip.MustParsePrefix("100.64.0.0/10")

	routes := DNSRootForUser("headscale.net", &prefix4, nil)

	assert.NotEmpty(t, routes)
}

func TestConvertExtraRecords(t *testing.T) {
	records := []types.DNSRecord{
		{Name: "service.headscale.net.", Type: "A", Value: "10.0.0.1"},
		{Name: "api.headscale.net.", Type: "A", Value: "10.0.0.2"},
	}

	result := convertExtraRecords(records)

	assert.Len(t, result, 2)
	assert.Equal(t, "service.headscale.net.", result[0].Name)
	assert.Equal(t, "10.0.0.1", result[0].Value)
}

func TestConvertExtraRecords_Empty(t *testing.T) {
	result := convertExtraRecords([]types.DNSRecord{})
	assert.Nil(t, result)
}