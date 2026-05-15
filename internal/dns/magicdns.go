package dns

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"go4.org/netipx"
	"github.com/juanfont/headscale-v2/internal/types"
	"tailscale.com/tailcfg"
	"tailscale.com/types/dnstype"
	"tailscale.com/util/dnsname"
)

const (
	byteSize          = 8
	ipv4AddressLength = 32
	ipv6AddressLength = 128
	labelHostnameLength = 63
)

func GenerateMagicDNSConfig(cfg *types.DNSConfig, baseDomain string, prefix4, prefix6 *netip.Prefix) *tailcfg.DNSConfig {
	if !cfg.MagicDNS {
		return nil
	}

	if baseDomain == "" {
		return nil
	}

	dnsCfg := &tailcfg.DNSConfig{
		Proxied: true,
	}

	if cfg.OverrideLocalDNS && len(cfg.Nameservers.Global) > 0 {
		dnsCfg.Resolvers = globalResolvers(cfg.Nameservers.Global)
	} else {
		dnsCfg.FallbackResolvers = globalResolvers(cfg.Nameservers.Global)
	}

	routes := splitResolvers(cfg.Nameservers.Split)
	dnsCfg.Routes = routes

	if baseDomain != "" {
		dnsCfg.Domains = []string{baseDomain}
	}

	dnsCfg.Domains = append(dnsCfg.Domains, cfg.SearchDomains...)

	dnsCfg.ExtraRecords = convertExtraRecords(cfg.ExtraRecords)

	return dnsCfg
}

func globalResolvers(nameservers []string) []*dnstype.Resolver {
	var resolvers []*dnstype.Resolver

	for _, nsStr := range nameservers {
		if _, err := netip.ParseAddr(nsStr); err == nil {
			resolvers = append(resolvers, &dnstype.Resolver{
				Addr: nsStr,
			})
			continue
		}

		if strings.HasPrefix(nsStr, "http://") || strings.HasPrefix(nsStr, "https://") {
			resolvers = append(resolvers, &dnstype.Resolver{
				Addr: nsStr,
			})
			continue
		}
	}

	return resolvers
}

func splitResolvers(split map[string][]string) map[string][]*dnstype.Resolver {
	routes := make(map[string][]*dnstype.Resolver)

	for domain, nameservers := range split {
		var resolvers []*dnstype.Resolver

		for _, nsStr := range nameservers {
			if _, err := netip.ParseAddr(nsStr); err == nil {
				resolvers = append(resolvers, &dnstype.Resolver{
					Addr: nsStr,
				})
				continue
			}

			if strings.HasPrefix(nsStr, "http://") || strings.HasPrefix(nsStr, "https://") {
				resolvers = append(resolvers, &dnstype.Resolver{
					Addr: nsStr,
				})
				continue
			}
		}

		if len(resolvers) > 0 {
			routes[domain] = resolvers
		}
	}

	return routes
}

func GenerateMagicDNSRootDomains(baseDomain string, prefix4, prefix6 *netip.Prefix) []*dnstype.Resolver {
	var domains []*dnstype.Resolver

	if baseDomain != "" {
		fqdn, err := dnsname.ToFQDN(baseDomain)
		if err == nil {
			domains = append(domains, &dnstype.Resolver{
				Addr: fqdn.WithTrailingDot(),
			})
		}
	}

	if prefix4 != nil {
		ipv4Domains := GenerateIPv4DNSRootDomain(*prefix4)
		for _, fqdn := range ipv4Domains {
			domains = append(domains, &dnstype.Resolver{
				Addr: fqdn.WithTrailingDot(),
			})
		}
	}

	if prefix6 != nil {
		ipv6Domains := GenerateIPv6DNSRootDomain(*prefix6)
		for _, fqdn := range ipv6Domains {
			domains = append(domains, &dnstype.Resolver{
				Addr: fqdn.WithTrailingDot(),
			})
		}
	}

	return domains
}

func GenerateIPv4DNSRootDomain(ipPrefix netip.Prefix) []dnsname.FQDN {
	netRange := netipx.PrefixIPNet(ipPrefix)
	maskBits, _ := netRange.Mask.Size()

	lastOctet := maskBits / byteSize
	wildcardBits := byteSize - maskBits%byteSize

	minVal := uint(netRange.IP[lastOctet])
	maxVal := (minVal + 1<<uint(wildcardBits)) - 1

	rdnsSlice := []string{}
	for i := lastOctet - 1; i >= 0; i-- {
		rdnsSlice = append(rdnsSlice, strconv.FormatUint(uint64(netRange.IP[i]), 10))
	}

	rdnsSlice = append(rdnsSlice, "in-addr.arpa.")
	rdnsBase := strings.Join(rdnsSlice, ".")

	fqdns := make([]dnsname.FQDN, 0, maxVal-minVal+1)
	for i := minVal; i <= maxVal; i++ {
		fqdn, err := dnsname.ToFQDN(fmt.Sprintf("%d.%s", i, rdnsBase))
		if err != nil {
			continue
		}

		fqdns = append(fqdns, fqdn)
	}

	return fqdns
}

func GenerateIPv6DNSRootDomain(ipPrefix netip.Prefix) []dnsname.FQDN {
	const nibbleLen = 4

	maskBits, _ := netipx.PrefixIPNet(ipPrefix).Mask.Size()
	expanded := ipPrefix.Addr().StringExpanded()
	nibbleStr := strings.Map(func(r rune) rune {
		if r == ':' {
			return -1
		}
		return r
	}, expanded)

	prefixConstantParts := []string{}
	for i := range maskBits / nibbleLen {
		prefixConstantParts = append(
			[]string{string(nibbleStr[i])},
			prefixConstantParts...)
	}

	makeDomain := func(variablePrefix ...string) (dnsname.FQDN, error) {
		prefix := strings.Join(append(variablePrefix, prefixConstantParts...), ".")
		return dnsname.ToFQDN(prefix + ".ip6.arpa")
	}

	var fqdns []dnsname.FQDN

	if maskBits%4 == 0 {
		dom, _ := makeDomain()
		fqdns = append(fqdns, dom)
	} else {
		domCount := 1 << (maskBits % nibbleLen)

		fqdns = make([]dnsname.FQDN, 0, domCount)
		for i := range domCount {
			varNibble := fmt.Sprintf("%x", i)

			dom, err := makeDomain(varNibble)
			if err != nil {
				continue
			}

			fqdns = append(fqdns, dom)
		}
	}

	return fqdns
}

func DNSRootForUser(baseDomain string, prefix4, prefix6 *netip.Prefix) map[string][]*dnstype.Resolver {
	root := GenerateMagicDNSRootDomains(baseDomain, prefix4, prefix6)

	routes := make(map[string][]*dnstype.Resolver)
	for _, resolver := range root {
		domain := strings.TrimSuffix(resolver.Addr, ".")
		routes[domain] = []*dnstype.Resolver{
			{
				Addr: "100.100.100.100",
			},
		}
	}

	return routes
}

func convertExtraRecords(records []types.DNSRecord) []tailcfg.DNSRecord {
	if len(records) == 0 {
		return nil
	}
	result := make([]tailcfg.DNSRecord, len(records))
	for i, r := range records {
		result[i] = tailcfg.DNSRecord{
			Name:  r.Name,
			Type:  r.Type,
			Value: r.Value,
		}
	}
	return result
}