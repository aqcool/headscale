package data

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/juanfont/headscale-v2/ent"
	"github.com/juanfont/headscale-v2/ent/node"
	"github.com/juanfont/headscale-v2/ent/user"
	"github.com/juanfont/headscale-v2/internal/biz"
	"github.com/juanfont/headscale-v2/internal/types"
	"tailscale.com/types/key"
	"tailscale.com/util/dnsname"
)

var (
	ErrNodeNotFound            = errors.New("node not found")
	ErrNodeNameNotUnique       = errors.New("node name is not unique")
	ErrNodeRouteIsNotAvailable = errors.New("route is not available on node")
)

type nodeRepo struct {
	data *Data
	log  *log.Helper
}

// NewNodeRepo creates a new node repository.
func NewNodeRepo(data *Data, logger log.Logger) biz.NodeRepo {
	return &nodeRepo{
		data: data,
		log:  log.NewHelper(logger),
	}
}

// Save creates or updates a node.
func (r *nodeRepo) Save(ctx context.Context, n *biz.Node) (*biz.Node, error) {
	// Convert IPs to strings
	var ipAddrs []string
	for _, ip := range n.IPs {
		ipAddrs = append(ipAddrs, ip.String())
	}

	// Convert endpoints to strings
	var endpoints []string
	for _, ep := range n.Endpoints {
		endpoints = append(endpoints, ep.String())
	}

	// Convert tags to strings
	var tags []string
	tags = append(tags, n.Tags...)

	// Convert routes to strings
	var approvedRoutes []string
	for _, route := range n.ApprovedRoutes {
		approvedRoutes = append(approvedRoutes, route.String())
	}

	if n.ID == 0 {
		// Create new node
		query := r.data.db.Node.Create().
			SetMachineKey(n.MachineKey.String()).
			SetNodeKey(n.NodeKey.String()).
			SetDiscoKey(n.DiscoKey.String()).
			SetName(n.Hostname).
			SetGivenName(n.GivenName).
			SetIPAddresses(ipAddrs).
			SetEndpoints(endpoints).
			SetTags(tags).
			SetApprovedRoutes(approvedRoutes).
			SetRegisterMethod(n.RegisterMethod).
			SetSessionEpoch(n.SessionEpoch)

		if n.UserID != nil {
			query.SetUserID(int(*n.UserID))
		}
		if n.Expiry != nil {
			query.SetExpiry(*n.Expiry)
		}
		if n.LastSeen != nil {
			query.SetLastSeen(*n.LastSeen)
		}
		if n.IsOnline != nil {
			query.SetIsOnline(*n.IsOnline)
		}

		po, err := query.Save(ctx)
		if err != nil {
			return nil, err
		}
		return entNodeToBiz(po), nil
	}

	// Update existing node
	query := r.data.db.Node.UpdateOneID(int(n.ID)).
		SetNodeKey(n.NodeKey.String()).
		SetDiscoKey(n.DiscoKey.String()).
		SetName(n.Hostname).
		SetIPAddresses(ipAddrs).
		SetEndpoints(endpoints).
		SetApprovedRoutes(approvedRoutes).
		SetSessionEpoch(n.SessionEpoch)

	if n.LastSeen != nil {
		query.SetLastSeen(*n.LastSeen)
	}
	if n.IsOnline != nil {
		query.SetIsOnline(*n.IsOnline)
	}

	po, err := query.Save(ctx)
	if err != nil {
		return nil, err
	}
	return entNodeToBiz(po), nil
}

// FindByID finds a node by ID.
func (r *nodeRepo) FindByID(ctx context.Context, id int) (*biz.Node, error) {
	po, err := r.data.db.Node.Query().
		Where(node.ID(id)).
		WithUser().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return entNodeToBiz(po), nil
}

// FindByMachineKey finds a node by machine key.
func (r *nodeRepo) FindByMachineKey(ctx context.Context, machineKey key.MachinePublic) (*biz.Node, error) {
	po, err := r.data.db.Node.Query().
		Where(node.MachineKey(machineKey.String())).
		WithUser().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return entNodeToBiz(po), nil
}

// FindByNodeKey finds a node by node key.
func (r *nodeRepo) FindByNodeKey(ctx context.Context, nodeKey key.NodePublic) (*biz.Node, error) {
	po, err := r.data.db.Node.Query().
		Where(node.NodeKey(nodeKey.String())).
		WithUser().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return entNodeToBiz(po), nil
}

