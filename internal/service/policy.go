package service

import (
	"context"

	v1 "github.com/juanfont/headscale-v2/api/proto/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

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
