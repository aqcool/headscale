package noise

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"net/http"
	"time"

	"github.com/juanfont/headscale-v2/internal/types"
	"tailscale.com/control/controlbase"
	"tailscale.com/tailcfg"
)

func (s *NoiseServer) handleRegisterHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgradeToNoise(r)
	if err != nil {
		s.logger.Errorf("Noise upgrade failed: %v", err)
		http.Error(w, "Noise upgrade failed", http.StatusBadRequest)
		return
	}
	defer conn.Close()

	s.handleConn(r.Context(), conn)
}

func (s *NoiseServer) handleMapHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgradeToNoise(r)
	if err != nil {
		s.logger.Errorf("Noise upgrade failed: %v", err)
		http.Error(w, "Noise upgrade failed", http.StatusBadRequest)
		return
	}
	defer conn.Close()

	s.handleConn(r.Context(), conn)
}

func (s *NoiseServer) handleRegisterMessage(ctx context.Context, conn *controlbase.Conn, data []byte) {
	var req tailcfg.RegisterRequest
	if err := json.Unmarshal(data, &req); err != nil {
		s.logger.Errorf("Failed to parse register request: %v", err)
		return
	}

	s.logger.Infof("Register request from node key: %s", req.NodeKey.ShortString())

	resp, err := s.processRegister(ctx, req)
	if err != nil {
		s.logger.Errorf("Registration failed: %v", err)
		return
	}

	respData, err := json.Marshal(resp)
	if err != nil {
		s.logger.Errorf("Failed to marshal register response: %v", err)
		return
	}

	if err := writeMsg(conn, respData); err != nil {
		s.logger.Errorf("Failed to send register response: %v", err)
	}
}

func (s *NoiseServer) processRegister(ctx context.Context, req tailcfg.RegisterRequest) (*tailcfg.RegisterResponse, error) {
	// Check for logout (expiry in the past)
	if !req.Expiry.IsZero() && req.Expiry.Before(time.Now()) {
		return s.processLogout(ctx, req)
	}

	// Check if node already exists
	nodeView, ok := s.state.GetNodeByNodeKey(req.NodeKey)
	if ok && nodeView.Valid() {
		return s.updateExistingNode(ctx, req, nodeView)
	}

	// New node registration
	return s.registerNewNode(ctx, req)
}

func (s *NoiseServer) processLogout(ctx context.Context, req tailcfg.RegisterRequest) (*tailcfg.RegisterResponse, error) {
	nodeView, ok := s.state.GetNodeByNodeKey(req.NodeKey)
	if !ok || !nodeView.Valid() {
		return nil, ErrInvalidNodeKey
	}

	// Mark node as expired
	_, err := s.state.UpdateNode(nodeView.ID(), func(n *types.Node) {
		now := time.Now()
		n.Expiry = &now
		n.LastSeen = &now
	})
	if err != nil {
		return nil, fmt.Errorf("failed to expire node: %w", err)
	}

	return &tailcfg.RegisterResponse{
		MachineAuthorized: false,
	}, nil
}

func (s *NoiseServer) updateExistingNode(ctx context.Context, req tailcfg.RegisterRequest, nodeView types.NodeView) (*tailcfg.RegisterResponse, error) {
	// Update node keys
	_, err := s.state.UpdateNode(nodeView.ID(), func(n *types.Node) {
		n.NodeKey = req.NodeKey
		if req.Hostinfo != nil {
			n.Hostinfo = req.Hostinfo
		}
		if !req.Expiry.IsZero() {
			n.Expiry = &req.Expiry
		}
		now := time.Now()
		n.LastSeen = &now
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update node: %w", err)
	}

	return &tailcfg.RegisterResponse{
		MachineAuthorized: true,
	}, nil
}

func (s *NoiseServer) registerNewNode(ctx context.Context, req tailcfg.RegisterRequest) (*tailcfg.RegisterResponse, error) {
	// Create new node
	node := &types.Node{
		NodeKey:   req.NodeKey,
		Hostinfo:  req.Hostinfo,
		CreatedAt: time.Now(),
	}

	if !req.Expiry.IsZero() {
		node.Expiry = &req.Expiry
	}

	now := time.Now()
	node.LastSeen = &now
	node.IsOnline = boolPtr(true)

	if req.Hostinfo != nil {
		node.Hostname = req.Hostinfo.Hostname
		node.GivenName = req.Hostinfo.Hostname
	}

	_, err := s.state.AddNode(ctx, node)
	if err != nil {
		return nil, fmt.Errorf("failed to add node: %w", err)
	}

	return &tailcfg.RegisterResponse{
		MachineAuthorized: true,
	}, nil
}

func (s *NoiseServer) handleMapRequest(ctx context.Context, conn *controlbase.Conn, data []byte) {
	var req tailcfg.MapRequest
	if err := json.Unmarshal(data, &req); err != nil {
		s.logger.Errorf("Failed to parse map request: %v", err)
		return
	}

	nodeView, ok := s.state.GetNodeByNodeKey(req.NodeKey)
	if !ok || !nodeView.Valid() {
		s.logger.Errorf("Node not found for node key: %s", req.NodeKey.ShortString())
		return
	}

	// Update node state
	_, err := s.state.UpdateNodeFromMapRequest(nodeView.ID(), req)
	if err != nil {
		s.logger.Errorf("Failed to update node from map request: %v", err)
		return
	}

	// Build and send MapResponse
	resp, err := s.buildMapResponse(ctx, req, nodeView)
	if err != nil {
		s.logger.Errorf("Failed to build map response: %v", err)
		return
	}

	respData, err := json.Marshal(resp)
	if err != nil {
		s.logger.Errorf("Failed to marshal map response: %v", err)
		return
	}

	if err := writeMsg(conn, respData); err != nil {
		s.logger.Errorf("Failed to send map response: %v", err)
	}
}

func (s *NoiseServer) buildMapResponse(ctx context.Context, req tailcfg.MapRequest, nodeView types.NodeView) (*tailcfg.MapResponse, error) {
	resp := &tailcfg.MapResponse{
		KeepAlive: false,
		DERPMap:   s.state.DERPMap(),
	}

	// Set domain
	if s.cfg != nil && s.cfg.ServerURL != "" {
		resp.Domain = s.cfg.ServerURL
	}

	// Include peers
	if !req.ReadOnly {
		peers := s.state.ListPeers(nodeView.ID())
		tailPeers := make([]*tailcfg.Node, 0, len(peers))
		for _, peer := range peers {
			tNode, err := peer.TailNode(req.Version, s.primaryRouteFunc(), nil)
			if err != nil {
				continue
			}
			tailPeers = append(tailPeers, tNode)
		}
		resp.Peers = tailPeers
	}

	return resp, nil
}

func (s *NoiseServer) primaryRouteFunc() types.RouteFunc {
	return func(id types.NodeID) []netip.Prefix {
		return s.state.PrimaryRoutesForNode(id)
	}
}

func boolPtr(b bool) *bool {
	return &b
}