// ListAll lists all nodes.
func (r *nodeRepo) ListAll(ctx context.Context) ([]*biz.Node, error) {
	pos, err := r.data.db.Node.Query().
		WithUser().
		All(ctx)
	if err != nil {
		return nil, err
	}
	nodes := make([]*biz.Node, 0, len(pos))
	for _, po := range pos {
		nodes = append(nodes, entNodeToBiz(po))
	}
	return nodes, nil
}

// ListByUser lists nodes by user ID.
func (r *nodeRepo) ListByUser(ctx context.Context, userID uint) ([]*biz.Node, error) {
	pos, err := r.data.db.Node.Query().
		Where(node.HasUserWith(user.ID(int(userID)))).
		WithUser().
		All(ctx)
	if err != nil {
		return nil, err
	}
	nodes := make([]*biz.Node, 0, len(pos))
	for _, po := range pos {
		nodes = append(nodes, entNodeToBiz(po))
	}
	return nodes, nil
}

// ListPeers lists all nodes except the given node ID.
func (r *nodeRepo) ListPeers(ctx context.Context, nodeID types.NodeID) ([]*biz.Node, error) {
	pos, err := r.data.db.Node.Query().
		Where(node.IDNEQ(int(nodeID))).
		WithUser().
		All(ctx)
	if err != nil {
		return nil, err
	}
	nodes := make([]*biz.Node, 0, len(pos))
	for _, po := range pos {
		nodes = append(nodes, entNodeToBiz(po))
	}
	return nodes, nil
}

// Delete deletes a node by ID.
func (r *nodeRepo) Delete(ctx context.Context, id int) error {
	return r.data.db.Node.DeleteOneID(id).Exec(ctx)
}

// SetTags sets the tags for a node.
func (r *nodeRepo) SetTags(ctx context.Context, nodeID types.NodeID, tags []string) error {
	slices.Sort(tags)
	tags = slices.Compact(tags)
	return r.data.db.Node.UpdateOneID(int(nodeID)).
		SetTags(tags).
		Exec(ctx)
}

// SetApprovedRoutes sets the approved routes for a node.
func (r *nodeRepo) SetApprovedRoutes(ctx context.Context, nodeID types.NodeID, routes []netip.Prefix) error {
	var routeStrs []string
	for _, r := range routes {
		routeStrs = append(routeStrs, r.String())
	}
	return r.data.db.Node.UpdateOneID(int(nodeID)).
		SetApprovedRoutes(routeStrs).
		Exec(ctx)
}

// SetLastSeen updates the last seen timestamp for a node.
func (r *nodeRepo) SetLastSeen(ctx context.Context, nodeID types.NodeID, lastSeen time.Time) error {
	return r.data.db.Node.UpdateOneID(int(nodeID)).
		SetLastSeen(lastSeen).
		Exec(ctx)
}

// SetExpiry sets the expiry time for a node.
func (r *nodeRepo) SetExpiry(ctx context.Context, nodeID types.NodeID, expiry *time.Time) error {
	if expiry == nil {
		return r.data.db.Node.UpdateOneID(int(nodeID)).
			ClearExpiry().
			Exec(ctx)
	}
	return r.data.db.Node.UpdateOneID(int(nodeID)).
		SetExpiry(*expiry).
		Exec(ctx)
}

// Rename renames a node.
func (r *nodeRepo) Rename(ctx context.Context, nodeID types.NodeID, newName string) error {
	if err := dnsname.ValidLabel(newName); err != nil {
		return fmt.Errorf("renaming node: %w", err)
	}

	// Check uniqueness
	count, err := r.data.db.Node.Query().
		Where(
			node.GivenName(newName),
			node.IDNEQ(int(nodeID)),
		).
		Count(ctx)
	if err != nil {
		return fmt.Errorf("checking name uniqueness: %w", err)
	}
	if count > 0 {
		return ErrNodeNameNotUnique
	}

	return r.data.db.Node.UpdateOneID(int(nodeID)).
		SetGivenName(newName).
		Exec(ctx)
}

// SetNodeKey sets the node key for a node.
func (r *nodeRepo) SetNodeKey(ctx context.Context, nodeID types.NodeID, nodeKey key.NodePublic) error {
	return r.data.db.Node.UpdateOneID(int(nodeID)).
		SetNodeKey(nodeKey.String()).
		Exec(ctx)
}

// SetMachineKey sets the machine key for a node.
func (r *nodeRepo) SetMachineKey(ctx context.Context, nodeID types.NodeID, machineKey key.MachinePublic) error {
	return r.data.db.Node.UpdateOneID(int(nodeID)).
		SetMachineKey(machineKey.String()).
		Exec(ctx)
}

