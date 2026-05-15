package service

import (
	"context"
	"net/netip"
	"slices"
	"sort"
	"time"

	v1 "github.com/juanfont/headscale-v2/api/proto/v1"
	"github.com/juanfont/headscale-v2/internal/biz"
	"github.com/juanfont/headscale-v2/internal/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"tailscale.com/net/tsaddr"
	"tailscale.com/types/key"
)

func (s *HeadscaleService) ListNodes(ctx context.Context, req *v1.ListNodesRequest) (*v1.ListNodesResponse, error) {
	var nodes []*biz.Node
	var err error

	if req.GetUser() != "" {
		users, err := s.userUC.ListUsers(ctx)
		if err != nil {
			return nil, err
		}
		var targetUser *biz.User
		for _, u := range users {
			if u.Name == req.GetUser() {
				targetUser = u
				break
			}
		}
		if targetUser == nil {
			return nil, status.Errorf(codes.NotFound, "user not found")
		}
		nodes, err = s.nodeUC.ListNodesByUser(ctx, uint(targetUser.ID))
	} else {
		nodes, err = s.nodeUC.ListNodes(ctx)
	}

	if err != nil {
		return nil, err
	}

	protoNodes := make([]*v1.Node, 0, len(nodes))
	for _, n := range nodes {
		protoNodes = append(protoNodes, bizNodeToProto(n))
	}

	sort.Slice(protoNodes, func(i, j int) bool {
		return protoNodes[i].Id < protoNodes[j].Id
	})

	return &v1.ListNodesResponse{Nodes: protoNodes}, nil
}

func (s *HeadscaleService) GetNode(ctx context.Context, req *v1.GetNodeRequest) (*v1.GetNodeResponse, error) {
	node, err := s.nodeUC.GetNode(ctx, int(req.NodeId))
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, status.Errorf(codes.NotFound, "node not found")
	}
	return &v1.GetNodeResponse{Node: bizNodeToProto(node)}, nil
}

func (s *HeadscaleService) DeleteNode(ctx context.Context, req *v1.DeleteNodeRequest) (*v1.DeleteNodeResponse, error) {
	err := s.nodeUC.DeleteNode(ctx, int(req.NodeId))
	if err != nil {
		return nil, err
	}
	return &v1.DeleteNodeResponse{}, nil
}

func (s *HeadscaleService) ExpireNode(ctx context.Context, req *v1.ExpireNodeRequest) (*v1.ExpireNodeResponse, error) {
	if req.GetDisableExpiry() && req.GetExpiry() != nil {
		return nil, status.Error(codes.InvalidArgument, "cannot set both disable_expiry and expiry")
	}

	var expiry *time.Time
	if req.GetDisableExpiry() {
		expiry = nil
	} else if req.GetExpiry() != nil {
		t := req.GetExpiry().AsTime()
		expiry = &t
	} else {
		now := time.Now()
		expiry = &now
	}

	node, err := s.nodeUC.GetNode(ctx, int(req.NodeId))
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, status.Errorf(codes.NotFound, "node not found")
	}

	err = s.nodeUC.SetExpiry(ctx, node.ID, expiry)
	if err != nil {
		return nil, err
	}

	node, err = s.nodeUC.GetNode(ctx, int(req.NodeId))
	if err != nil {
		return nil, err
	}

	return &v1.ExpireNodeResponse{Node: bizNodeToProto(node)}, nil
}

func (s *HeadscaleService) RenameNode(ctx context.Context, req *v1.RenameNodeRequest) (*v1.RenameNodeResponse, error) {
	err := s.nodeUC.RenameNode(ctx, types.NodeID(req.NodeId), req.NewName)
	if err != nil {
		return nil, err
	}

	node, err := s.nodeUC.GetNode(ctx, int(req.NodeId))
	if err != nil {
		return nil, err
	}

	return &v1.RenameNodeResponse{Node: bizNodeToProto(node)}, nil
}

func (s *HeadscaleService) SetTags(ctx context.Context, req *v1.SetTagsRequest) (*v1.SetTagsResponse, error) {
	if len(req.GetTags()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "cannot remove all tags from a node")
	}

	for _, tag := range req.GetTags() {
		if err := validateTag(tag); err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	err := s.nodeUC.SetTags(ctx, types.NodeID(req.NodeId), req.Tags)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	node, err := s.nodeUC.GetNode(ctx, int(req.NodeId))
	if err != nil {
		return nil, err
	}

	return &v1.SetTagsResponse{Node: bizNodeToProto(node)}, nil
}

