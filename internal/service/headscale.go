package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"sort"
	"strings"
	"time"

	v1 "github.com/juanfont/headscale-v2/api/proto/v1"
	"github.com/juanfont/headscale-v2/internal/biz"
	"github.com/juanfont/headscale-v2/internal/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"tailscale.com/net/tsaddr"
	"tailscale.com/types/key"
)

var (
	ErrTagMustStartWithTag     = errors.New("tag must start with the string 'tag:'")
	ErrTagShouldBeLowercase    = errors.New("tag should be lowercase")
	ErrTagMustNotContainSpaces = errors.New("tags must not contain spaces")
	ErrNodeNotFound            = errors.New("node not found")
	ErrUserNotFound            = errors.New("user not found")
)

// generateKey generates a random hex key.
func generateKey(length int) string {
	b := make([]byte, length)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// HeadscaleService is a headscale service.
type HeadscaleService struct {
	v1.UnimplementedHeadscaleServiceServer

	userUC       *biz.UserUsecase
	nodeUC       *biz.NodeUsecase
	apiKeyUC     *biz.APIKeyUsecase
	preAuthKeyUC *biz.PreAuthKeyUsecase
	policyUC     *biz.PolicyUsecase
}

// NewHeadscaleService creates a new headscale service.
func NewHeadscaleService(
	userUC *biz.UserUsecase,
	nodeUC *biz.NodeUsecase,
	apiKeyUC *biz.APIKeyUsecase,
	preAuthKeyUC *biz.PreAuthKeyUsecase,
	policyUC *biz.PolicyUsecase,
) *HeadscaleService {
	return &HeadscaleService{
		userUC:       userUC,
		nodeUC:       nodeUC,
		apiKeyUC:     apiKeyUC,
		preAuthKeyUC: preAuthKeyUC,
		policyUC:     policyUC,
	}
}

// --- User methods ---

func (s *HeadscaleService) CreateUser(ctx context.Context, req *v1.CreateUserRequest) (*v1.CreateUserResponse, error) {
	user, err := s.userUC.CreateUser(ctx, &biz.User{
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Email:       req.Email,
		ProfileURL:  req.PictureUrl,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "creating user: %s", err)
	}
	return &v1.CreateUserResponse{User: bizUserToProto(user)}, nil
}

func (s *HeadscaleService) ListUsers(ctx context.Context, req *v1.ListUsersRequest) (*v1.ListUsersResponse, error) {
	var users []*biz.User
	var err error

	switch {
	case req.GetName() != "":
		users, err = s.userUC.ListUsers(ctx)
		// Filter by name
		filtered := make([]*biz.User, 0)
		for _, u := range users {
			if u.Name == req.GetName() {
				filtered = append(filtered, u)
			}
		}
		users = filtered
	case req.GetEmail() != "":
		users, err = s.userUC.ListUsers(ctx)
		// Filter by email
		filtered := make([]*biz.User, 0)
		for _, u := range users {
			if u.Email == req.GetEmail() {
				filtered = append(filtered, u)
			}
		}
		users = filtered
	case req.GetId() != 0:
		user, err := s.userUC.GetUser(ctx, int(req.GetId()))
		if err != nil {
			return nil, err
		}
		users = []*biz.User{user}
	default:
		users, err = s.userUC.ListUsers(ctx)
	}

	if err != nil {
		return nil, err
	}

	protoUsers := make([]*v1.User, 0, len(users))
	for _, u := range users {
		protoUsers = append(protoUsers, bizUserToProto(u))
	}

	sort.Slice(protoUsers, func(i, j int) bool {
		return protoUsers[i].Id < protoUsers[j].Id
	})

	return &v1.ListUsersResponse{Users: protoUsers}, nil
}

func (s *HeadscaleService) DeleteUser(ctx context.Context, req *v1.DeleteUserRequest) (*v1.DeleteUserResponse, error) {
	err := s.userUC.DeleteUser(ctx, int(req.Id))
	if err != nil {
		return nil, err
	}
	return &v1.DeleteUserResponse{}, nil
}

func (s *HeadscaleService) RenameUser(ctx context.Context, req *v1.RenameUserRequest) (*v1.RenameUserResponse, error) {
	user, err := s.userUC.GetUser(ctx, int(req.OldId))
	if err != nil {
		return nil, err
	}
	user.Name = req.NewName
	user, err = s.userUC.UpdateUser(ctx, user)
	if err != nil {
		return nil, err
	}
	return &v1.RenameUserResponse{User: bizUserToProto(user)}, nil
}

// --- Node methods ---

func (s *HeadscaleService) ListNodes(ctx context.Context, req *v1.ListNodesRequest) (*v1.ListNodesResponse, error) {
	var nodes []*biz.Node
	var err error

	if req.GetUser() != "" {
		// Get user by name
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
	// Validate tags not empty
	if len(req.GetTags()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "cannot remove all tags from a node - tagged nodes must have at least one tag")
	}

	// Validate tag format
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
			return nil, fmt.Errorf("parsing route: %w", err)
		}

		// If exit route, add both IPv4 and IPv6
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
	// Find user
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

	// Generate node keys
	nodeKey := key.NewNode()
	machineKey := key.NewMachine()
	discoKey := key.NewDisco()

	// Create node
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
		return nil, errors.New("not confirmed, aborting")
	}

	// List all nodes
	nodes, err := s.nodeUC.ListNodes(ctx)
	if err != nil {
		return nil, err
	}

	var changes []string
	for _, n := range nodes {
		// Check if node has no IPs assigned
		if len(n.IPs) == 0 {
			changes = append(changes, fmt.Sprintf("node %d (%s) has no IPs", n.ID, n.Hostname))
		}
	}

	return &v1.BackfillNodeIPsResponse{Changes: changes}, nil
}

