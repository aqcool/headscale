package state

import (
	"net/netip"

	"github.com/juanfont/headscale-v2/internal/ip"
	"github.com/juanfont/headscale-v2/internal/policy"
	"github.com/juanfont/headscale-v2/internal/types"
	"tailscale.com/tailcfg"
)

type NotifierFunc func(change types.Change)

func (s *State) SetNotifier(notifier NotifierFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notifier = notifier
}

func (s *State) SetIPAllocator(alloc *ip.IPAllocator) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ipAlloc = alloc
}

func (s *State) SetPolicyManager(pm policy.PolicyManager) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.polMan = pm
}

func (s *State) SetUsers(users []types.User) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.polMan != nil {
		s.polMan.SetUsers(users)
	}
}

func (s *State) notify(change types.Change) {
	if s.notifier != nil {
		s.notifier(change)
	}
}

func (s *State) BuildMapResponse(node types.NodeView, change types.Change) *tailcfg.MapResponse {
	if !node.Valid() {
		return nil
	}

	resp := &tailcfg.MapResponse{}

	routeFunc := func(id types.NodeID) []netip.Prefix {
		return s.GetNodePrimaryRoutes(id)
	}

	if change.SendAllPeers {
		peers := s.ListPeers(node.ID())
		peerViews := make([]*tailcfg.Node, len(peers))
		for i, peer := range peers {
			tNode, err := peer.TailNode(tailcfg.CapabilityVersion(0), routeFunc, s.cfg)
			if err != nil {
				continue
			}
			peerViews[i] = tNode
		}
		resp.Peers = peerViews
	}

	if change.IncludeSelf || change.TargetNode == node.ID() {
		tNode, err := node.TailNode(tailcfg.CapabilityVersion(0), routeFunc, s.cfg)
		if err == nil {
			resp.Node = tNode
		}
	}

	resp.DERPMap = s.DERPMap()

	if s.polMan != nil {
		rules, _ := s.Filter()
		resp.PacketFilter = rules
	}

	return resp
}