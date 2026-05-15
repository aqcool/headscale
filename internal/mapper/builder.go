package mapper

import (
	"fmt"
	"net/netip"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/juanfont/headscale-v2/internal/types"
	"tailscale.com/tailcfg"
	"tailscale.com/types/dnstype"
	"tailscale.com/util/multierr"
)

const nextDNSDoHPrefix = "https://dns.nextdns.io"

// MapResponseBuilder provides a fluent interface for building tailcfg.MapResponse.
type MapResponseBuilder struct {
	resp   *tailcfg.MapResponse
	mapper *Mapper
	nodeID types.NodeID
	capVer tailcfg.CapabilityVersion
	errs   []error
}

// NewMapResponseBuilder creates a new builder with basic fields set.
func NewMapResponseBuilder(m *Mapper, nodeID types.NodeID) *MapResponseBuilder {
	now := time.Now()

	return &MapResponseBuilder{
		resp: &tailcfg.MapResponse{
			KeepAlive:   false,
			ControlTime: &now,
		},
		mapper: m,
		nodeID: nodeID,
		errs:   nil,
	}
}

// addError adds an error to the builder's error list.
func (b *MapResponseBuilder) addError(err error) {
	if err != nil {
		b.errs = append(b.errs, err)
	}
}

// WithCapabilityVersion sets the capability version for the response.
func (b *MapResponseBuilder) WithCapabilityVersion(capVer tailcfg.CapabilityVersion) *MapResponseBuilder {
	b.capVer = capVer
	return b
}

// WithSelfNode adds the requesting node to the response.
func (b *MapResponseBuilder) WithSelfNode() *MapResponseBuilder {
	nv, ok := b.mapper.state.GetNodeByID(b.nodeID)
	if !ok {
		b.addError(ErrNodeNotFoundMapper)
		return b
	}

	tailnode, err := nv.TailNode(
		b.capVer,
		func(id types.NodeID) []netip.Prefix {
			return b.mapper.state.PrimaryRoutesForNode(id)
		},
		b.mapper.cfg)
	if err != nil {
		b.addError(err)
		return b
	}

	b.resp.Node = tailnode
	return b
}

// WithDERPMap adds the DERP map to the response.
func (b *MapResponseBuilder) WithDERPMap() *MapResponseBuilder {
	b.resp.DERPMap = b.mapper.state.DERPMap()
	return b
}

// WithDomain adds the domain configuration.
func (b *MapResponseBuilder) WithDomain() *MapResponseBuilder {
	if b.mapper.cfg != nil {
		b.resp.Domain = b.mapper.cfg.BaseDomain
	}
	return b
}

// WithCollectServicesDisabled sets the collect services flag to false.
func (b *MapResponseBuilder) WithCollectServicesDisabled() *MapResponseBuilder {
	b.resp.CollectServices.Set(false)
	return b
}

// WithSSHPolicy adds SSH policy configuration for the requesting node.
func (b *MapResponseBuilder) WithSSHPolicy() *MapResponseBuilder {
	node, ok := b.mapper.state.GetNodeByID(b.nodeID)
	if !ok {
		b.addError(ErrNodeNotFoundMapper)
		return b
	}

	sshPolicy, err := b.mapper.state.SSHPolicy(node)
	if err != nil {
		b.addError(err)
		return b
	}

	b.resp.SSHPolicy = sshPolicy
	return b
}

// WithPacketFilters adds packet filter rules based on policy.
func (b *MapResponseBuilder) WithPacketFilters() *MapResponseBuilder {
	node, ok := b.mapper.state.GetNodeByID(b.nodeID)
	if !ok {
		b.addError(ErrNodeNotFoundMapper)
		return b
	}

	filter, err := b.mapper.state.FilterForNode(node)
	if err != nil {
		b.addError(err)
		return b
	}

	b.resp.PacketFilters = map[string][]tailcfg.FilterRule{
		"base": filter,
	}
	return b
}

// WithPeers adds full peer list.
func (b *MapResponseBuilder) WithPeers() *MapResponseBuilder {
	peers := b.mapper.state.ListPeers(b.nodeID)
	if len(peers) == 0 {
		b.resp.Peers = []*tailcfg.Node{}
		return b
	}

	tailPeers := make([]*tailcfg.Node, 0, len(peers))
	for _, peer := range peers {
		tn, err := peer.TailNode(b.capVer, func(id types.NodeID) []netip.Prefix {
			return b.mapper.state.PrimaryRoutesForNode(id)
		}, b.mapper.cfg)
		if err != nil {
			continue
		}
		tailPeers = append(tailPeers, tn)
	}

	sort.SliceStable(tailPeers, func(x, y int) bool {
		return tailPeers[x].ID < tailPeers[y].ID
	})

	b.resp.Peers = tailPeers
	b.resp.UserProfiles = generateUserProfilesFromPeers(peers)
	return b
}

