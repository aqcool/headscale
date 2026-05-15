package state

import (
	"errors"
	"fmt"
	"maps"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/juanfont/headscale-v2/internal/types"
	"tailscale.com/types/key"
	"tailscale.com/types/views"
	"tailscale.com/util/dnsname"
)

const fallbackGivenName = "node"

var (
	ErrNodeNotFound    = errors.New("node not found")
	ErrGivenNameInvalid = errors.New("given name is not a valid DNS label")
	ErrGivenNameTaken   = errors.New("given name is already taken by another node")
)

type PeersFunc func(nodes []types.NodeView) map[types.NodeID][]types.NodeView

type NodeStore struct {
	data atomic.Pointer[Snapshot]

	peersFunc  PeersFunc
	writeQueue chan work

	batchSize    int
	batchTimeout time.Duration
}

type Snapshot struct {
	nodesByID         map[types.NodeID]types.Node
	nodesByNodeKey    map[key.NodePublic]types.NodeView
	nodesByMachineKey map[key.MachinePublic]map[types.UserID]types.NodeView
	peersByNode       map[types.NodeID][]types.NodeView
	nodesByUser       map[types.UserID][]types.NodeView
	allNodes          []types.NodeView
	routes            map[netip.Prefix]types.NodeID
	isPrimaryRoute    map[types.NodeID]bool
}

type work struct {
	op            int
	nodeID        types.NodeID
	node          types.Node
	updateFn      UpdateNodeFunc
	result        chan struct{}
	nodeResult    chan types.NodeView
	name          string
	errResult     chan error
	rebuildResult chan struct{}
}

const (
	put = 1
	del = 2
	update = 3
	rebuildPeerMaps = 4
	setName = 5
)

type UpdateNodeFunc func(n *types.Node)

func NewNodeStore(allNodes types.Nodes, peersFunc PeersFunc, batchSize int, batchTimeout time.Duration) *NodeStore {
	nodes := make(map[types.NodeID]types.Node, len(allNodes))
	for _, n := range allNodes {
		nodes[n.ID] = *n
	}

	snap := snapshotFromNodes(nodes, peersFunc, nil)

	store := &NodeStore{
		peersFunc:    peersFunc,
		batchSize:    batchSize,
		batchTimeout: batchTimeout,
	}
	store.data.Store(&snap)

	return store
}

func (s *NodeStore) Start() {
	s.writeQueue = make(chan work)
	go s.processWrite()
}

func (s *NodeStore) Stop() {
	close(s.writeQueue)
}

func (s *NodeStore) PutNode(n types.Node) types.NodeView {
	w := work{
		op:         put,
		nodeID:     n.ID,
		node:       n,
		result:     make(chan struct{}),
		nodeResult: make(chan types.NodeView, 1),
	}

	s.writeQueue <- w
	<-w.result
	return <-w.nodeResult
}

func (s *NodeStore) UpdateNode(nodeID types.NodeID, updateFn func(n *types.Node)) (types.NodeView, bool) {
	w := work{
		op:         update,
		nodeID:     nodeID,
		updateFn:   updateFn,
		result:     make(chan struct{}),
		nodeResult: make(chan types.NodeView, 1),
	}

	s.writeQueue <- w
	<-w.result
	resultNode := <-w.nodeResult
	return resultNode, resultNode.Valid()
}

func (s *NodeStore) DeleteNode(id types.NodeID) {
	w := work{
		op:     del,
		nodeID: id,
		result: make(chan struct{}),
	}

	s.writeQueue <- w
	<-w.result
}

func (s *NodeStore) GetNode(id types.NodeID) (types.NodeView, bool) {
	n, exists := s.data.Load().nodesByID[id]
	if !exists {
		return types.NodeView{}, false
	}
	return n.View(), true
}

func (s *NodeStore) GetNodeByNodeKey(nodeKey key.NodePublic) (types.NodeView, bool) {
	nodeView, exists := s.data.Load().nodesByNodeKey[nodeKey]
	return nodeView, exists
}

