package types

import (
	"fmt"
	"slices"
	"time"

	"tailscale.com/tailcfg"
)

// Change declares what should be included in a MapResponse.
// The mapper uses this to build the response without guessing.
type Change struct {
	// Reason is a human-readable description for logging/debugging.
	Reason string

	// TargetNode, if set, means this response should only be sent to this node.
	TargetNode NodeID

	// OriginNode is the node that triggered this change.
	// Used for self-update detection and filtering.
	OriginNode NodeID

	// Content flags - what to include in the MapResponse.
	IncludeSelf    bool
	IncludeDERPMap bool
	IncludeDNS     bool
	IncludeDomain  bool
	IncludePolicy  bool // PacketFilters and SSHPolicy - always sent together

	// Peer changes.
	PeersChanged []NodeID
	PeersRemoved []NodeID
	PeerPatches  []*tailcfg.PeerChange
	SendAllPeers bool

	// RequiresRuntimePeerComputation indicates that peer visibility
	// must be computed at runtime per-node. Used for policy changes
	// where each node may have different peer visibility.
	RequiresRuntimePeerComputation bool

	// PingRequest, if non-nil, is a ping request to send to the node.
	// Used by the debug ping endpoint to verify node connectivity.
	// PingRequest is always targeted to a specific node via TargetNode.
	PingRequest *tailcfg.PingRequest
}

// IsEmpty returns true if this change has no effect.
func (r Change) IsEmpty() bool {
	if r.IncludeSelf || r.IncludeDERPMap || r.IncludeDNS ||
		r.IncludeDomain || r.IncludePolicy || r.SendAllPeers {
		return false
	}

	if r.RequiresRuntimePeerComputation {
		return false
	}

	if r.PingRequest != nil {
		return false
	}

	return len(r.PeersChanged) == 0 &&
		len(r.PeersRemoved) == 0 &&
		len(r.PeerPatches) == 0
}

// Merge combines two changes into one.
func (r Change) Merge(other Change) Change {
	merged := r

	merged.IncludeSelf = r.IncludeSelf || other.IncludeSelf
	merged.IncludeDERPMap = r.IncludeDERPMap || other.IncludeDERPMap
	merged.IncludeDNS = r.IncludeDNS || other.IncludeDNS
	merged.IncludeDomain = r.IncludeDomain || other.IncludeDomain
	merged.IncludePolicy = r.IncludePolicy || other.IncludePolicy
	merged.SendAllPeers = r.SendAllPeers || other.SendAllPeers
	merged.RequiresRuntimePeerComputation = r.RequiresRuntimePeerComputation || other.RequiresRuntimePeerComputation

	merged.PeersChanged = uniqueNodeIDs(slices.Concat(r.PeersChanged, other.PeersChanged))
	merged.PeersRemoved = uniqueNodeIDs(slices.Concat(r.PeersRemoved, other.PeersRemoved))
	merged.PeerPatches = slices.Concat(r.PeerPatches, other.PeerPatches)

	if merged.OriginNode == 0 {
		merged.OriginNode = other.OriginNode
	}

	if merged.TargetNode != 0 && other.TargetNode != 0 && merged.TargetNode != other.TargetNode {
		panic(fmt.Sprintf(
			"cannot merge changes with different TargetNode: %d != %d",
			merged.TargetNode, other.TargetNode,
		))
	}

	if merged.TargetNode == 0 {
		merged.TargetNode = other.TargetNode
	}

	if merged.PingRequest == nil {
		merged.PingRequest = other.PingRequest
	}

	if r.Reason != "" && other.Reason != "" && r.Reason != other.Reason {
		merged.Reason = r.Reason + "; " + other.Reason
	} else if other.Reason != "" {
		merged.Reason = other.Reason
	}

	return merged
}

// IsSelfOnly returns true if this response should only be sent to TargetNode.
func (r Change) IsSelfOnly() bool {
	if r.TargetNode == 0 || !r.IncludeSelf {
		return false
	}

	if r.SendAllPeers || len(r.PeersChanged) > 0 || len(r.PeersRemoved) > 0 || len(r.PeerPatches) > 0 {
		return false
	}

	return true
}

// IsTargetedToNode returns true if this response should only be sent to TargetNode.
func (r Change) IsTargetedToNode() bool {
	return r.TargetNode != 0
}