// WithPeerChanges adds changed peers.
func (b *MapResponseBuilder) WithPeerChanges(peers []types.NodeView) *MapResponseBuilder {
	tailPeers := make([]*tailcfg.Node, 0, len(peers))
	for _, peer := range peers {
		tn, err := peer.TailNode(b.capVer, func(id types.NodeID) []netip.Prefix {
			return b.mapper.state.PrimaryRoutesForNode(id)
		}, b.mapper.cfg)
		if err != nil {
			continue
		}
		tailPeers = append(tailPeers, tn)
	}

	sort.SliceStable(tailPeers, func(x, y int) bool {
		return tailPeers[x].ID < tailPeers[y].ID
	})

	b.resp.PeersChanged = tailPeers
	return b
}

// WithPeersRemoved adds removed peer IDs.
func (b *MapResponseBuilder) WithPeersRemoved(removedIDs ...types.NodeID) *MapResponseBuilder {
	tailscaleIDs := make([]tailcfg.NodeID, 0, len(removedIDs))
	for _, id := range removedIDs {
		tailscaleIDs = append(tailscaleIDs, id.NodeID())
	}

	b.resp.PeersRemoved = tailscaleIDs
	return b
}

// WithUserProfiles adds user profiles for the node and its peers.
func (b *MapResponseBuilder) WithUserProfiles(peers []types.NodeView) *MapResponseBuilder {
	node, ok := b.mapper.state.GetNodeByID(b.nodeID)
	if !ok {
		return b
	}

	b.resp.UserProfiles = generateUserProfiles(node, peers)
	return b
}

// WithDNSConfig adds DNS configuration if available.
func (b *MapResponseBuilder) WithDNSConfig() *MapResponseBuilder {
	if b.mapper.cfg == nil {
		return b
	}

	node, ok := b.mapper.state.GetNodeByID(b.nodeID)
	if !ok {
		return b
	}

	dnsConfig := b.generateDNSConfig(node)
	if dnsConfig != nil {
		b.resp.DNSConfig = dnsConfig
	}
	return b
}

// generateDNSConfig creates DNS configuration for a node.
func (b *MapResponseBuilder) generateDNSConfig(node types.NodeView) *tailcfg.DNSConfig {
	if b.mapper.cfg == nil {
		return nil
	}

	// Basic DNS config
	dnsConfig := &tailcfg.DNSConfig{
		Proxied: true,
		Resolvers: []*dnstype.Resolver{
			{Addr: "https://dns.google/dns-query"},
		},
	}

	// Add NextDNS metadata if configured
	b.addNextDNSMetadata(dnsConfig.Resolvers, node)

	return dnsConfig
}

// addNextDNSMetadata adds device metadata to NextDNS DoH resolvers.
func (b *MapResponseBuilder) addNextDNSMetadata(resolvers []*dnstype.Resolver, node types.NodeView) {
	for _, resolver := range resolvers {
		if strings.HasPrefix(resolver.Addr, nextDNSDoHPrefix) {
			attrs := url.Values{
				"device_name": []string{node.Hostname()},
			}
			if len(node.IPs()) > 0 {
				attrs.Add("device_ip", node.IPs()[0].String())
			}
			resolver.Addr = fmt.Sprintf("%s?%s", resolver.Addr, attrs.Encode())
		}
	}
}

// Build finalizes the response and returns marshaled bytes.
func (b *MapResponseBuilder) Build() (*tailcfg.MapResponse, error) {
	if len(b.errs) > 0 {
		return nil, multierr.New(b.errs...)
	}
	return b.resp, nil
}

// generateUserProfiles creates user profiles for MapResponse.
func generateUserProfiles(node types.NodeView, peers []types.NodeView) []tailcfg.UserProfile {
	userMap := make(map[types.UserID]types.UserView)
	ids := make([]types.UserID, 0)

	// Add node's owner
	if node.Valid() {
		if owner := node.Owner(); owner.Valid() {
			userID := owner.ID()
			if _, exists := userMap[userID]; !exists {
				userMap[userID] = owner
				ids = append(ids, userID)
			}
		}
	}

	// Add peers' owners
	for _, peer := range peers {
		if owner := peer.Owner(); owner.Valid() {
			userID := owner.ID()
			if _, exists := userMap[userID]; !exists {
				userMap[userID] = owner
				ids = append(ids, userID)
			}
		}
	}

	slices.Sort(ids)
	ids = slices.Compact(ids)

	profiles := make([]tailcfg.UserProfile, 0, len(ids))
	for _, id := range ids {
		if u, ok := userMap[id]; ok {
			profiles = append(profiles, u.TailscaleUserProfile())
		}
	}

	return profiles
}

// generateUserProfilesFromPeers creates user profiles from peers only.
func generateUserProfilesFromPeers(peers []types.NodeView) []tailcfg.UserProfile {
	userMap := make(map[types.UserID]types.UserView)
	ids := make([]types.UserID, 0)

	for _, peer := range peers {
		if owner := peer.Owner(); owner.Valid() {
			userID := owner.ID()
			if _, exists := userMap[userID]; !exists {
				userMap[userID] = owner
				ids = append(ids, userID)
			}
		}
	}

	slices.Sort(ids)
	ids = slices.Compact(ids)

	profiles := make([]tailcfg.UserProfile, 0, len(ids))
	for _, id := range ids {
		if u, ok := userMap[id]; ok {
			profiles = append(profiles, u.TailscaleUserProfile())
		}
	}

	return profiles
}