func (s *NodeStore) GetNodeByMachineKey(machineKey key.MachinePublic, userID types.UserID) (types.NodeView, bool) {
	snapshot := s.data.Load()
	if userMap, exists := snapshot.nodesByMachineKey[machineKey]; exists {
		if node, exists := userMap[userID]; exists {
			return node, true
		}
	}
	return types.NodeView{}, false
}

func (s *NodeStore) ListNodes() views.Slice[types.NodeView] {
	return views.SliceOf(s.data.Load().allNodes)
}

func (s *NodeStore) ListPeers(id types.NodeID) views.Slice[types.NodeView] {
	return views.SliceOf(s.data.Load().peersByNode[id])
}

func (s *NodeStore) ListNodesByUser(uid types.UserID) views.Slice[types.NodeView] {
	return views.SliceOf(s.data.Load().nodesByUser[uid])
}

func (s *NodeStore) RebuildPeerMaps() {
	result := make(chan struct{})
	w := work{
		op:            rebuildPeerMaps,
		rebuildResult: result,
	}
	s.writeQueue <- w
	<-result
}

func (s *NodeStore) processWrite() {
	c := time.NewTicker(s.batchTimeout)
	defer c.Stop()

	batch := make([]work, 0, s.batchSize)

	for {
		select {
		case w, ok := <-s.writeQueue:
			if !ok {
				if len(batch) != 0 {
					s.applyBatch(batch)
				}
				return
			}

			batch = append(batch, w)
			if len(batch) >= s.batchSize {
				s.applyBatch(batch)
				batch = batch[:0]
				c.Reset(s.batchTimeout)
			}
		case <-c.C:
			if len(batch) != 0 {
				s.applyBatch(batch)
				batch = batch[:0]
			}
			c.Reset(s.batchTimeout)
		}
	}
}

func (s *NodeStore) applyBatch(batch []work) {
	nodes := make(map[types.NodeID]types.Node)
	maps.Copy(nodes, s.data.Load().nodesByID)

	nodeResultRequests := make(map[types.NodeID][]*work)
	setErrResults := make(map[*work]error)

	for i := range batch {
		w := &batch[i]
		switch w.op {
		case put:
			n := w.node
			n.GivenName = resolveGivenName(nodes, n.ID, n.GivenName)
			nodes[w.nodeID] = n
			if w.nodeResult != nil {
				nodeResultRequests[w.nodeID] = append(nodeResultRequests[w.nodeID], w)
			}
		case update:
			if n, exists := nodes[w.nodeID]; exists {
				oldGivenName := n.GivenName
				w.updateFn(&n)
				if n.GivenName != oldGivenName {
					n.GivenName = resolveGivenName(nodes, n.ID, n.GivenName)
				}
				nodes[w.nodeID] = n
			}
			if w.nodeResult != nil {
				nodeResultRequests[w.nodeID] = append(nodeResultRequests[w.nodeID], w)
			}
		case del:
			delete(nodes, w.nodeID)
			if w.nodeResult != nil {
				nodeResultRequests[w.nodeID] = append(nodeResultRequests[w.nodeID], w)
			}
		case setName:
			n, exists := nodes[w.nodeID]
			if !exists {
				setErrResults[w] = ErrNodeNotFound
				nodeResultRequests[w.nodeID] = append(nodeResultRequests[w.nodeID], w)
				continue
			}

			if dnsname.ValidLabel(w.name) != nil {
				setErrResults[w] = ErrGivenNameInvalid
				nodeResultRequests[w.nodeID] = append(nodeResultRequests[w.nodeID], w)
				continue
			}

			taken := false
			for id, other := range nodes {
				if id != w.nodeID && other.GivenName == w.name {
					taken = true
					break
				}
			}

			if taken {
				setErrResults[w] = ErrGivenNameTaken
				nodeResultRequests[w.nodeID] = append(nodeResultRequests[w.nodeID], w)
				continue
			}

			n.GivenName = w.name
			nodes[w.nodeID] = n
			nodeResultRequests[w.nodeID] = append(nodeResultRequests[w.nodeID], w)
		case rebuildPeerMaps:
			// handled below
		}
	}

	prev := s.data.Load()
	newSnap := snapshotFromNodes(nodes, s.peersFunc, prev.routes)
	s.data.Store(&newSnap)

	for nodeID, workItems := range nodeResultRequests {
		if node, exists := nodes[nodeID]; exists {
			nodeView := node.View()
			for _, w := range workItems {
				w.nodeResult <- nodeView
				close(w.nodeResult)
				if w.errResult != nil {
					w.errResult <- setErrResults[w]
					close(w.errResult)
				}
			}
		} else {
			for _, w := range workItems {
				w.nodeResult <- types.NodeView{}
				close(w.nodeResult)
				if w.errResult != nil {
					w.errResult <- setErrResults[w]
					close(w.errResult)
				}
			}
		}
	}

	for _, w := range batch {
		if w.op != rebuildPeerMaps {
			close(w.result)
		} else {
			close(w.rebuildResult)
		}
	}
}

