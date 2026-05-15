package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sort"
	"time"

	v1 "github.com/juanfont/headscale-v2/api/proto/v1"
	"github.com/juanfont/headscale-v2/internal/biz"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *HeadscaleService) CreateApiKey(ctx context.Context, req *v1.CreateApiKeyRequest) (*v1.CreateApiKeyResponse, error) {
	prefix := generateAPIKey(8)
	secret := generateAPIKey(32)

	var expiry *time.Time
	if req.GetExpiration() != nil {
		t := req.GetExpiration().AsTime()
		expiry = &t
	}

	savedKey, err := s.apiKeyUC.CreateAPIKey(ctx, &biz.APIKey{
		Prefix: prefix,
		Key:    secret,
		Expiry: expiry,
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

func generateAPIKey(length int) string {
	b := make([]byte, length)
	rand.Read(b)
	return hex.EncodeToString(b)
}
