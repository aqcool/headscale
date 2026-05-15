package util

import "net/netip"

const Base10 = 10
const MaxHostnameLength = 255

func PrefixesToString(prefixes []netip.Prefix) []string {
	if len(prefixes) == 0 {
		return nil
	}
	result := make([]string, len(prefixes))
	for i, p := range prefixes {
		result[i] = p.String()
	}
	return result
}