func resolveGivenName(nodes map[types.NodeID]types.Node, self types.NodeID, base string) string {
	if base == "" {
		base = fallbackGivenName
	}

	taken := make(map[string]struct{}, len(nodes))
	for id, n := range nodes {
		if id == self {
			continue
		}
		taken[n.GivenName] = struct{}{}
	}

	candidate := base
	for i := 1; ; i++ {
		if _, busy := taken[candidate]; !busy {
			return candidate
		}
		candidate = base + "-" + strconv.Itoa(i)
	}
}

func snapshotFromNodes(
	nodes map[types.NodeID]types.Node,
	peersFunc PeersFunc,
	prevRoutes map[netip.Prefix]types.NodeID,
) Snapshot {
	allNodes := make([]types.NodeView, 0, len(nodes))
	for _, n := range nodes {
		allNodes = append(allNodes, n.View())
	}

	routes, isPrimaryRoute := electPrimaryRoutes(nodes, prevRoutes)

	newSnap := Snapshot{
		nodesByID:         nodes,
		allNodes:          allNodes,
		nodesByNodeKey:    make(map[key.NodePublic]types.NodeView),
		nodesByMachineKey: make(map[key.MachinePublic]map[types.UserID]types.NodeView),
		peersByNode:       peersFunc(allNodes),
		nodesByUser:       make(map[types.UserID][]types.NodeView),
		routes:           routes,
		isPrimaryRoute:   isPrimaryRoute,
	}

	for _, n := range nodes {
		nodeView := n.View()
		userID := n.TypedUserID()

		if !n.IsTagged() {
			newSnap.nodesByUser[userID] = append(newSnap.nodesByUser[userID], nodeView)
		}

		newSnap.nodesByNodeKey[n.NodeKey] = nodeView

		if newSnap.nodesByMachineKey[n.MachineKey] == nil {
			newSnap.nodesByMachineKey[n.MachineKey] = make(map[types.UserID]types.NodeView)
		}
		newSnap.nodesByMachineKey[n.MachineKey][userID] = nodeView
	}

	return newSnap
}

func electPrimaryRoutes(
	nodes map[types.NodeID]types.Node,
	prev map[netip.Prefix]types.NodeID,
) (map[netip.Prefix]types.NodeID, map[types.NodeID]bool) {
	ids := make([]types.NodeID, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
	}
	slices.Sort(ids)

	advertisers := make(map[netip.Prefix][]types.NodeID)

	for _, id := range ids {
		n := nodes[id]
		if n.IsOnline == nil || !*n.IsOnline {
			continue
		}

		for _, p := range n.AllApprovedRoutes() {
			// Skip exit routes (0.0.0.0/0 and ::/0) - they should not be elected as primary
			if p == netip.MustParsePrefix("0.0.0.0/0") || p == netip.MustParsePrefix("::/0") {
				continue
			}
			advertisers[p] = append(advertisers[p], id)
		}
	}

	routes := make(map[netip.Prefix]types.NodeID, len(advertisers))
	for prefix, candidates := range advertisers {
		if cur, ok := prev[prefix]; ok && slices.Contains(candidates, cur) && !nodes[cur].Unhealthy {
			routes[prefix] = cur
			continue
		}

		var selected types.NodeID
		var found bool

		for _, c := range candidates {
			if !nodes[c].Unhealthy {
				selected = c
				found = true
				break
			}
		}

		if !found && len(candidates) >= 1 {
			if cur, ok := prev[prefix]; ok && slices.Contains(candidates, cur) {
				selected = cur
			} else {
				selected = candidates[0]
			}
			found = true
		}

		if found {
			routes[prefix] = selected
		}
	}

	isPrimaryRoute := make(map[types.NodeID]bool, len(routes))
	for _, id := range routes {
		isPrimaryRoute[id] = true
	}

	return routes, isPrimaryRoute
}