func (s *HeadscaleService) SetApprovedRoutes(ctx context.Context, req *v1.SetApprovedRoutesRequest) (*v1.SetApprovedRoutesResponse, error) {
	var newApproved []netip.Prefix
	for _, route := range req.GetRoutes() {
		prefix, err := netip.ParsePrefix(route)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "parsing route: %v", err)
		}

		if prefix == tsaddr.AllIPv4() || prefix == tsaddr.AllIPv6() {
			newApproved = append(newApproved, tsaddr.AllIPv4(), tsaddr.AllIPv6())
		} else {
			newApproved = append(newApproved, prefix)
		}
	}
	slices.SortFunc(newApproved, func(a, b netip.Prefix) int {
		return a.Addr().Compare(b.Addr())
	})
	newApproved = slices.Compact(newApproved)

	err := s.nodeUC.SetApprovedRoutes(ctx, types.NodeID(req.NodeId), newApproved)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	node, err := s.nodeUC.GetNode(ctx, int(req.NodeId))
	if err != nil {
		return nil, err
	}

	return &v1.SetApprovedRoutesResponse{Node: bizNodeToProto(node)}, nil
}

func (s *HeadscaleService) RegisterNode(ctx context.Context, req *v1.RegisterNodeRequest) (*v1.RegisterNodeResponse, error) {
	users, err := s.userUC.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	var targetUser *biz.User
	for _, u := range users {
		if u.Name == req.GetUser() {
			targetUser = u
			break
		}
	}
	if targetUser == nil {
		return nil, status.Errorf(codes.NotFound, "user not found")
	}

	nodeKey := key.NewNode()
	machineKey := key.NewMachine()
	discoKey := key.NewDisco()

	node := &biz.Node{
		MachineKey:     machineKey.Public(),
		NodeKey:        nodeKey.Public(),
		DiscoKey:       discoKey.Public(),
		Hostname:       "registered-node",
		GivenName:      "registered-node",
		UserID:         ptrUint(uint(targetUser.ID)),
		RegisterMethod: "cli",
	}

	savedNode, err := s.nodeUC.CreateNode(ctx, node)
	if err != nil {
		return nil, err
	}

	return &v1.RegisterNodeResponse{Node: bizNodeToProto(savedNode)}, nil
}

func (s *HeadscaleService) BackfillNodeIPs(ctx context.Context, req *v1.BackfillNodeIPsRequest) (*v1.BackfillNodeIPsResponse, error) {
	if !req.Confirmed {
		return nil, status.Error(codes.InvalidArgument, "not confirmed, aborting")
	}

	nodes, err := s.nodeUC.ListNodes(ctx)
	if err != nil {
		return nil, err
	}

	var changes []string
	for _, n := range nodes {
		if len(n.IPs) == 0 {
			changes = append(changes, "node "+n.Hostname+" has no IPs")
		}
	}

	return &v1.BackfillNodeIPsResponse{Changes: changes}, nil
}

func (s *HeadscaleService) DebugCreateNode(ctx context.Context, req *v1.DebugCreateNodeRequest) (*v1.DebugCreateNodeResponse, error) {
	users, err := s.userUC.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	var targetUser *biz.User
	for _, u := range users {
		if u.Name == req.GetUser() {
			targetUser = u
			break
		}
	}
	if targetUser == nil {
		return nil, status.Errorf(codes.NotFound, "user not found")
	}

	nodeKey := key.NewNode()
	machineKey := key.NewMachine()
	discoKey := key.NewDisco()

	node := &biz.Node{
		MachineKey:     machineKey.Public(),
		NodeKey:        nodeKey.Public(),
		DiscoKey:       discoKey.Public(),
		Hostname:       req.GetName(),
		GivenName:      req.GetName(),
		UserID:         ptrUint(uint(targetUser.ID)),
		RegisterMethod: "debug",
	}

	savedNode, err := s.nodeUC.CreateNode(ctx, node)
	if err != nil {
		return nil, err
	}

	return &v1.DebugCreateNodeResponse{Node: bizNodeToProto(savedNode)}, nil
}
