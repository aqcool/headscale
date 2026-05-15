package service

import (
	"context"
	"sort"
	"time"

	v1 "github.com/juanfont/headscale-v2/api/proto/v1"
	"github.com/juanfont/headscale-v2/internal/biz"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *HeadscaleService) CreatePreAuthKey(ctx context.Context, req *v1.CreatePreAuthKeyRequest) (*v1.CreatePreAuthKeyResponse, error) {
	for _, tag := range req.AclTags {
		if err := validateTag(tag); err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	secret := generateAPIKey(32)

	var expiry *time.Time
	if req.GetExpiration() != nil {
		t := req.GetExpiration().AsTime()
		expiry = &t
	}

	var userID uint
	if req.GetUser() != 0 {
		userID = uint(req.GetUser())
	} else {
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
