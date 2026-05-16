package types

import (
	"errors"
	"net/netip"
	"slices"
	"strconv"
	"time"

	"github.com/juanfont/headscale-v2/internal/policy/matcher"
	"go4.org/netipx"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
)

var (
	ErrNodeAddressesInvalid = errors.New("parsing node addresses")
	ErrHostnameTooLong      = errors.New("hostname too long, cannot accept more than 255 ASCII chars")
	ErrNodeHasNoGivenName  = errors.New("node has no given name")
	ErrNodeUserHasNoName   = errors.New("node user has no name")
	ErrInvalidNodeView      = errors.New("cannot convert invalid NodeView to tailcfg.Node")
)

type (
	NodeID  uint64
	NodeIDs []NodeID
)

func (n NodeIDs) Len() int           { return len(n) }
func (n NodeIDs) Less(i, j int) bool { return n[i] < n[j] }
func (n NodeIDs) Swap(i, j int)      { n[i], n[j] = n[j], n[i] }

func (id NodeID) StableID() tailcfg.StableNodeID {
	return tailcfg.StableNodeID(strconv.FormatUint(uint64(id), 10))
}

func (id NodeID) NodeID() tailcfg.NodeID {
	return tailcfg.NodeID(id)
}

func (id NodeID) Uint64() uint64 {
	return uint64(id)
}

func (id NodeID) String() string {
	return strconv.FormatUint(id.Uint64(), 10)
}

func ParseNodeID(s string) (NodeID, error) {
	id, err := strconv.ParseUint(s, 10, 64)
	return NodeID(id), err
}

type RouteFunc func(id NodeID) []netip.Prefix

type ViaRouteResult struct {
	Include    []netip.Prefix
	Exclude    []netip.Prefix
	UsePrimary []netip.Prefix
}

type Node struct {
	ID NodeID

	MachineKey key.MachinePublic
	NodeKey    key.NodePublic
	DiscoKey   key.DiscoPublic

	Endpoints []netip.AddrPort

	Hostinfo *tailcfg.Hostinfo

	IPv4 *netip.Addr
	IPv6 *netip.Addr

	Hostname string
	GivenName string

	UserID *uint
	User   *User

	RegisterMethod string

	Tags []string

	AuthKeyID *uint64
	AuthKey   *PreAuthKey

	Expiry *time.Time

	LastSeen *time.Time

	ApprovedRoutes []netip.Prefix

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time

	IsOnline *bool

	Unhealthy bool

	SessionEpoch uint64
}

type Nodes []*Node

func (node *Node) IsExpired() bool {
	if node.Expiry == nil || node.Expiry.IsZero() {
		return false
	}
	return time.Since(*node.Expiry) > 0
}

func (node *Node) IsEphemeral() bool {
	return node.AuthKey != nil && node.AuthKey.Ephemeral
}

func (node *Node) IPs() []netip.Addr {
	var ret []netip.Addr
	if node.IPv4 != nil {
		ret = append(ret, *node.IPv4)
	}
	if node.IPv6 != nil {
		ret = append(ret, *node.IPv6)
	}
	return ret
}

func (node *Node) IsTagged() bool {
	return len(node.Tags) > 0
}

func (node *Node) IsUserOwned() bool {
	return !node.IsTagged()
}