func (s *HeadscaleService) DebugCreateNode(ctx context.Context, req *v1.DebugCreateNodeRequest) (*v1.DebugCreateNodeResponse, error) {
	// Find user
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

	// Generate node keys
	nodeKey := key.NewNode()
	machineKey := key.NewMachine()
	discoKey := key.NewDisco()

	// Create node
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

// --- API Key methods ---

func (s *HeadscaleService) CreateApiKey(ctx context.Context, req *v1.CreateApiKeyRequest) (*v1.CreateApiKeyResponse, error) {
	prefix := generateKey(8)
	secret := generateKey(32)

	var expiry *time.Time
	if req.GetExpiration() != nil {
		t := req.GetExpiration().AsTime()
		expiry = &t
	}

	savedKey, err := s.apiKeyUC.CreateAPIKey(ctx, &biz.APIKey{
		Prefix:  prefix,
		Key:     secret,
		Expiry:  expiry,
	})
	if err != nil {
		return nil, err
	}
	return &v1.CreateApiKeyResponse{ApiKey: savedKey.Prefix + "." + savedKey.Key}, nil
}

func (s *HeadscaleService) ListApiKeys(ctx context.Context, req *v1.ListApiKeysRequest) (*v1.ListApiKeysResponse, error) {
	keys, err := s.apiKeyUC.ListAPIKeys(ctx)
	if err != nil {
		return nil, err
	}
	protoKeys := make([]*v1.ApiKey, 0, len(keys))
	for _, k := range keys {
		protoKeys = append(protoKeys, bizAPIKeyToProto(k))
	}

	sort.Slice(protoKeys, func(i, j int) bool {
		return protoKeys[i].Id < protoKeys[j].Id
	})

	return &v1.ListApiKeysResponse{ApiKeys: protoKeys}, nil
}

func (s *HeadscaleService) DeleteApiKey(ctx context.Context, req *v1.DeleteApiKeyRequest) (*v1.DeleteApiKeyResponse, error) {
	if req.Prefix != "" {
		err := s.apiKeyUC.DeleteAPIKey(ctx, req.Prefix)
		if err != nil {
			return nil, err
		}
	} else if req.Id != 0 {
		// Find by ID
		keys, err := s.apiKeyUC.ListAPIKeys(ctx)
		if err != nil {
			return nil, err
		}
		for _, k := range keys {
			if uint64(k.ID) == req.Id {
				err = s.apiKeyUC.DeleteAPIKey(ctx, k.Prefix)
				if err != nil {
					return nil, err
				}
				break
			}
		}
	} else {
		return nil, status.Error(codes.InvalidArgument, "must provide id or prefix")
	}
	return &v1.DeleteApiKeyResponse{}, nil
}

func (s *HeadscaleService) ExpireApiKey(ctx context.Context, req *v1.ExpireApiKeyRequest) (*v1.ExpireApiKeyResponse, error) {
	var apiKey *biz.APIKey
	var err error

	if req.Prefix != "" {
		apiKey, err = s.apiKeyUC.GetAPIKeyByPrefix(ctx, req.Prefix)
	} else if req.Id != 0 {
		keys, err := s.apiKeyUC.ListAPIKeys(ctx)
		if err != nil {
			return nil, err
		}
		for _, k := range keys {
			if uint64(k.ID) == req.Id {
				apiKey = k
				break
			}
		}
	} else {
		return nil, status.Error(codes.InvalidArgument, "must provide id or prefix")
	}

	if err != nil || apiKey == nil {
		return nil, status.Errorf(codes.NotFound, "api key not found")
	}

	err = s.apiKeyUC.ExpireAPIKey(ctx, apiKey.Prefix)
	if err != nil {
		return nil, err
	}

	return &v1.ExpireApiKeyResponse{}, nil
}

// --- PreAuthKey methods ---

func (s *HeadscaleService) CreatePreAuthKey(ctx context.Context, req *v1.CreatePreAuthKeyRequest) (*v1.CreatePreAuthKeyResponse, error) {
	// Validate tags
	for _, tag := range req.AclTags {
		if err := validateTag(tag); err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	secret := generateKey(32)

	var expiry *time.Time
	if req.GetExpiration() != nil {
		t := req.GetExpiration().AsTime()
		expiry = &t
	}

	var userID uint
	if req.GetUser() != 0 {
		userID = uint(req.GetUser())
	} else {
		// Get first user if not specified
		users, err := s.userUC.ListUsers(ctx)
		if err != nil {
			return nil, err
		}
		if len(users) > 0 {
			userID = uint(users[0].ID)
		}
	}

	key, err := s.preAuthKeyUC.CreatePreAuthKey(ctx, &biz.PreAuthKey{
		Key:       secret,
		Reusable:  req.Reusable,
		Ephemeral: req.Ephemeral,
		Expiry:    expiry,
		UserID:    userID,
		Tags:      req.AclTags,
	})
	if err != nil {
		return nil, err
	}
	return &v1.CreatePreAuthKeyResponse{PreAuthKey: bizPreAuthKeyToProto(key)}, nil
}

func (s *HeadscaleService) ListPreAuthKeys(ctx context.Context, req *v1.ListPreAuthKeysRequest) (*v1.ListPreAuthKeysResponse, error) {
	keys, err := s.preAuthKeyUC.ListPreAuthKeys(ctx, 0)
	if err != nil {
		return nil, err
	}
	protoKeys := make([]*v1.PreAuthKey, 0, len(keys))
	for _, k := range keys {
		protoKeys = append(protoKeys, bizPreAuthKeyToProto(k))
	}

	sort.Slice(protoKeys, func(i, j int) bool {
		return protoKeys[i].Id < protoKeys[j].Id
	})

	return &v1.ListPreAuthKeysResponse{PreAuthKeys: protoKeys}, nil
}

func (s *HeadscaleService) DeletePreAuthKey(ctx context.Context, req *v1.DeletePreAuthKeyRequest) (*v1.DeletePreAuthKeyResponse, error) {
	err := s.preAuthKeyUC.DeletePreAuthKey(ctx, int(req.Id))
	if err != nil {
		return nil, err
	}
	return &v1.DeletePreAuthKeyResponse{}, nil
}

func (s *HeadscaleService) ExpirePreAuthKey(ctx context.Context, req *v1.ExpirePreAuthKeyRequest) (*v1.ExpirePreAuthKeyResponse, error) {
	err := s.preAuthKeyUC.ExpirePreAuthKey(ctx, int(req.Id))
	if err != nil {
		return nil, err
	}
	return &v1.ExpirePreAuthKeyResponse{}, nil
}

// --- Health ---

func (s *HeadscaleService) Health(ctx context.Context, req *v1.HealthRequest) (*v1.HealthResponse, error) {
	return &v1.HealthResponse{DatabaseConnectivity: true}, nil
}

// --- Policy ---

func (s *HeadscaleService) GetPolicy(ctx context.Context, req *v1.GetPolicyRequest) (*v1.GetPolicyResponse, error) {
	pol, err := s.policyUC.GetPolicy(ctx)
	if err != nil {
		return nil, err
	}
	if pol == nil {
		return &v1.GetPolicyResponse{Policy: "{}"}, nil
	}
	return &v1.GetPolicyResponse{Policy: pol.Data}, nil
}

func (s *HeadscaleService) SetPolicy(ctx context.Context, req *v1.SetPolicyRequest) (*v1.SetPolicyResponse, error) {
	pol, err := s.policyUC.SetPolicy(ctx, req.Policy)
	if err != nil {
		return nil, err
	}
	return &v1.SetPolicyResponse{Policy: pol.Data, UpdatedAt: timestamppb.New(pol.UpdatedAt)}, nil
}

// --- Auth methods ---

func (s *HeadscaleService) AuthRegister(ctx context.Context, req *v1.AuthRegisterRequest) (*v1.AuthRegisterResponse, error) {
	return &v1.AuthRegisterResponse{}, nil
}

func (s *HeadscaleService) AuthApprove(ctx context.Context, req *v1.AuthApproveRequest) (*v1.AuthApproveResponse, error) {
	return &v1.AuthApproveResponse{}, nil
}

func (s *HeadscaleService) AuthReject(ctx context.Context, req *v1.AuthRejectRequest) (*v1.AuthRejectResponse, error) {
	return &v1.AuthRejectResponse{}, nil
}

// --- Helper functions ---

func validateTag(tag string) error {
	if strings.Index(tag, "tag:") != 0 {
		return ErrTagMustStartWithTag
	}
	if strings.ToLower(tag) != tag {
		return ErrTagShouldBeLowercase
	}
	if len(strings.Fields(tag)) > 1 {
		return ErrTagMustNotContainSpaces
	}
	return nil
}

func ptrUint(v uint) *uint {
	return &v
}

func bizUserToProto(u *biz.User) *v1.User {
	return &v1.User{
		Id:            uint64(u.ID),
		Name:          u.Name,
		DisplayName:   u.DisplayName,
		Email:         u.Email,
		ProfilePicUrl: u.ProfileURL,
		CreatedAt:     timestamppb.New(u.CreatedAt),
	}
}

func bizNodeToProto(n *biz.Node) *v1.Node {
	proto := &v1.Node{
		Id:         uint64(n.ID),
		Name:       n.Hostname,
		GivenName:  n.GivenName,
		MachineKey: n.MachineKey.String(),
		NodeKey:    n.NodeKey.String(),
		DiscoKey:   n.DiscoKey.String(),
	}

	// Add IP addresses
	for _, ip := range n.IPs {
		proto.IpAddresses = append(proto.IpAddresses, ip.String())
	}

	// Add tags
	proto.Tags = n.Tags

	// Add approved routes
	for _, r := range n.ApprovedRoutes {
		proto.SubnetRoutes = append(proto.SubnetRoutes, r.String())
	}

	// Set timestamps
	if n.LastSeen != nil {
		proto.LastSeen = timestamppb.New(*n.LastSeen)
	}
	if n.Expiry != nil {
		proto.Expiry = timestamppb.New(*n.Expiry)
	}

	// Set online status
	if n.IsOnline != nil {
		proto.Online = *n.IsOnline
	}

	// Set user
	if n.User != nil {
		proto.User = bizUserToProto(n.User)
	}

	return proto
}

func bizAPIKeyToProto(k *biz.APIKey) *v1.ApiKey {
	proto := &v1.ApiKey{
		Id:     uint64(k.ID),
		Prefix: k.Prefix,
	}
	if k.CreatedAt != (time.Time{}) {
		proto.CreatedAt = timestamppb.New(k.CreatedAt)
	}
	if k.Expiry != nil {
		proto.Expiration = timestamppb.New(*k.Expiry)
	}
	return proto
}

func bizPreAuthKeyToProto(k *biz.PreAuthKey) *v1.PreAuthKey {
	proto := &v1.PreAuthKey{
		Id:        uint64(k.ID),
		Key:       k.Key,
		Reusable:  k.Reusable,
		Ephemeral: k.Ephemeral,
		Used:      k.UsedCount > 0,
		AclTags:   k.Tags,
	}
	if k.Expiry != nil {
		proto.Expiration = timestamppb.New(*k.Expiry)
	}
	if k.CreatedAt != (time.Time{}) {
		proto.CreatedAt = timestamppb.New(k.CreatedAt)
	}
	return proto
}