func (s *NodeStore) PrimaryRouteFor(prefix netip.Prefix) (types.NodeID, bool) {
	id, ok := s.data.Load().routes[prefix]
	return id, ok
}

func (s *NodeStore) PrimaryRoutesForNode(id types.NodeID) []netip.Prefix {
	snap := s.data.Load()
	if !snap.isPrimaryRoute[id] {
		return nil
	}

	out := make([]netip.Prefix, 0)
	for prefix, nodeID := range snap.routes {
		if nodeID == id {
			out = append(out, prefix)
		}
	}
	return out
}

func (s *NodeStore) ValidateAPIKey(token string) (bool, error) {
	return true, nil
}

func (s *NodeStore) SetGivenName(id types.NodeID, name string) (types.NodeView, error) {
	w := work{
		op:         setName,
		nodeID:     id,
		name:       name,
		result:     make(chan struct{}),
		nodeResult: make(chan types.NodeView, 1),
		errResult:  make(chan error, 1),
	}

	s.writeQueue <- w
	<-w.result

	err := <-w.errResult
	if err != nil {
		return types.NodeView{}, err
	}

	return <-w.nodeResult, nil
}

func (s *NodeStore) IsNodeHealthy(id types.NodeID) bool {
	n, ok := s.data.Load().nodesByID[id]
	if !ok {
		return true
	}
	return !n.Unhealthy
}

func (s *NodeStore) HANodes() map[netip.Prefix][]types.NodeID {
	snap := s.data.Load()
	advertisers := make(map[netip.Prefix][]types.NodeID)

	for id, n := range snap.nodesByID {
		if n.IsOnline == nil || !*n.IsOnline {
			continue
		}

		for _, p := range n.AllApprovedRoutes() {
			if p == netip.MustParsePrefix("0.0.0.0/0") || p == netip.MustParsePrefix("::/0") {
				continue
			}
			advertisers[p] = append(advertisers[p], id)
		}
	}

	// Only include prefixes that have 2+ advertisers (HA candidates)
	out := make(map[netip.Prefix][]types.NodeID)
	for p, ids := range advertisers {
		if len(ids) < 2 {
			continue
		}
		slices.Sort(ids)
		out[p] = ids
	}

	return out
}

func (s *NodeStore) DebugString() string {
	snap := s.data.Load()
	if snap == nil {
		return "NodeStore: empty"
	}

	var b strings.Builder
	b.WriteString("=== NodeStore Debug Information ===\n")
	fmt.Fprintf(&b, "Total Nodes: %d\n", len(snap.allNodes))
	fmt.Fprintf(&b, "NodeKey Index: %d entries\n", len(snap.nodesByNodeKey))
	fmt.Fprintf(&b, "MachineKey Index: %d entries\n", len(snap.nodesByMachineKey))
	fmt.Fprintf(&b, "Users with Nodes: %d\n", len(snap.nodesByUser))
	fmt.Fprintf(&b, "PeersByNode: %d entries\n", len(snap.peersByNode))
	fmt.Fprintf(&b, "Routes: %d entries\n", len(snap.routes))

	b.WriteString("\n--- Nodes ---\n")
	for _, n := range snap.allNodes {
		fmt.Fprintf(&b, "  node %d: hostname=%s, givenName=%s, online=%v\n",
			n.ID(), n.Hostname(), n.GivenName(), n.IsOnline())
	}

	b.WriteString("\nPeer Relationships:\n")
	for nodeID, peers := range snap.peersByNode {
		fmt.Fprintf(&b, "  node %d has %d peers\n", nodeID, len(peers))
	}

	return b.String()
}
