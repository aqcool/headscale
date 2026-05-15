package state

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/juanfont/headscale-v2/internal/ip"
	"github.com/juanfont/headscale-v2/internal/policy"
	"github.com/juanfont/headscale-v2/internal/policy/matcher"
	"github.com/juanfont/headscale-v2/internal/types"
	"github.com/juanfont/headscale-v2/internal/util"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
	"tailscale.com/types/views"
	"tailscale.com/util/dnsname"
)

var (
	ErrRegistrationExpired = errors.New("registration expired")
	ErrInvalidNodeView     = errors.New("invalid node view provided")
	ErrNodeNotInNodeStore  = errors.New("node no longer exists in NodeStore")
	ErrUserNotFound        = errors.New("user not found")
)

type State struct {
	mu sync.RWMutex

	nodeStore  *NodeStore
	repo       StateRepository
	ipAlloc    *ip.IPAllocator
	derpMap    atomic.Pointer[tailcfg.DERPMap]
	authCache  sync.Map
	privateKey key.MachinePrivate
	polMan     policy.PolicyManager
	cfg        *types.Config
	notifier   NotifierFunc

	logger *log.Helper

	sshCheckMu   sync.RWMutex
	sshCheckAuth map[sshCheckPair]time.Time
}

type sshCheckPair struct {
	Src types.NodeID
	Dst types.NodeID
}

type StateConfig struct {
	Nodes         types.Nodes
	PeersFunc     PeersFunc
	BatchSize     int
	BatchTimeout  time.Duration
	IPAlloc       *ip.IPAllocator
	Repo          StateRepository
	Config        *types.Config
}

func NewState(cfg *StateConfig, logger log.Logger) *State {
	s := &State{
		ipAlloc:      cfg.IPAlloc,
		repo:         cfg.Repo,
		cfg:          cfg.Config,
		logger:       log.NewHelper(logger),
		sshCheckAuth: make(map[sshCheckPair]time.Time),
	}

	s.nodeStore = NewNodeStore(cfg.Nodes, cfg.PeersFunc, cfg.BatchSize, cfg.BatchTimeout)
	s.nodeStore.Start()

	return s
}

func (s *State) Stop() {
	s.nodeStore.Stop()
}

// persistNodeToDB saves the given node state to the database.
func (s *State) persistNodeToDB(node types.NodeView) (types.NodeView, types.Change, error) {
	if !node.Valid() {
		return types.NodeView{}, types.Change{}, ErrInvalidNodeView
	}

	_, exists := s.nodeStore.GetNode(node.ID())
	if !exists {
		s.logger.Warnf("Node no longer exists in NodeStore, skipping database persist")
		return types.NodeView{}, types.Change{}, fmt.Errorf("%w: %d", ErrNodeNotInNodeStore, node.ID())
	}

	nodePtr := node.AsStruct()

	err := s.repo.PersistNode(nodePtr, nodeUpdateColumns, []string{"Expiry"})
	if err != nil {
		return types.NodeView{}, types.Change{}, fmt.Errorf("saving node: %w", err)
	}

	c, err := s.updatePolicyManagerNodes()
	if err != nil {
		return nodePtr.View(), types.Change{}, fmt.Errorf("updating policy manager: %w", err)
	}

	if c.IsEmpty() {
		c = types.NodeAdded(node.ID())
	}

	return node, c, nil
}

// SaveNode updates NodeStore first, then persists to database.
func (s *State) SaveNode(node types.NodeView) (types.NodeView, types.Change, error) {
	nodePtr := node.AsStruct()
	resultNode := s.nodeStore.PutNode(*nodePtr)
	return s.persistNodeToDB(resultNode)
}

// DeleteNode permanently removes a node and cleans up associated resources.
func (s *State) DeleteNode(node types.NodeView) (types.Change, error) {
	s.nodeStore.DeleteNode(node.ID())

	err := s.repo.DeleteNode(node.AsStruct())
	if err != nil {
		return types.Change{}, err
	}

	if s.ipAlloc != nil {
		s.ipAlloc.FreeIPs(node.IPs())
	}

	c := types.NodeRemoved(node.ID())

	policyChange, err := s.updatePolicyManagerNodes()
	if err != nil {
		return types.Change{}, fmt.Errorf("updating policy manager: %w", err)
	}

	if !policyChange.IsEmpty() {
		c = c.Merge(policyChange)
	}

	return c, nil
}

