package biz

import (
	"context"
	"net/netip"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/juanfont/headscale-v2/internal/types"
	"tailscale.com/types/key"
)

// Node is a Node model.
type Node struct {
	ID             types.NodeID
	MachineKey     key.MachinePublic
	NodeKey        key.NodePublic
	DiscoKey       key.DiscoPublic
	Hostname       string
	GivenName      string
	IPs            []netip.Addr
	Endpoints      []netip.AddrPort
	UserID         *uint
	User           *User
	Tags           []string
	ApprovedRoutes []netip.Prefix
	LastSeen       *time.Time
	Expiry         *time.Time
	IsOnline       *bool
	SessionEpoch   uint64
	RegisterMethod string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Nodes is a list of nodes.
type Nodes []*Node

// NodeRepo is a Node repository interface.
type NodeRepo interface {
	Save(ctx context.Context, node *Node) (*Node, error)
	FindByID(ctx context.Context, id int) (*Node, error)
	FindByMachineKey(ctx context.Context, machineKey key.MachinePublic) (*Node, error)
	FindByNodeKey(ctx context.Context, nodeKey key.NodePublic) (*Node, error)
	ListAll(ctx context.Context) ([]*Node, error)
	ListByUser(ctx context.Context, userID uint) ([]*Node, error)
	ListPeers(ctx context.Context, nodeID types.NodeID) ([]*Node, error)
	Delete(ctx context.Context, id int) error
	SetTags(ctx context.Context, nodeID types.NodeID, tags []string) error
	SetApprovedRoutes(ctx context.Context, nodeID types.NodeID, routes []netip.Prefix) error
	SetLastSeen(ctx context.Context, nodeID types.NodeID, lastSeen time.Time) error
	SetExpiry(ctx context.Context, nodeID types.NodeID, expiry *time.Time) error
	Rename(ctx context.Context, nodeID types.NodeID, newName string) error
	SetNodeKey(ctx context.Context, nodeID types.NodeID, nodeKey key.NodePublic) error
	SetMachineKey(ctx context.Context, nodeID types.NodeID, machineKey key.MachinePublic) error
	SetOnline(ctx context.Context, nodeID types.NodeID, isOnline bool) error
	ListEphemeral(ctx context.Context) ([]*Node, error)
	NodeCount(ctx context.Context) (int, error)
	NodeCountByUser(ctx context.Context, userID uint) (int, error)
	Transaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// NodeUsecase is a Node usecase.
type NodeUsecase struct {
	repo NodeRepo
	log  *log.Helper
}

// NewNodeUsecase creates a new Node usecase.
func NewNodeUsecase(repo NodeRepo, logger log.Logger) *NodeUsecase {
	return &NodeUsecase{
		repo: repo,
		log:  log.NewHelper(logger),
	}
}

// CreateNode creates a Node.
func (uc *NodeUsecase) CreateNode(ctx context.Context, node *Node) (*Node, error) {
	return uc.repo.Save(ctx, node)
}

// GetNode gets a Node by ID.
func (uc *NodeUsecase) GetNode(ctx context.Context, id int) (*Node, error) {
	return uc.repo.FindByID(ctx, id)
}

// GetNodeByMachineKey gets a Node by MachineKey.
func (uc *NodeUsecase) GetNodeByMachineKey(ctx context.Context, machineKey key.MachinePublic) (*Node, error) {
	return uc.repo.FindByMachineKey(ctx, machineKey)
}

// GetNodeByNodeKey gets a Node by NodeKey.
func (uc *NodeUsecase) GetNodeByNodeKey(ctx context.Context, nodeKey key.NodePublic) (*Node, error) {
	return uc.repo.FindByNodeKey(ctx, nodeKey)
}

// ListNodes lists all Nodes.
func (uc *NodeUsecase) ListNodes(ctx context.Context) ([]*Node, error) {
	return uc.repo.ListAll(ctx)
}

// ListNodesByUser lists nodes by user.
func (uc *NodeUsecase) ListNodesByUser(ctx context.Context, userID uint) ([]*Node, error) {
	return uc.repo.ListByUser(ctx, userID)
}

// ListPeers lists peers for a node.
func (uc *NodeUsecase) ListPeers(ctx context.Context, nodeID types.NodeID) ([]*Node, error) {
	return uc.repo.ListPeers(ctx, nodeID)
}

// DeleteNode deletes a Node.
func (uc *NodeUsecase) DeleteNode(ctx context.Context, id int) error {
	return uc.repo.Delete(ctx, id)
}

// SetTags sets tags for a node.
func (uc *NodeUsecase) SetTags(ctx context.Context, nodeID types.NodeID, tags []string) error {
	return uc.repo.SetTags(ctx, nodeID, tags)
}

// SetApprovedRoutes sets approved routes for a node.
func (uc *NodeUsecase) SetApprovedRoutes(ctx context.Context, nodeID types.NodeID, routes []netip.Prefix) error {
	return uc.repo.SetApprovedRoutes(ctx, nodeID, routes)
}

// SetLastSeen sets last seen time for a node.
func (uc *NodeUsecase) SetLastSeen(ctx context.Context, nodeID types.NodeID, lastSeen time.Time) error {
	return uc.repo.SetLastSeen(ctx, nodeID, lastSeen)
}

// SetExpiry sets expiry time for a node.
func (uc *NodeUsecase) SetExpiry(ctx context.Context, nodeID types.NodeID, expiry *time.Time) error {
	return uc.repo.SetExpiry(ctx, nodeID, expiry)
}

// RenameNode renames a node.
func (uc *NodeUsecase) RenameNode(ctx context.Context, nodeID types.NodeID, newName string) error {
	return uc.repo.Rename(ctx, nodeID, newName)
}

// SetNodeKey sets the node key.
func (uc *NodeUsecase) SetNodeKey(ctx context.Context, nodeID types.NodeID, nodeKey key.NodePublic) error {
	return uc.repo.SetNodeKey(ctx, nodeID, nodeKey)
}

// SetMachineKey sets the machine key.
func (uc *NodeUsecase) SetMachineKey(ctx context.Context, nodeID types.NodeID, machineKey key.MachinePublic) error {
	return uc.repo.SetMachineKey(ctx, nodeID, machineKey)
}

// SetOnline sets the online status.
func (uc *NodeUsecase) SetOnline(ctx context.Context, nodeID types.NodeID, isOnline bool) error {
	return uc.repo.SetOnline(ctx, nodeID, isOnline)
}

// ListEphemeralNodes lists ephemeral nodes.
func (uc *NodeUsecase) ListEphemeralNodes(ctx context.Context) ([]*Node, error) {
	return uc.repo.ListEphemeral(ctx)
}

// NodeCount returns total node count.
func (uc *NodeUsecase) NodeCount(ctx context.Context) (int, error) {
	return uc.repo.NodeCount(ctx)
}

// NodeCountByUser returns node count for a user.
func (uc *NodeUsecase) NodeCountByUser(ctx context.Context, userID uint) (int, error) {
	return uc.repo.NodeCountByUser(ctx, userID)
}