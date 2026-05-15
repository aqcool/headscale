package util

import (
	"net/netip"
	"sync"

	"go4.org/netipx"
	"tailscale.com/net/tsaddr"
)

// TheInternet returns the IPSet for the Internet.
var TheInternet = sync.OnceValue(func() *netipx.IPSet {
	var internetBuilder netipx.IPSetBuilder
	internetBuilder.AddPrefix(netip.MustParsePrefix("2000::/3"))
	internetBuilder.AddPrefix(tsaddr.AllIPv4())

	// Remove private network addresses
	internetBuilder.RemovePrefix(netip.MustParsePrefix("fc00::/7"))
	internetBuilder.RemovePrefix(netip.MustParsePrefix("10.0.0.0/8"))
	internetBuilder.RemovePrefix(netip.MustParsePrefix("172.16.0.0/12"))
	internetBuilder.RemovePrefix(netip.MustParsePrefix("192.168.0.0/16"))

	// Remove Tailscale networks
	internetBuilder.RemovePrefix(tsaddr.TailscaleULARange())
	internetBuilder.RemovePrefix(tsaddr.CGNATRange())

	// Remove link-local
	internetBuilder.RemovePrefix(netip.MustParsePrefix("fe80::/10"))
	internetBuilder.RemovePrefix(netip.MustParsePrefix("169.254.0.0/16"))

	theInternetSet, _ := internetBuilder.IPSet()
	return theInternetSet
})

// PrefixesToString converts prefixes to strings.
func PrefixesToString(prefixes []netip.Prefix) []string {
	ret := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		ret = append(ret, prefix.String())
	}
	return ret
}