// GetNodeByID retrieves a node by ID.
func (s *State) GetNodeByID(id types.NodeID) (types.NodeView, bool) {
	return s.nodeStore.GetNode(id)
}

// GetNodeByNodeKey retrieves a node by its NodeKey.
func (s *State) GetNodeByNodeKey(nodeKey key.NodePublic) (types.NodeView, bool) {
	return s.nodeStore.GetNodeByNodeKey(nodeKey)
}

// GetNodeByMachineKey retrieves a node by MachineKey and UserID.
func (s *State) GetNodeByMachineKey(machineKey key.MachinePublic, userID types.UserID) (types.NodeView, bool) {
	return s.nodeStore.GetNodeByMachineKey(machineKey, userID)
}

// ListNodes retrieves all nodes.
func (s *State) ListNodes() []types.NodeView {
	nodes := s.nodeStore.ListNodes()
	return nodes.AsSlice()
}

// ListPeers retrieves peers for a node.
func (s *State) ListPeers(nodeID types.NodeID) []types.NodeView {
	peers := s.nodeStore.ListPeers(nodeID)
	return peers.AsSlice()
}

// AddNode adds a new node with IP allocation.
func (s *State) AddNode(ctx context.Context, node *types.Node) (types.NodeView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if node.IPv4 == nil && node.IPv6 == nil && s.ipAlloc != nil {
		v4, v6, err := s.ipAlloc.Next()
		if err != nil {
			return types.NodeView{}, err
		}
		node.IPv4 = v4
		node.IPv6 = v6
	}

	return s.nodeStore.PutNode(*node), nil
}

// UpdateNode updates a node in NodeStore.
func (s *State) UpdateNode(nodeID types.NodeID, updateFn func(*types.Node)) (types.NodeView, error) {
	nodeView, ok := s.nodeStore.UpdateNode(nodeID, updateFn)
	if !ok {
		return types.NodeView{}, ErrNodeNotFound
	}
	return nodeView, nil
}

// UpdateNodeFromMapRequest updates node from MapRequest.
func (s *State) UpdateNodeFromMapRequest(nodeID types.NodeID, req tailcfg.MapRequest) (types.NodeView, error) {
	nodeView, ok := s.nodeStore.UpdateNode(nodeID, func(n *types.Node) {
		now := time.Now()
		n.LastSeen = &now
		if req.Hostinfo != nil {
			n.Hostinfo = req.Hostinfo
		}
		if len(req.Endpoints) > 0 {
			n.Endpoints = req.Endpoints
		}
		if req.NodeKey != (key.NodePublic{}) {
			n.NodeKey = req.NodeKey
		}
		if req.DiscoKey != (key.DiscoPublic{}) {
			n.DiscoKey = req.DiscoKey
		}
	})
	if !ok {
		return types.NodeView{}, ErrNodeNotFound
	}
	return nodeView, nil
}

// DERPMap returns the current DERP map.
func (s *State) DERPMap() *tailcfg.DERPMap {
	return s.derpMap.Load()
}

// SetDERPMap sets the DERP map.
func (s *State) SetDERPMap(derpMap *tailcfg.DERPMap) {
	s.derpMap.Store(derpMap)
}

// PrimaryRouteFor returns the primary route for a prefix.
func (s *State) PrimaryRouteFor(prefix netip.Prefix) (types.NodeID, bool) {
	return s.nodeStore.PrimaryRouteFor(prefix)
}

// PrimaryRoutesForNode returns primary routes for a node.
func (s *State) PrimaryRoutesForNode(id types.NodeID) []netip.Prefix {
	return s.nodeStore.PrimaryRoutesForNode(id)
}

// RebuildPeerMaps rebuilds peer maps.
func (s *State) RebuildPeerMaps() {
	s.nodeStore.RebuildPeerMaps()
}