// IsFull reports whether this is a full update response.
func (r Change) IsFull() bool {
	return r.SendAllPeers && r.IncludeSelf && r.IncludeDERPMap &&
		r.IncludeDNS && r.IncludeDomain && r.IncludePolicy
}

// Type returns a categorized type string for metrics.
func (r Change) Type() string {
	if r.IsFull() {
		return "full"
	}

	if r.IsSelfOnly() {
		return "self"
	}

	if r.RequiresRuntimePeerComputation {
		return "policy"
	}

	if len(r.PeerPatches) > 0 && len(r.PeersChanged) == 0 && len(r.PeersRemoved) == 0 && !r.SendAllPeers {
		return "patch"
	}

	if len(r.PeersChanged) > 0 || len(r.PeersRemoved) > 0 || r.SendAllPeers {
		return "peers"
	}

	if r.IncludeDERPMap || r.IncludeDNS || r.IncludeDomain || r.IncludePolicy {
		return "config"
	}

	if r.PingRequest != nil {
		return "ping"
	}

	return "unknown"
}

// ShouldSendToNode determines if this response should be sent to nodeID.
func (r Change) ShouldSendToNode(nodeID NodeID) bool {
	if r.TargetNode != 0 {
		return r.TargetNode == nodeID
	}

	return true
}

func uniqueNodeIDs(ids []NodeID) []NodeID {
	if len(ids) == 0 {
		return nil
	}

	slices.Sort(ids)
	return slices.Compact(ids)
}

// Constructor functions

// FullUpdate returns a full MapResponse.
func FullUpdate() Change {
	return Change{
		Reason:         "full update",
		IncludeSelf:    true,
		IncludeDERPMap: true,
		IncludeDNS:     true,
		IncludeDomain:  true,
		IncludePolicy:  true,
		SendAllPeers:   true,
	}
}

// FullSelf returns a full update targeted at a specific node.
func FullSelf(nodeID NodeID) Change {
	return Change{
		Reason:         "full self update",
		TargetNode:     nodeID,
		IncludeSelf:    true,
		IncludeDERPMap:  true,
		IncludeDNS:     true,
		IncludeDomain:  true,
		IncludePolicy:  true,
		SendAllPeers:   true,
	}
}

// SelfUpdate returns a self-only update for a specific node.
func SelfUpdate(nodeID NodeID) Change {
	return Change{
		Reason:      "self update",
		TargetNode:  nodeID,
		IncludeSelf: true,
	}
}

// PolicyOnly returns a policy-only update.
func PolicyOnly() Change {
	return Change{
		Reason:        "policy update",
		IncludePolicy: true,
	}
}

// PolicyAndPeers returns a policy update with peer changes.
func PolicyAndPeers(changedPeers ...NodeID) Change {
	return Change{
		Reason:        "policy and peers update",
		IncludePolicy: true,
		PeersChanged:  changedPeers,
	}
}

// VisibilityChange returns a change for peer visibility updates.
func VisibilityChange(reason string, added, removed []NodeID) Change {
	return Change{
		Reason:        reason,
		IncludePolicy: true,
		PeersChanged:  added,
		PeersRemoved:  removed,
	}
}

// PeersChanged returns a change for peer additions/updates.
func PeersChanged(reason string, peerIDs ...NodeID) Change {
	return Change{
		Reason:       reason,
		PeersChanged: peerIDs,
	}
}

// PeersRemoved returns a change for peer removals.
func PeersRemoved(peerIDs ...NodeID) Change {
	return Change{
		Reason:       "peers removed",
		PeersRemoved: peerIDs,
	}
}

// PeerPatched returns a change for peer patches.
func PeerPatched(reason string, patches ...*tailcfg.PeerChange) Change {
	return Change{
		Reason:      reason,
		PeerPatches: patches,
	}
}

// DERPMap returns a change for DERP map updates.
func DERPMap() Change {
	return Change{
		Reason:         "DERP map update",
		IncludeDERPMap: true,
	}
}

// PolicyChange creates a response for policy changes.
func PolicyChange() Change {
	return Change{
		Reason:                         "policy change",
		IncludePolicy:                  true,
		RequiresRuntimePeerComputation: true,
	}
}

// DNSConfigChange creates a response for DNS configuration updates.
func DNSConfigChange() Change {
	return Change{
		Reason:     "DNS config update",
		IncludeDNS: true,
	}
}

