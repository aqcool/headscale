package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/juanfont/headscale-v2/internal/types"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
)

// AuthProvider interface for authentication providers
type AuthProvider interface {
	RegisterHandler(w http.ResponseWriter, r *http.Request)
	AuthHandler(w http.ResponseWriter, r *http.Request)
	RegisterURL(authID types.AuthID) string
	AuthURL(authID types.AuthID) string
}

// handleRegister processes node registration requests
func (h *Headscale) handleRegister(
	ctx context.Context,
	req tailcfg.RegisterRequest,
	machineKey key.MachinePublic,
) (*tailcfg.RegisterResponse, error) {
	h.logger.Debugf("Register request: nodeKey=%s", req.NodeKey.ShortString())

	// Check for logout (expiry in past)
	if !req.Expiry.IsZero() && req.Expiry.Before(time.Now()) {
		if node, ok := h.state.GetNodeByNodeKey(req.NodeKey); ok {
			resp, err := h.handleLogout(ctx, node, req, machineKey)
			if err != nil {
				return nil, err
			}
			if resp != nil {
				return resp, nil
			}
		}
	}

	// No auth provided - handle existing node or return error
	if req.Auth == nil {
		if node, ok := h.state.GetNodeByNodeKey(req.NodeKey); ok {
			if node.MachineKey() != machineKey {
				return nil, NewHTTPError(http.StatusUnauthorized, "machine key mismatch", nil)
			}

			// Return current node state for restart
			if req.Expiry.IsZero() {
				return nodeToRegisterResponse(node), nil
			}

			return h.handleLogout(ctx, node, req, machineKey)
		}

		// New node registration
		return nil, NewHTTPError(http.StatusUnauthorized, "registration required", nil)
	}

	// Auth key registration
	if req.Auth.AuthKey != "" {
		return h.handleRegisterWithAuthKey(ctx, req, machineKey)
	}

	// Interactive registration - needs approval
	return h.handleRegisterInteractive(req, machineKey)
}

func (h *Headscale) handleLogout(
	ctx context.Context,
	node types.NodeView,
	req tailcfg.RegisterRequest,
	machineKey key.MachinePublic,
) (*tailcfg.RegisterResponse, error) {
	if node.MachineKey() != machineKey {
		return nil, NewHTTPError(http.StatusUnauthorized, "machine key mismatch", nil)
	}

	// Mark node expired
	now := time.Now()
	_, err := h.state.UpdateNode(node.ID(), func(n *types.Node) {
		n.Expiry = &now
	})
	if err != nil {
		return nil, err
	}

	h.logger.Infof("Node %d logged out", node.ID())

	return &tailcfg.RegisterResponse{
		MachineAuthorized: false,
		NodeKeyExpired:    true,
	}, nil
}

func (h *Headscale) handleRegisterWithAuthKey(
	ctx context.Context,
	req tailcfg.RegisterRequest,
	machineKey key.MachinePublic,
) (*tailcfg.RegisterResponse, error) {
	// Validate auth key and register node
	node := &types.Node{
		MachineKey: machineKey,
		NodeKey:    req.NodeKey,
		Hostname:   req.Hostinfo.Hostname,
	}

	addedNode, err := h.state.AddNode(ctx, node)
	if err != nil {
		return nil, err
	}

	return nodeToRegisterResponse(addedNode), nil
}

func (h *Headscale) handleRegisterInteractive(
	req tailcfg.RegisterRequest,
	machineKey key.MachinePublic,
) (*tailcfg.RegisterResponse, error) {
	// Interactive registration requires user approval
	authURL := fmt.Sprintf("%s/register?node_key=%s", h.cfg.ServerURL, req.NodeKey.ShortString())

	return &tailcfg.RegisterResponse{
		AuthURL:           authURL,
		MachineAuthorized: false,
	}, nil
}

func nodeToRegisterResponse(node types.NodeView) *tailcfg.RegisterResponse {
	user := node.User()
	return &tailcfg.RegisterResponse{
		MachineAuthorized: true,
		User: tailcfg.User{
			ID:          tailcfg.UserID(user.ID()),
			DisplayName: user.Name(),
		},
		Login: tailcfg.Login{
			ID:          tailcfg.LoginID(user.ID()),
			DisplayName: user.Name(),
		},
	}
}