// ValidateAPIKey validates an API key.
func (s *State) ValidateAPIKey(token string) (bool, error) {
	return s.nodeStore.ValidateAPIKey(token)
}

// ReloadPolicy reloads the policy.
func (s *State) ReloadPolicy() ([]types.Change, error) {
	s.RebuildPeerMaps()
	return []types.Change{{SendAllPeers: true}}, nil
}

// Connect marks a node connected.
func (s *State) Connect(id types.NodeID) ([]types.Change, uint64) {
	var epoch uint64
	_, ok := s.nodeStore.UpdateNode(id, func(n *types.Node) {
		n.SessionEpoch++
		epoch = n.SessionEpoch
		n.IsOnline = new(bool)
		*n.IsOnline = true
		n.Unhealthy = false
	})
	if !ok {
		return nil, 0
	}

	c := []types.Change{types.NodeOnlineFor(s.getNodeView(id))}
	s.logger.Infof("node %d connected", id)
	return c, epoch
}

// Disconnect marks the node offline.
func (s *State) Disconnect(id types.NodeID, epoch uint64) ([]types.Change, error) {
	var stale bool
	_, ok := s.nodeStore.UpdateNode(id, func(n *types.Node) {
		if n.SessionEpoch != epoch {
			stale = true
			return
		}
		now := time.Now()
		n.LastSeen = &now
		n.IsOnline = new(bool)
		*n.IsOnline = false
		n.Unhealthy = false
	})

	if !ok {
		return nil, ErrNodeNotFound
	}

	if stale {
		s.logger.Debugf("stale disconnect rejected for node %d", id)
		return nil, nil
	}

	s.logger.Infof("node %d disconnected", id)
	return []types.Change{types.NodeOfflineFor(s.getNodeView(id))}, nil
}

func (s *State) getNodeView(id types.NodeID) types.NodeView {
	node, _ := s.nodeStore.GetNode(id)
	return node
}

// HandleNodeFromPreAuthKey handles node registration with pre-auth key.
func (s *State) HandleNodeFromPreAuthKey(
	regReq tailcfg.RegisterRequest,
	machineKey key.MachinePublic,
) (types.NodeView, types.Change, error) {
	pak, err := s.GetPreAuthKey(regReq.Auth.AuthKey)
	if err != nil {
		return types.NodeView{}, types.Change{}, err
	}

	// Validate auth key
	if err := pak.Validate(); err != nil {
		return types.NodeView{}, types.Change{}, err
	}

	// Get hostname
	var hostname string
	if regReq.Hostinfo != nil {
		hostname = regReq.Hostinfo.Hostname
	}

	validHostinfo := regReq.Hostinfo
	if validHostinfo == nil {
		validHostinfo = &tailcfg.Hostinfo{}
	}
	validHostinfo.Hostname = hostname

	// Check for existing node
	var existingNode types.NodeView
	if pak.User != nil {
		existingNode, _ = s.nodeStore.GetNodeByMachineKey(machineKey, types.UserID(pak.User.ID))
	}

	var finalNode types.NodeView

	if existingNode.Valid() {
		// Update existing node
		updatedNode, ok := s.nodeStore.UpdateNode(existingNode.ID(), func(n *types.Node) {
			n.NodeKey = regReq.NodeKey
			n.Hostname = hostname
			n.Hostinfo = validHostinfo
			n.RegisterMethod = util.RegisterMethodAuthKey
			n.AuthKey = pak
			n.AuthKeyID = &pak.ID
			n.LastSeen = new(time.Now())

			if !n.IsTagged() {
				if !regReq.Expiry.IsZero() {
					n.Expiry = &regReq.Expiry
				}
			}
		})
		if !ok {
			return types.NodeView{}, types.Change{}, fmt.Errorf("%w: %d", ErrNodeNotInNodeStore, existingNode.ID())
		}

		// Persist to database
		err := s.repo.PersistNode(updatedNode.AsStruct(), nodeUpdateColumns, nil)
		if err != nil {
			return types.NodeView{}, types.Change{}, fmt.Errorf("saving node: %w", err)
		}

		// Mark key as used
		if !pak.Reusable && !pak.Used {
			_ = s.UsePreAuthKey(pak)
		}

		finalNode = updatedNode
	} else {
		// Create new node
		var pakUser types.User
		if pak.User != nil {
			pakUser = *pak.User
		}

		var reqExpiry *time.Time
		if !regReq.Expiry.IsZero() {
			reqExpiry = &regReq.Expiry
		}

		var err error
		finalNode, err = s.createAndSaveNewNode(newNodeParams{
			User:           pakUser,
			MachineKey:     machineKey,
			NodeKey:        regReq.NodeKey,
			Hostname:       hostname,
			Hostinfo:       validHostinfo,
			Expiry:         reqExpiry,
			RegisterMethod: util.RegisterMethodAuthKey,
			PreAuthKey:     pak,
		})
		if err != nil {
			return types.NodeView{}, types.Change{}, fmt.Errorf("creating node: %w", err)
		}
	}

	// Update policy managers
	nodesChange, err := s.updatePolicyManagerNodes()
	if err != nil {
		return finalNode, types.NodeAdded(finalNode.ID()), fmt.Errorf("updating policy manager: %w", err)
	}

	var c types.Change
	if !nodesChange.IsEmpty() {
		c = types.PolicyChange()
	} else {
		c = types.NodeAdded(finalNode.ID())
	}

	return finalNode, c, nil
}

