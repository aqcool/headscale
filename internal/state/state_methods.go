package state

import (
	"errors"
	"net/netip"
	"slices"
	"time"

	"github.com/juanfont/headscale-v2/internal/types"
	"tailscale.com/types/views"
)

// SetNodeExpiry sets the expiry time for a node.
func (s *State) SetNodeExpiry(nodeID types.NodeID, expiry *time.Time) (types.NodeView, types.Change, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	nodeView, ok := s.nodeStore.UpdateNode(nodeID, func(n *types.Node) {
		n.Expiry = expiry
	})
	if !ok {
		return types.NodeView{}, types.Change{}, ErrNodeNotFound
	}

	resultNode, c, err := s.persistNodeToDB(nodeView)
	if err != nil {
		return types.NodeView{}, types.Change{}, err
	}

	return resultNode, c, nil
}

// SetNodeTags sets tags for a node.
func (s *State) SetNodeTags(nodeID types.NodeID, tags []string) (types.NodeView, types.Change, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	slices.Sort(tags)

	nodeView, ok := s.nodeStore.UpdateNode(nodeID, func(n *types.Node) {
		n.Tags = tags
	})
	if !ok {
		return types.NodeView{}, types.Change{}, ErrNodeNotFound
	}

	resultNode, c, err := s.persistNodeToDB(nodeView)
	if err != nil {
		return types.NodeView{}, types.Change{}, err
	}

	policyChange, err := s.updatePolicyManagerNodes()
	if err != nil {
		return resultNode, types.Change{}, err
	}

	if !policyChange.IsEmpty() {
		c = c.Merge(policyChange)
	}

	return resultNode, c, nil
}

// SetApprovedRoutes sets approved routes for a node.
func (s *State) SetApprovedRoutes(nodeID types.NodeID, routes []netip.Prefix) (types.NodeView, types.Change, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	nodeView, ok := s.nodeStore.UpdateNode(nodeID, func(n *types.Node) {
		n.ApprovedRoutes = routes
	})
	if !ok {
		return types.NodeView{}, types.Change{}, ErrNodeNotFound
	}

	resultNode, c, err := s.persistNodeToDB(nodeView)
	if err != nil {
		return types.NodeView{}, types.Change{}, err
	}

	return resultNode, c, nil
}

// RenameNode renames a node.
func (s *State) RenameNode(nodeID types.NodeID, newName string) (types.NodeView, types.Change, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	nodeView, err := s.nodeStore.SetGivenName(nodeID, newName)
	if err != nil {
		return types.NodeView{}, types.Change{}, err
	}

	resultNode, c, err := s.persistNodeToDB(nodeView)
	if err != nil {
		return types.NodeView{}, types.Change{}, err
	}

	return resultNode, c, nil
}

// UpdateUser updates a user in the database.
func (s *State) UpdateUser(userID types.UserID, updateFn func(*types.User) error) (*types.User, types.Change, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db == nil {
		return nil, types.Change{}, errors.New("database not available")
	}

	user, err := s.db.UpdateUser(userID, updateFn)
	if err != nil {
		return nil, types.Change{}, err
	}

	return user, types.PolicyChange(), nil
}

// RenameUser renames a user.
func (s *State) RenameUser(userID types.UserID, newName string) (*types.User, types.Change, error) {
	return s.UpdateUser(userID, func(u *types.User) error {
		u.Name = newName
		return nil
	})
}

// CreateAPIKey creates a new API key.
func (s *State) CreateAPIKey(expiration *time.Time) (string, *types.APIKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db == nil {
		return "", nil, errors.New("database not available")
	}

	return s.db.CreateAPIKey(expiration)
}

// GetAPIKey gets an API key by prefix.
func (s *State) GetAPIKey(displayPrefix string) (*types.APIKey, error) {
	if s.db == nil {
		return nil, errors.New("database not available")
	}
	return s.db.GetAPIKey(displayPrefix)
}

// GetAPIKeyByID gets an API key by ID.
func (s *State) GetAPIKeyByID(id uint64) (*types.APIKey, error) {
	if s.db == nil {
		return nil, errors.New("database not available")
	}
	return s.db.GetAPIKeyByID(id)
}

// ExpireAPIKey expires an API key.
func (s *State) ExpireAPIKey(key *types.APIKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db == nil {
		return errors.New("database not available")
	}

	return s.db.ExpireAPIKey(key)
}

// ListAPIKeys lists all API keys.
func (s *State) ListAPIKeys() ([]types.APIKey, error) {
	if s.db == nil {
		return nil, errors.New("database not available")
	}
	return s.db.ListAPIKeys()
}

// DestroyAPIKey destroys an API key.
func (s *State) DestroyAPIKey(key types.APIKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db == nil {
		return errors.New("database not available")
	}

	return s.db.DestroyAPIKey(key)
}

// SetNodeUnhealthy sets the unhealthy status for a node.
func (s *State) SetNodeUnhealthy(id types.NodeID, unhealthy bool) bool {
	_, ok := s.nodeStore.UpdateNode(id, func(n *types.Node) {
		n.Unhealthy = unhealthy
	})
	return ok
}

// ResolveNode resolves a node by query (ID, IP, hostname, or given name).
func (s *State) ResolveNode(query string) (types.NodeView, bool) {
	for _, node := range s.ListNodes() {
		if node.GivenName() == query {
			return node, true
		}
		if node.Hostname() == query {
			return node, true
		}
		if node.IPv4().String() == query || node.IPv6().String() == query {
			return node, true
		}
	}
	return types.NodeView{}, false
}

