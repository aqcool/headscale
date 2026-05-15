package integration

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"github.com/juanfont/headscale-v2/internal/ip"
	"github.com/juanfont/headscale-v2/internal/mapper"
	"github.com/juanfont/headscale-v2/internal/state"
	"github.com/juanfont/headscale-v2/internal/types"
	"tailscale.com/types/key"
)

// TestSmoke tests basic system functionality
func TestSmoke(t *testing.T) {
	// Create IP allocator
	v4Prefix := netip.MustParsePrefix("100.64.0.0/10")
	v6Prefix := netip.MustParsePrefix("fd7a:115c:a1e0::/48")
	ipAlloc := ip.NewIPAllocator(&v4Prefix, &v6Prefix, types.IPAllocationStrategySequential)

	// Create state
	stateCfg := &state.StateConfig{
		IPAlloc:       ipAlloc,
		BatchSize:     100,
		BatchTimeout:  500 * time.Millisecond,
		PeersFunc:     func(nodes []types.NodeView) map[types.NodeID][]types.NodeView { return nil },
	}
	s := state.NewState(stateCfg, nil)

	// Create mapper
	m := mapper.NewMapper(s, nil, nil)

	t.Run("state_operations", func(t *testing.T) {
		// Create keys
		mk := key.NewMachine()
		mkP := mk.Public()
		nk := key.NewNode()
		nkP := nk.Public()

		// Test node creation
		node := &types.Node{
			Hostname:   "test-node",
			GivenName:  "test-node",
			MachineKey: mkP,
			NodeKey:    nkP,
		}

		ctx := context.Background()
		nodeView, err := s.AddNode(ctx, node)
		if err != nil {
			t.Fatalf("failed to add node: %v", err)
		}

		if !nodeView.Valid() {
			t.Fatal("expected valid node view")
		}

		if nodeView.Hostname() != "test-node" {
			t.Fatalf("expected hostname 'test-node', got %s", nodeView.Hostname())
		}

		// Test node retrieval
		retrieved, ok := s.GetNodeByID(nodeView.ID())
		if !ok {
			t.Fatal("failed to retrieve node")
		}

		if retrieved.ID() != nodeView.ID() {
			t.Fatal("node ID mismatch")
		}
	})

	t.Run("mapper_operations", func(t *testing.T) {
		// Test mapper creation
		if m == nil {
			t.Fatal("expected non-nil mapper")
		}
	})

	t.Run("node_store_operations", func(t *testing.T) {
		// Test list nodes
		nodes := s.ListNodes()
		if len(nodes) == 0 {
			t.Fatal("expected at least one node")
		}
	})
}

// TestIPAllocator tests IP allocation
func TestIPAllocator(t *testing.T) {
	v4Prefix := netip.MustParsePrefix("100.64.0.0/10")
	v6Prefix := netip.MustParsePrefix("fd7a:115c:a1e0::/48")
	ipAlloc := ip.NewIPAllocator(&v4Prefix, &v6Prefix, types.IPAllocationStrategySequential)

	// Allocate multiple IPs
	for i := 0; i < 10; i++ {
		v4, v6, err := ipAlloc.Next()
		if err != nil {
			t.Fatalf("failed to allocate IP: %v", err)
		}

		if !v4.IsValid() {
			t.Fatal("expected valid IPv4")
		}

		if !v6.IsValid() {
			t.Fatal("expected valid IPv6")
		}

		t.Logf("Allocated: %s, %s", v4, v6)
	}
}

// TestChangeTypes tests change type operations
func TestChangeTypes(t *testing.T) {
	// Test NodeAdded
	c := types.NodeAdded(1)
	if c.OriginNode != 1 {
		t.Fatal("expected origin node 1")
	}

	// Test PolicyChange
	c = types.PolicyChange()
	if !c.RequiresRuntimePeerComputation {
		t.Fatal("expected RequiresRuntimePeerComputation")
	}

	// Test Merge preserves fields
	c1 := types.NodeAdded(1)
	c2 := types.PolicyChange()
	merged := c1.Merge(c2)
	if merged.OriginNode != 1 {
		t.Fatal("expected merged change to have origin node 1")
	}
	if !merged.RequiresRuntimePeerComputation {
		t.Fatal("expected merged change to have RequiresRuntimePeerComputation")
	}
}