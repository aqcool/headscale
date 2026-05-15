package service

import (
	"errors"
	"strings"
	"time"

	v1 "github.com/juanfont/headscale-v2/api/proto/v1"
	"github.com/juanfont/headscale-v2/internal/biz"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	ErrTagMustStartWithTag     = errors.New("tag must start with the string 'tag:'")
	ErrTagShouldBeLowercase    = errors.New("tag should be lowercase")
	ErrTagMustNotContainSpaces = errors.New("tags must not contain spaces")
)

type HeadscaleService struct {
	v1.UnimplementedHeadscaleServiceServer

	userUC       *biz.UserUsecase
	nodeUC       *biz.NodeUsecase
	apiKeyUC     *biz.APIKeyUsecase
	preAuthKeyUC *biz.PreAuthKeyUsecase
	policyUC     *biz.PolicyUsecase
}

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

	for _, ip := range n.IPs {
		proto.IpAddresses = append(proto.IpAddresses, ip.String())
	}

	proto.Tags = n.Tags

	for _, r := range n.ApprovedRoutes {
		proto.SubnetRoutes = append(proto.SubnetRoutes, r.String())
	}

	if n.LastSeen != nil {
		proto.LastSeen = timestamppb.New(*n.LastSeen)
	}
	if n.Expiry != nil {
		proto.Expiry = timestamppb.New(*n.Expiry)
	}

	if n.IsOnline != nil {
		proto.Online = *n.IsOnline
	}

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