// newNodeParams contains parameters for creating a new node.
type newNodeParams struct {
	User           types.User
	MachineKey     key.MachinePublic
	NodeKey        key.NodePublic
	DiscoKey       key.DiscoPublic
	Hostname       string
	Hostinfo       *tailcfg.Hostinfo
	Endpoints      []netip.AddrPort
	Expiry         *time.Time
	RegisterMethod string
	PreAuthKey     *types.PreAuthKey
}

// createAndSaveNewNode creates a new node, allocates IPs, saves to DB.
func (s *State) createAndSaveNewNode(params newNodeParams) (types.NodeView, error) {
	nodeToRegister := types.Node{
		Hostname:       params.Hostname,
		MachineKey:     params.MachineKey,
		NodeKey:        params.NodeKey,
		DiscoKey:       params.DiscoKey,
		Hostinfo:       params.Hostinfo,
		Endpoints:      params.Endpoints,
		LastSeen:       new(time.Now()),
		IsOnline:       new(bool),
		RegisterMethod: params.RegisterMethod,
		Expiry:         params.Expiry,
	}

	// Assign ownership based on PreAuthKey
	if params.PreAuthKey != nil {
		if params.PreAuthKey.IsTagged() {
			nodeToRegister.Tags = params.PreAuthKey.Tags
			nodeToRegister.Expiry = nil
		} else if params.PreAuthKey.User != nil {
			uid := uint(params.PreAuthKey.User.ID)
			nodeToRegister.UserID = &uid
			nodeToRegister.User = params.PreAuthKey.User
		}
		nodeToRegister.AuthKey = params.PreAuthKey
		nodeToRegister.AuthKeyID = &params.PreAuthKey.ID
	} else {
		uid := uint(params.User.ID); nodeToRegister.UserID = &uid
		
		nodeToRegister.User = &params.User
	}

	// Allocate IPs
	if s.ipAlloc != nil {
		ipv4, ipv6, err := s.ipAlloc.Next()
		if err != nil {
			return types.NodeView{}, fmt.Errorf("allocating IPs: %w", err)
		}
		nodeToRegister.IPv4 = ipv4
		nodeToRegister.IPv6 = ipv6
	}

	// Set GivenName
	if nodeToRegister.GivenName == "" {
		nodeToRegister.GivenName = dnsname.SanitizeHostname(nodeToRegister.Hostname)
	}

	// Save to database
	var savedNode *types.Node
	err := s.repo.SaveNode(&nodeToRegister)
	if err != nil {
		return types.NodeView{}, fmt.Errorf("saving node: %w", err)
	}

	if params.PreAuthKey != nil && !params.PreAuthKey.Reusable {
		if err := s.UsePreAuthKey(params.PreAuthKey); err != nil {
			s.logger.Warnf("failed to mark pre-auth key as used: %v", err)
		}
	}

	savedNode = &nodeToRegister
	if err != nil {
		return types.NodeView{}, err
	}

	// Add to NodeStore
	return s.nodeStore.PutNode(*savedNode), nil
}

