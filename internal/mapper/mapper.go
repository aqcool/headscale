package mapper

import (
	"context"
	"errors"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/juanfont/headscale-v2/internal/state"
	"github.com/juanfont/headscale-v2/internal/types"
	"tailscale.com/tailcfg"
)

var (
	ErrNodeNotFound       = errors.New("node not found")
	ErrNodeNotFoundMapper = errors.New("node not found in mapper")
)

// Mapper handles MapResponse generation.
type Mapper struct {
	state  *state.State
	cfg    *types.Config
	logger *log.Helper
}

// NewMapper creates a new Mapper.
func NewMapper(st *state.State, cfg *types.Config, logger log.Logger) *Mapper {
	return &Mapper{
		state:  st,
		cfg:    cfg,
		logger: log.NewHelper(logger),
	}
}

// BuildMapResponse builds a MapResponse for a node.
func (m *Mapper) BuildMapResponse(
	ctx context.Context,
	nodeID types.NodeID,
	req tailcfg.MapRequest,
) (*tailcfg.MapResponse, error) {
	nodeView, ok := m.state.GetNodeByID(nodeID)
	if !ok || !nodeView.Valid() {
		return nil, ErrNodeNotFound
	}

	builder := NewMapResponseBuilder(m, nodeID).
		WithCapabilityVersion(req.Version).
		WithDERPMap().
		WithDomain().
		WithSelfNode()

	if !req.OmitPeers {
		builder.WithPeers()
	}

	return builder.Build()
}

// FullMapResponse returns a complete MapResponse for initial connection.
func (m *Mapper) FullMapResponse(
	nodeID types.NodeID,
	capVer tailcfg.CapabilityVersion,
) (*tailcfg.MapResponse, error) {
	return NewMapResponseBuilder(m, nodeID).
		WithCapabilityVersion(capVer).
		WithSelfNode().
		WithDERPMap().
		WithDomain().
		WithCollectServicesDisabled().
		WithSSHPolicy().
		WithPacketFilters().
		WithPeers().
		Build()
}

// SelfMapResponse returns a self-node update.
func (m *Mapper) SelfMapResponse(
	nodeID types.NodeID,
	capVer tailcfg.CapabilityVersion,
) (*tailcfg.MapResponse, error) {
	resp, err := NewMapResponseBuilder(m, nodeID).
		WithCapabilityVersion(capVer).
		WithSelfNode().
		Build()
	if err != nil {
		return nil, err
	}
	resp.Peers = nil
	return resp, nil
}

// ChangeMapResponse builds a MapResponse from a Change.
func (m *Mapper) ChangeMapResponse(
	nodeID types.NodeID,
	capVer tailcfg.CapabilityVersion,
	change types.Change,
) (*tailcfg.MapResponse, error) {
	if change.SendAllPeers {
		return m.FullMapResponse(nodeID, capVer)
	}

	builder := NewMapResponseBuilder(m, nodeID).
		WithCapabilityVersion(capVer)

	if len(change.PeersChanged) > 0 {
		peers := make([]types.NodeView, 0, len(change.PeersChanged))
		for _, id := range change.PeersChanged {
			if p, ok := m.state.GetNodeByID(id); ok {
				peers = append(peers, p)
			}
		}
		builder.WithPeerChanges(peers)
	}

	if len(change.PeersRemoved) > 0 {
		builder.WithPeersRemoved(change.PeersRemoved...)
	}

	return builder.Build()
}