// ListEphemeralNodes lists all ephemeral nodes.
func (s *State) ListEphemeralNodes() views.Slice[types.NodeView] {
	var ephemeral []types.NodeView
	for _, node := range s.ListNodes() {
		if node.IsEphemeral() {
			ephemeral = append(ephemeral, node)
		}
	}
	return views.SliceOf(ephemeral)
}

// BackfillNodeIPs backfills missing IPs for nodes.
func (s *State) BackfillNodeIPs() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ipAlloc == nil {
		return nil, nil
	}

	var backfilled []string
	for _, node := range s.ListNodes() {
		if !node.IPv4().IsValid() || !node.IPv6().IsValid() {
			v4, v6, err := s.ipAlloc.Next()
			if err != nil {
				return nil, err
			}
			_, ok := s.nodeStore.UpdateNode(node.ID(), func(n *types.Node) {
				if !n.IPv4.IsValid() {
					n.IPv4 = v4
				}
				if !n.IPv6.IsValid() {
					n.IPv6 = v6
				}
			})
			if ok {
				backfilled = append(backfilled, node.GivenName())
			}
		}
	}

	return backfilled, nil
}

// NodeCanHaveTag checks if a node can have a specific tag.
func (s *State) NodeCanHaveTag(node types.NodeView, tag string) bool {
	if s.polMan == nil {
		return false
	}
	return s.polMan.NodeCanHaveTag(node, tag)
}

// SSHCheckParams returns SSH check duration for a src/dst pair.
func (s *State) SSHCheckParams(srcNodeID, dstNodeID types.NodeID) (time.Duration, bool) {
	if s.polMan == nil {
		return 0, false
	}
	return s.polMan.SSHCheckParams(srcNodeID, dstNodeID)
}

// GetNodePrimaryRoutes returns primary routes advertised by a node.
func (s *State) GetNodePrimaryRoutes(nodeID types.NodeID) []netip.Prefix {
	return s.nodeStore.PrimaryRoutesForNode(nodeID)
}

// ListNodesWithFilter returns nodes matching a filter.
func (s *State) ListNodesWithFilter(filter *types.Node) ([]types.NodeView, error) {
	var result []types.NodeView
	for _, node := range s.ListNodes() {
		if filter.ID != 0 && node.ID() != filter.ID {
			continue
		}
		if filter.Hostname != "" && node.Hostname() != filter.Hostname {
			continue
		}
		result = append(result, node)
	}
	return result, nil
}

// CreateNodeForTest creates a test node.
func (s *State) CreateNodeForTest(user *types.User, hostname ...string) *types.Node {
	name := "test"
	if len(hostname) > 0 {
		name = hostname[0]
	}

	node := &types.Node{
		Hostname: name,
	}
	if user != nil {
		uid := uint(user.ID)
		node.UserID = &uid
		node.User = user
	}
	return node
}

// PutNodeInStoreForTest adds a node to the store for testing.
func (s *State) PutNodeInStoreForTest(node types.Node) types.NodeView {
	return s.nodeStore.PutNode(node)
}

// CreateRegisteredNodeForTest creates and registers a test node.
func (s *State) CreateRegisteredNodeForTest(user *types.User, hostname ...string) *types.Node {
	node := s.CreateNodeForTest(user, hostname...)
	if s.ipAlloc != nil {
		v4, v6, err := s.ipAlloc.Next()
		if err == nil {
			node.IPv4 = v4
			node.IPv6 = v6
		}
	}
	return node
}

// CreateUserForTest creates a test user.
func (s *State) CreateUserForTest(name ...string) *types.User {
	n := "test"
	if len(name) > 0 {
		n = name[0]
	}
	return &types.User{Name: n}
}

// CreateNodesForTest creates multiple test nodes.
func (s *State) CreateNodesForTest(user *types.User, count int, namePrefix ...string) []*types.Node {
	prefix := "test"
	if len(namePrefix) > 0 {
		prefix = namePrefix[0]
	}

	nodes := make([]*types.Node, count)
	for i := 0; i < count; i++ {
		nodes[i] = s.CreateNodeForTest(user, prefix+"-"+string(rune('0'+i)))
	}
	return nodes
}

// CreateUsersForTest creates multiple test users.
func (s *State) CreateUsersForTest(count int, namePrefix ...string) []*types.User {
	prefix := "test"
	if len(namePrefix) > 0 {
		prefix = namePrefix[0]
	}

	users := make([]*types.User, count)
	for i := 0; i < count; i++ {
		users[i] = &types.User{Name: prefix + "-" + string(rune('0'+i))}
	}
	return users
}

// ListAllUsers returns all users.
func (s *State) ListAllUsers() ([]types.User, error) {
	return s.ListUsers()
}

// ListPreAuthKeys lists all pre-auth keys.
func (s *State) ListPreAuthKeys() ([]types.PreAuthKey, error) {
	if s.db == nil {
		return nil, errors.New("database not available")
	}
	return s.db.ListPreAuthKeys()
}

// GetPolicy retrieves the current policy.
func (s *State) GetPolicy() (*types.Policy, error) {
	if s.db == nil {
		return nil, errors.New("database not available")
	}
	return s.db.GetPolicy()
}

// DeletePreAuthKey deletes a pre-auth key.
func (s *State) DeletePreAuthKey(id uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db == nil {
		return errors.New("database not available")
	}

	return s.db.DeletePreAuthKey(id)
}