func (node *Node) HasTag(tag string) bool {
	if node == nil {
		return false
	}
	for _, t := range node.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

func (node *Node) TypedUserID() UserID {
	if node.UserID == nil {
		return 0
	}
	return UserID(*node.UserID)
}

func (node *Node) AnnouncedRoutes() []netip.Prefix {
	if node.Hostinfo == nil {
		return nil
	}
	return node.Hostinfo.RoutableIPs
}

func (node *Node) SubnetRoutes() []netip.Prefix {
	var routes []netip.Prefix
	for _, route := range node.AnnouncedRoutes() {
		if isExitRoute(route) {
			continue
		}
		for _, approved := range node.ApprovedRoutes {
			if route == approved {
				routes = append(routes, route)
				break
			}
		}
	}
	return routes
}

func (node *Node) ExitRoutes() []netip.Prefix {
	var routes []netip.Prefix
	for _, route := range node.AnnouncedRoutes() {
		if isExitRoute(route) {
			for _, approved := range node.ApprovedRoutes {
				if route == approved {
					routes = append(routes, route)
					break
				}
			}
		}
	}
	return routes
}

func (node *Node) IsExitNode() bool {
	return len(node.ExitRoutes()) > 0
}

func (node *Node) AllApprovedRoutes() []netip.Prefix {
	return append(node.SubnetRoutes(), node.ExitRoutes()...)
}

func isExitRoute(p netip.Prefix) bool {
	return p == netip.MustParsePrefix("0.0.0.0/0") || p == netip.MustParsePrefix("::/0")
}

// CanAccess determines whether this node can access node2 based on the
// provided matchers from the policy filter rules.
func (node *Node) CanAccess(matchers []matcher.Match, node2 *Node) bool {
	src := node.IPs()
	allowedIPs := node2.IPs()
	srcRoutes := node.SubnetRoutes()
	dstRoutes := node2.SubnetRoutes()
	dstIsExit := node2.IsExitNode()

	for _, m := range matchers {
		srcMatchesIP := m.SrcsContainsIPs(src...)
		srcMatchesRoutes := len(srcRoutes) > 0 && m.SrcsOverlapsPrefixes(srcRoutes...)

		if !srcMatchesIP && !srcMatchesRoutes {
			continue
		}

		if m.DestsContainsIP(allowedIPs...) {
			return true
		}

		if len(dstRoutes) > 0 && m.DestsOverlapsPrefixes(dstRoutes...) {
			return true
		}

		if dstIsExit && m.DestsIsTheInternet() {
			return true
		}
	}

	return false
}

// CanAccessRoute determines whether a specific route prefix should be
// visible to this node based on the given matchers.
func (node *Node) CanAccessRoute(matchers []matcher.Match, route netip.Prefix) bool {
	src := node.IPs()
	subnetRoutes := node.SubnetRoutes()

	for _, matcher := range matchers {
		if matcher.SrcsContainsIPs(src...) && matcher.DestsOverlapsPrefixes(route) {
			return true
		}

		if matcher.SrcsOverlapsPrefixes(subnetRoutes...) && matcher.DestsContainsIP(src...) {
			return true
		}
	}

	return false
}

func (node *Node) Prefixes() []netip.Prefix {
	ips := node.IPs()
	if len(ips) == 0 {
		return nil
	}
	addrs := make([]netip.Prefix, 0, len(ips))
	for _, nodeAddress := range ips {
		ip := netip.PrefixFrom(nodeAddress, nodeAddress.BitLen())
		addrs = append(addrs, ip)
	}
	return addrs
}

func (node *Node) String() string {
	return node.Hostname
}

func (node *Node) AppendToIPSet(builder *netipx.IPSetBuilder) {
	if node.IPv4 != nil {
		builder.Add(*node.IPv4)
	}
	if node.IPv6 != nil {
		builder.Add(*node.IPv6)
	}
}

func (node *Node) ApplyPeerChange(change *tailcfg.PeerChange) {
	if change.Key != nil {
		node.NodeKey = *change.Key
	}
	if change.DiscoKey != nil {
		node.DiscoKey = *change.DiscoKey
	}
	if change.Online != nil {
		node.IsOnline = change.Online
	}
	if change.Endpoints != nil {
		node.Endpoints = change.Endpoints
	}
	if change.DERPRegion != 0 {
		if node.Hostinfo == nil {
			node.Hostinfo = &tailcfg.Hostinfo{
				NetInfo: &tailcfg.NetInfo{
					PreferredDERP: change.DERPRegion,
				},
			}
		} else if node.Hostinfo.NetInfo == nil {
			node.Hostinfo.NetInfo = &tailcfg.NetInfo{
				PreferredDERP: change.DERPRegion,
			}
		} else {
			node.Hostinfo.NetInfo.PreferredDERP = change.DERPRegion
		}
	}
	node.LastSeen = change.LastSeen
}

func (node *Node) PeerChangeFromMapRequest(req tailcfg.MapRequest) tailcfg.PeerChange {
	ret := tailcfg.PeerChange{
		NodeID: tailcfg.NodeID(node.ID),
	}

	if node.NodeKey.String() != req.NodeKey.String() {
		ret.Key = &req.NodeKey
	}

	if node.DiscoKey.String() != req.DiscoKey.String() {
		ret.DiscoKey = &req.DiscoKey
	}

	if node.Hostinfo != nil && node.Hostinfo.NetInfo != nil &&
		req.Hostinfo != nil && req.Hostinfo.NetInfo != nil &&
		node.Hostinfo.NetInfo.PreferredDERP != req.Hostinfo.NetInfo.PreferredDERP {
		ret.DERPRegion = req.Hostinfo.NetInfo.PreferredDERP
	}

	if req.Hostinfo != nil && req.Hostinfo.NetInfo != nil {
		if node.Hostinfo == nil {
			ret.DERPRegion = req.Hostinfo.NetInfo.PreferredDERP
		} else if node.Hostinfo.NetInfo == nil {
			ret.DERPRegion = req.Hostinfo.NetInfo.PreferredDERP
		} else if node.Hostinfo.NetInfo.PreferredDERP != req.Hostinfo.NetInfo.PreferredDERP {
			ret.DERPRegion = req.Hostinfo.NetInfo.PreferredDERP
		}
	}

	if EndpointsChanged(node.Endpoints, req.Endpoints) {
		ret.Endpoints = req.Endpoints
	}

	now := time.Now()
	ret.LastSeen = &now

	return ret
}

func EndpointsChanged(oldEndpoints, newEndpoints []netip.AddrPort) bool {
	if len(oldEndpoints) != len(newEndpoints) {
		return true
	}

	if len(oldEndpoints) == 0 {
		return false
	}

	oldCopy := slices.Clone(oldEndpoints)
	newCopy := slices.Clone(newEndpoints)

	slices.SortFunc(oldCopy, netip.AddrPort.Compare)
	slices.SortFunc(newCopy, netip.AddrPort.Compare)

	for i := range oldCopy {
		if oldCopy[i] != newCopy[i] {
			return true
		}
	}

	return false
}
