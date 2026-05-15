package service

import (
	"context"

	v1 "github.com/juanfont/headscale-v2/api/proto/v1"
)

func (s *HeadscaleService) Health(ctx context.Context, req *v1.HealthRequest) (*v1.HealthResponse, error) {
	return &v1.HealthResponse{DatabaseConnectivity: true}, nil
}

func (s *HeadscaleService) AuthRegister(ctx context.Context, req *v1.AuthRegisterRequest) (*v1.AuthRegisterResponse, error) {
	return &v1.AuthRegisterResponse{}, nil
}

func (s *HeadscaleService) AuthApprove(ctx context.Context, req *v1.AuthApproveRequest) (*v1.AuthApproveResponse, error) {
	return &v1.AuthApproveResponse{}, nil
}

func (s *HeadscaleService) AuthReject(ctx context.Context, req *v1.AuthRejectRequest) (*v1.AuthRejectResponse, error) {
	return &v1.AuthRejectResponse{}, nil
}