// SetOnline sets the online status for a node.
func (r *nodeRepo) SetOnline(ctx context.Context, nodeID types.NodeID, isOnline bool) error {
	return r.data.db.Node.UpdateOneID(int(nodeID)).
		SetIsOnline(isOnline).
		Exec(ctx)
}

// ListEphemeral lists all ephemeral nodes.
func (r *nodeRepo) ListEphemeral(ctx context.Context) ([]*biz.Node, error) {
	pos, err := r.data.db.Node.Query().
		Where(node.Ephemeral(true)).
		WithUser().
		All(ctx)
	if err != nil {
		return nil, err
	}
	nodes := make([]*biz.Node, 0, len(pos))
	for _, po := range pos {
		nodes = append(nodes, entNodeToBiz(po))
	}
	return nodes, nil
}

// entNodeToBiz converts an ent.Node to a biz.Node.
func entNodeToBiz(po *ent.Node) *biz.Node {
	n := &biz.Node{
		ID:             types.NodeID(po.ID),
		MachineKey:     parseMachineKey(po.MachineKey),
		NodeKey:        parseNodeKey(po.NodeKey),
		DiscoKey:       parseDiscoKey(po.DiscoKey),
		Hostname:       po.Name,
		GivenName:      po.GivenName,
		Tags:           po.Tags,
		RegisterMethod: po.RegisterMethod,
		IsOnline:       ptrBool(po.IsOnline),
		SessionEpoch:   po.SessionEpoch,
		CreatedAt:      po.CreatedAt,
		UpdatedAt:       po.UpdatedAt,
	}

	// Parse IP addresses
	for _, ipStr := range po.IPAddresses {
		if ip, err := netip.ParseAddr(ipStr); err == nil {
			n.IPs = append(n.IPs, ip)
		}
	}

	// Parse endpoints
	for _, epStr := range po.Endpoints {
		if ep, err := netip.ParseAddrPort(epStr); err == nil {
			n.Endpoints = append(n.Endpoints, ep)
		}
	}

	// Parse approved routes
	for _, r := range po.ApprovedRoutes {
		if pfx, err := netip.ParsePrefix(r); err == nil {
			n.ApprovedRoutes = append(n.ApprovedRoutes, pfx)
		}
	}

	// Set user if loaded
	if po.Edges.User != nil {
		userID := uint(po.Edges.User.ID)
		n.UserID = &userID
		n.User = &biz.User{
			ID:          po.Edges.User.ID,
			Name:        po.Edges.User.Name,
			DisplayName: po.Edges.User.DisplayName,
			Email:       po.Edges.User.Email,
			CreatedAt:   po.Edges.User.CreatedAt,
		}
	}

	// Set timestamps
	if po.LastSeen != nil {
		n.LastSeen = po.LastSeen
	}
	if po.Expiry != nil {
		n.Expiry = po.Expiry
	}

	return n
}

// Helper functions
func parseMachineKey(s string) key.MachinePublic {
	var mk key.MachinePublic
	_ = mk.UnmarshalText([]byte(s))
	return mk
}

func parseNodeKey(s string) key.NodePublic {
	var nk key.NodePublic
	_ = nk.UnmarshalText([]byte(s))
	return nk
}

func parseDiscoKey(s string) key.DiscoPublic {
	var dk key.DiscoPublic
	_ = dk.UnmarshalText([]byte(s))
	return dk
}

func ptrBool(b bool) *bool {
	return &b
}

// NodeCount returns the total number of nodes.
func (r *nodeRepo) NodeCount(ctx context.Context) (int, error) {
	return r.data.db.Node.Query().Count(ctx)
}

// NodeCountByUser returns the number of nodes for a user.
func (r *nodeRepo) NodeCountByUser(ctx context.Context, userID uint) (int, error) {
	return r.data.db.Node.Query().
		Where(node.HasUserWith(user.ID(int(userID)))).
		Count(ctx)
}

// PurgeNodes deletes all nodes (for testing).
func (r *nodeRepo) PurgeNodes(ctx context.Context) error {
	_, err := r.data.db.Node.Delete().Exec(ctx)
	return err
}

// Transaction executes a function within a transaction.
func (r *nodeRepo) Transaction(ctx context.Context, fn func(ctx context.Context) error) error {
	tx, err := r.data.db.Tx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := fn(ctx); err != nil {
		return err
	}
	return tx.Commit()
}
