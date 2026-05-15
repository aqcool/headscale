package ip

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/netip"
	"sync"

	"github.com/juanfont/headscale-v2/internal/types"
	"go4.org/netipx"
	"tailscale.com/net/tsaddr"
)

var (
	ErrGeneratedIPBytesInvalid = errors.New("generated ip bytes are invalid ip")
	ErrGeneratedIPNotInPrefix  = errors.New("generated ip not in prefix")
	ErrCouldNotAllocateIP      = errors.New("failed to allocate IP")
	ErrIPAllocatorNil          = errors.New("ip allocator was nil")
)

type IPAllocator struct {
	mu sync.Mutex

	prefix4 *netip.Prefix
	prefix6 *netip.Prefix

	prev4 netip.Addr
	prev6 netip.Addr

	strategy types.IPAllocationStrategy

	usedIPs netipx.IPSetBuilder
}

func NewIPAllocator(
	prefix4, prefix6 *netip.Prefix,
	strategy types.IPAllocationStrategy,
) *IPAllocator {
	ret := &IPAllocator{
		prefix4:  prefix4,
		prefix6:  prefix6,
		strategy: strategy,
	}

	var ips netipx.IPSetBuilder

	if prefix4 != nil {
		network4, broadcast4 := getIPPrefixEndpoints(*prefix4)
		ips.Add(network4)
		ips.Add(broadcast4)
		ret.prev4 = network4
	}

	if prefix6 != nil {
		network6, broadcast6 := getIPPrefixEndpoints(*prefix6)
		ips.Add(network6)
		ips.Add(broadcast6)
		ret.prev6 = network6
	}

	ret.usedIPs = ips

	return ret
}

func getIPPrefixEndpoints(pfx netip.Prefix) (netip.Addr, netip.Addr) {
	rang := netipx.RangeOfPrefix(pfx)
	return rang.From(), rang.To()
}

func (i *IPAllocator) Next() (*netip.Addr, *netip.Addr, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	var (
		err  error
		ret4 *netip.Addr
		ret6 *netip.Addr
	)

	if i.prefix4 != nil {
		ret4, err = i.next(i.prev4, i.prefix4)
		if err != nil {
			return nil, nil, fmt.Errorf("allocating IPv4 address: %w", err)
		}
		i.prev4 = *ret4
	}

	if i.prefix6 != nil {
		ret6, err = i.next(i.prev6, i.prefix6)
		if err != nil {
			return nil, nil, fmt.Errorf("allocating IPv6 address: %w", err)
		}
		i.prev6 = *ret6
	}

	return ret4, ret6, nil
}

func (i *IPAllocator) next(prev netip.Addr, prefix *netip.Prefix) (*netip.Addr, error) {
	var (
		ip  netip.Addr
		err error
	)

	switch i.strategy {
	case types.IPAllocationStrategySequential:
		ip = prev.Next()
	case types.IPAllocationStrategyRandom:
		ip, err = randomNext(*prefix)
		if err != nil {
			return nil, fmt.Errorf("getting random IP: %w", err)
		}
	}

	set, err := i.usedIPs.IPSet()
	if err != nil {
		return nil, err
	}

	for {
		if !prefix.Contains(ip) {
			return nil, ErrCouldNotAllocateIP
		}

		if set.Contains(ip) || isTailscaleReservedIP(ip) {
			switch i.strategy {
			case types.IPAllocationStrategySequential:
				ip = ip.Next()
			case types.IPAllocationStrategyRandom:
				ip, err = randomNext(*prefix)
				if err != nil {
					return nil, fmt.Errorf("getting random IP: %w", err)
				}
			}
			continue
		}

		i.usedIPs.Add(ip)
		return &ip, nil
	}
}

func randomNext(pfx netip.Prefix) (netip.Addr, error) {
	rang := netipx.RangeOfPrefix(pfx)
	fromIP, toIP := rang.From(), rang.To()

	var from, to big.Int

	from.SetBytes(fromIP.AsSlice())
	to.SetBytes(toIP.AsSlice())

	tempMax := big.NewInt(0).Sub(&to, &from)

	out, err := rand.Int(rand.Reader, tempMax)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("generating random IP: %w", err)
	}

	valInRange := big.NewInt(0).Add(&from, out)

	ip, ok := netip.AddrFromSlice(valInRange.Bytes())
	if !ok {
		return netip.Addr{}, ErrGeneratedIPBytesInvalid
	}

	if !pfx.Contains(ip) {
		return netip.Addr{}, fmt.Errorf("%w: ip(%s) not in prefix(%s)", ErrGeneratedIPNotInPrefix, ip.String(), pfx.String())
	}

	return ip, nil
}

func isTailscaleReservedIP(ip netip.Addr) bool {
	return tsaddr.ChromeOSVMRange().Contains(ip) ||
		tsaddr.TailscaleServiceIP() == ip ||
		tsaddr.TailscaleServiceIPv6() == ip
}

func (i *IPAllocator) AddUsedIP(ip netip.Addr) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.usedIPs.Add(ip)
}

func (i *IPAllocator) FreeIPs(ips []netip.Addr) {
	i.mu.Lock()
	defer i.mu.Unlock()
	for _, ip := range ips {
		i.usedIPs.Remove(ip)
	}
}