// updatePolicyManagerNodes updates the policy manager with current nodes.
func (s *State) updatePolicyManagerNodes() (types.Change, error) {
	if s.polMan == nil {
		return types.Change{}, nil
	}

	nodes := s.nodeStore.ListNodes()
	changed, err := s.polMan.SetNodes(nodes)
	if err != nil {
		return types.Change{}, fmt.Errorf("updating policy manager nodes: %w", err)
	}

	if changed {
		s.nodeStore.RebuildPeerMaps()
		return types.PolicyChange(), nil
	}

	return types.Change{}, nil
}

// Policy Manager methods

func (s *State) SSHPolicy(node types.NodeView) (*tailcfg.SSHPolicy, error) {
	if s.polMan != nil {
		return s.polMan.SSHPolicy("", node)
	}
	return &tailcfg.SSHPolicy{Rules: []*tailcfg.SSHRule{}}, nil
}

func (s *State) FilterForNode(node types.NodeView) ([]tailcfg.FilterRule, error) {
	if s.polMan != nil {
		return s.polMan.FilterForNode(node)
	}
	return tailcfg.FilterAllowAll, nil
}

func (s *State) Filter() ([]tailcfg.FilterRule, []matcher.Match) {
	if s.polMan != nil {
		return s.polMan.Filter()
	}
	return tailcfg.FilterAllowAll, nil
}

func (s *State) MatchersForNode(node types.NodeView) ([]matcher.Match, error) {
	if s.polMan != nil {
		return s.polMan.MatchersForNode(node)
	}
	return nil, nil
}

func (s *State) SetPolicy(pol []byte, users []types.User, nodes views.Slice[types.NodeView]) error {
	var err error
	s.polMan, err = policy.NewPolicyManager(pol, users, nodes)
	return err
}

// User management methods

func (s *State) CreateUser(name string) (*types.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return &types.User{Name: name}, nil
}

func (s *State) GetUserByID(userID types.UserID) (*types.User, error) {
	if s.repo != nil {
		return s.repo.GetUserByID(userID)
	}
	return &types.User{ID: userID}, nil
}

func (s *State) GetUserByName(name string) (*types.User, error) {
	if s.repo != nil {
		return s.repo.GetUserByName(name)
	}
	return &types.User{Name: name}, nil
}

func (s *State) ListUsers() ([]types.User, error) {
	if s.repo != nil {
		return s.repo.ListUsers()
	}
	return []types.User{}, nil
}

func (s *State) DeleteUser(userID types.UserID) error {
	return nil
}

// PreAuthKey methods

func (s *State) GetPreAuthKey(id string) (*types.PreAuthKey, error) {
	if s.repo != nil {
		return s.repo.GetPreAuthKey(id)
	}
	return nil, errors.New("database not available")
}

func (s *State) UsePreAuthKey(pak *types.PreAuthKey) error {
	if s.repo != nil {
		return s.repo.UsePreAuthKey(pak)
	}
	return nil
}

func (s *State) CreatePreAuthKey(userID uint64, reusable bool, ephemeral bool, expiration *time.Time) (*types.PreAuthKeyNew, error) {
	key, err := util.GenerateRandomStringURLSafe(32)
	if err != nil {
		return nil, err
	}
	return &types.PreAuthKeyNew{
		Key:        "hskey-auth-" + key,
		Reusable:   reusable,
		Ephemeral:  ephemeral,
		Expiration: expiration,
	}, nil
}

// Health check methods

func (s *State) PingDB(ctx context.Context) error {
	if s.repo != nil {
		return s.repo.PingDB(ctx)
	}
	return nil
}

func (s *State) ExpireExpiredNodes(lastCheck time.Time) (time.Time, []types.Change, error) {
	now := time.Now()
	changes := []types.Change{}

	for _, node := range s.ListNodes() {
		if node.IsExpired() {
			changes = append(changes, types.Change{TargetNode: node.ID(), SendAllPeers: true, IncludeSelf: true})
		}
	}

	return now, changes, nil
}

