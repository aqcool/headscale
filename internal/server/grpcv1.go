package server

import (
	"context"

	v1 "github.com/juanfont/headscale-v2/api/proto/v1"
)

// Health check endpoint
func (h *Headscale) Health(ctx context.Context, req *v1.HealthRequest) (*v1.HealthResponse, error) {
	return &v1.HealthResponse{}, nil
}

// ListNodes returns all nodes (placeholder)
func (h *Headscale) ListNodes(ctx context.Context, req *v1.ListNodesRequest) (*v1.ListNodesResponse, error) {
	nodes := h.state.ListNodes()
	protoNodes := make([]*v1.Node, 0, len(nodes))
	for _, n := range nodes {
		protoNodes = append(protoNodes, &v1.Node{
			Id:        uint64(n.ID()),
			Name:      n.Hostname(),
			GivenName: n.Hostname(),
		})
	}
	return &v1.ListNodesResponse{Nodes: protoNodes}, nil
}