// NodeOnline creates a patch response for a node coming online.
func NodeOnline(nodeID NodeID) Change {
	return Change{
		Reason: "node online",
		PeerPatches: []*tailcfg.PeerChange{
			{
				NodeID: tailcfg.NodeID(nodeID),
				Online: new(bool),
			},
		},
	}
}

// NodeOffline creates a patch response for a node going offline.
func NodeOffline(nodeID NodeID) Change {
	return Change{
		Reason: "node offline",
		PeerPatches: []*tailcfg.PeerChange{
			{
				NodeID: tailcfg.NodeID(nodeID),
				Online: new(bool),
			},
		},
	}
}

// KeyExpiry creates a patch response for a node's key expiry change.
func KeyExpiry(nodeID NodeID, expiry *time.Time) Change {
	return Change{
		Reason: "key expiry",
		PeerPatches: []*tailcfg.PeerChange{
			{
				NodeID:    tailcfg.NodeID(nodeID),
				KeyExpiry: expiry,
			},
		},
	}
}

// NodeAdded returns a Change for when a node is added or updated.
func NodeAdded(id NodeID) Change {
	c := PeersChanged("node added", id)
	c.OriginNode = id
	return c
}

// NodeRemoved returns a Change for when a node is removed.
func NodeRemoved(id NodeID) Change {
	return PeersRemoved(id)
}

// NodeOnlineFor returns a Change for when a node comes online.
func NodeOnlineFor(node NodeView) Change {
	if node.IsSubnetRouter() {
		c := FullUpdate()
		c.Reason = "subnet router online"
		return c
	}
	return NodeOnline(node.ID())
}

// NodeOfflineFor returns a Change for when a node goes offline.
func NodeOfflineFor(node NodeView) Change {
	if node.IsSubnetRouter() {
		c := FullUpdate()
		c.Reason = "subnet router offline"
		return c
	}
	return NodeOffline(node.ID())
}

// KeyExpiryFor returns a Change for when a node's key expiry changes.
func KeyExpiryFor(id NodeID, expiry time.Time) Change {
	c := KeyExpiry(id, &expiry)
	c.OriginNode = id
	return c
}

// EndpointOrDERPUpdate returns a Change for when a node's endpoints or DERP region changes.
func EndpointOrDERPUpdate(id NodeID, patch *tailcfg.PeerChange) Change {
	c := PeerPatched("endpoint/DERP update", patch)
	c.OriginNode = id
	return c
}

// UserAdded returns a Change for when a user is added or updated.
func UserAdded() Change {
	c := FullUpdate()
	c.Reason = "user added"
	return c
}

// UserRemoved returns a Change for when a user is removed.
func UserRemoved() Change {
	c := FullUpdate()
	c.Reason = "user removed"
	return c
}

// PingNode creates a Change that sends a PingRequest to a specific node.
func PingNode(nodeID NodeID, pr *tailcfg.PingRequest) Change {
	return Change{
		Reason:      "ping node",
		TargetNode:  nodeID,
		PingRequest: pr,
	}
}

// ExtraRecords returns a Change for when DNS extra records change.
func ExtraRecords() Change {
	c := DNSConfigChange()
	c.Reason = "extra records update"
	return c
}

// HasFull returns true if any response in the slice is a full update.
func HasFull(rs []Change) bool {
	for _, r := range rs {
		if r.IsFull() {
			return true
		}
	}
	return false
}

// SplitTargetedAndBroadcast separates responses into targeted and broadcast.
func SplitTargetedAndBroadcast(rs []Change) ([]Change, []Change) {
	var broadcast, targeted []Change

	for _, r := range rs {
		if r.IsTargetedToNode() {
			targeted = append(targeted, r)
		} else {
			broadcast = append(broadcast, r)
		}
	}

	return broadcast, targeted
}

// FilterForNode returns responses that should be sent to the given node.
func FilterForNode(nodeID NodeID, rs []Change) []Change {
	var result []Change

	for _, r := range rs {
		if r.ShouldSendToNode(nodeID) {
			result = append(result, r)
		}
	}

	return result
}

// MergeAll combines all changes into a single change.
func MergeAll(rs []Change) Change {
	if len(rs) == 0 {
		return Change{}
	}

	merged := rs[0]
	for i := 1; i < len(rs); i++ {
		merged = merged.Merge(rs[i])
	}

	return merged
}