// Routes

func (s *State) RoutesForPeer(srcNode, peerNode types.NodeView, matchers []matcher.Match) []netip.Prefix {
	return s.PrimaryRoutesForNode(peerNode.ID())
}

// Auth cache methods

func (s *State) GetAuthCacheEntry(id types.AuthID) (*types.AuthRequest, bool) {
	v, ok := s.authCache.Load(id)
	if !ok {
		return nil, false
	}
	return v.(*types.AuthRequest), true
}

func (s *State) SetAuthCacheEntry(id types.AuthID, entry *types.AuthRequest) {
	s.authCache.Store(id, entry)
}

// SSH check methods

func (s *State) SetLastSSHAuth(src, dst types.NodeID) {
	s.sshCheckMu.Lock()
	defer s.sshCheckMu.Unlock()
	s.sshCheckAuth[sshCheckPair{Src: src, Dst: dst}] = time.Now()
}

func (s *State) GetLastSSHAuth(src, dst types.NodeID) (time.Time, bool) {
	s.sshCheckMu.RLock()
	defer s.sshCheckMu.RUnlock()
	t, ok := s.sshCheckAuth[sshCheckPair{Src: src, Dst: dst}]
	return t, ok
}

func (s *State) ClearSSHCheckAuth() {
	s.sshCheckMu.Lock()
	defer s.sshCheckMu.Unlock()
	s.sshCheckAuth = make(map[sshCheckPair]time.Time)
}

// Ping

func (s *State) CompletePing(pingID string) bool {
	return true
}

// Debug

func (s *State) DebugString() string {
	return s.nodeStore.DebugString()
}

func (s *State) GetUserByProviderIdentifier(identifier string) (*types.User, error) {
	if s.repo == nil {
		return nil, ErrUserNotFound
	}
	users, err := s.repo.ListUsers()
	if err != nil {
		return nil, err
	}
	for _, user := range users {
		if user.ProviderIdentifier.Valid && user.ProviderIdentifier.String == identifier {
			return &user, nil
		}
	}
	return nil, ErrUserNotFound
}

func (s *State) GetUserByOIDCIdentifier(identifier string) (*types.User, error) {
	return s.GetUserByProviderIdentifier(identifier)
}

func (s *State) GetRegistrationData(authID types.AuthID) (*types.RegistrationData, bool) {
	if req, ok := s.GetAuthCacheEntry(authID); ok && req.RegistrationData() != nil {
		return req.RegistrationData(), true
	}
	return nil, false
}

func (s *State) HandleNodeFromAuthPath(
	registrationID types.AuthID,
	userID types.UserID,
	expiry *time.Time,
	registerMethod string,
) (types.NodeView, types.Change, error) {
	regData, ok := s.GetRegistrationData(registrationID)
	if !ok {
		return types.NodeView{}, types.Change{}, ErrNodeNotFound
	}

	node := &types.Node{
		MachineKey:     regData.MachineKey,
		NodeKey:        regData.NodeKey,
		DiscoKey:       regData.DiscoKey,
		Hostname:       regData.Hostname,
		Hostinfo:       regData.Hostinfo,
		Endpoints:      regData.Endpoints,
		Expiry:         expiry,
		RegisterMethod: registerMethod,
	}

	uid := uint(userID)
	node.UserID = &uid

	addedNode, err := s.AddNode(context.Background(), node)
	if err != nil {
		return types.NodeView{}, types.Change{}, err
	}
	return addedNode, types.NodeAdded(addedNode.ID()), nil
}

func (s *State) AutoApproveRoutes(node types.NodeView) (types.Change, error) {
	return types.Change{}, nil
}

func (s *State) SetTuningConfig(cfg *types.TuningConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
}

type gormDB = interface{}

var nodeUpdateColumns = []string{
	"MachineKey",
	"NodeKey",
	"DiscoKey",
	"Endpoints",
	"Hostinfo",
	"IPv4",
	"IPv6",
	"Hostname",
	"GivenName",
	"UserID",
	"RegisterMethod",
	"Tags",
	"AuthKeyID",
	"LastSeen",
	"ApprovedRoutes",
}
