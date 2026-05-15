package server

import (
	"context"
	"strings"

	v1 "github.com/juanfont/headscale-v2/api/proto/v1"
	"github.com/juanfont/headscale-v2/internal/conf"
	"github.com/juanfont/headscale-v2/internal/service"
	"github.com/juanfont/headscale-v2/internal/state"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware"
	kratosgrpc "github.com/go-kratos/kratos/v2/transport/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

func NewGRPCServer(
	c *conf.Server,
	headscale *service.HeadscaleService,
	logger log.Logger,
) *kratosgrpc.Server {
	var opts []kratosgrpc.ServerOption

	if c.Grpc.Network != "" {
		opts = append(opts, kratosgrpc.Network(c.Grpc.Network))
	}
	if c.Grpc.Addr != "" {
		opts = append(opts, kratosgrpc.Address(c.Grpc.Addr))
	}
	if c.Grpc.Timeout != nil {
		opts = append(opts, kratosgrpc.Timeout(c.Grpc.Timeout.AsDuration()))
	}

	srv := kratosgrpc.NewServer(opts...)
	v1.RegisterHeadscaleServiceServer(srv, headscale)
	return srv
}

func NewGRPCServerWithAuth(
	c *conf.Server,
	headscale *service.HeadscaleService,
	st *state.State,
	logger log.Logger,
) *kratosgrpc.Server {
	var opts []kratosgrpc.ServerOption

	if c.Grpc.Network != "" {
		opts = append(opts, kratosgrpc.Network(c.Grpc.Network))
	}
	if c.Grpc.Addr != "" {
		opts = append(opts, kratosgrpc.Address(c.Grpc.Addr))
	}
	if c.Grpc.Timeout != nil {
		opts = append(opts, kratosgrpc.Timeout(c.Grpc.Timeout.AsDuration()))
	}

	authMiddleware := createGRPCAuthMiddleware(st, logger)

	opts = append(opts, kratosgrpc.Middleware(authMiddleware))

	srv := kratosgrpc.NewServer(opts...)
	v1.RegisterHeadscaleServiceServer(srv, headscale)
	return srv
}

func createGRPCAuthMiddleware(st *state.State, logger log.Logger) middleware.Middleware {
	helper := log.NewHelper(logger)

	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			client, _ := peer.FromContext(ctx)
			helper.Debugf("GRPC auth: client=%s", client.Addr.String())

			meta, ok := metadata.FromIncomingContext(ctx)
			if !ok {
				return nil, status.Errorf(codes.InvalidArgument, "missing metadata")
			}

			authHeader, ok := meta["authorization"]
			if !ok || len(authHeader) == 0 {
				return nil, status.Errorf(codes.Unauthenticated, "authorization token not supplied")
			}

			token := authHeader[0]
			if !strings.HasPrefix(token, AuthPrefix) {
				return nil, status.Errorf(codes.Unauthenticated, "missing \"Bearer \" prefix in Authorization header")
			}

			valid, err := st.ValidateAPIKey(strings.TrimPrefix(token, AuthPrefix))
			if err != nil {
				helper.Errorf("validating API key: %v", err)
				return nil, status.Errorf(codes.Internal, "validating token")
			}

			if !valid {
				helper.Infof("invalid token from client %s", client.Addr.String())
				return nil, status.Errorf(codes.Unauthenticated, "invalid token")
			}

			return handler(ctx, req)
		}
	}
}

type GRPCAuthInterceptor struct {
	state  *state.State
	logger *log.Helper
}

func NewGRPCAuthInterceptor(st *state.State, logger log.Logger) *GRPCAuthInterceptor {
	return &GRPCAuthInterceptor{
		state:  st,
		logger: log.NewHelper(logger),
	}
}

func (i *GRPCAuthInterceptor) UnaryServerInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	client, _ := peer.FromContext(ctx)
	i.logger.Debugf("GRPC auth interceptor: client=%s, method=%s", client.Addr.String(), info.FullMethod)

	meta, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "missing metadata")
	}

	authHeader, ok := meta["authorization"]
	if !ok || len(authHeader) == 0 {
		return nil, status.Errorf(codes.Unauthenticated, "authorization token not supplied")
	}

	token := authHeader[0]
	if !strings.HasPrefix(token, AuthPrefix) {
		return nil, status.Errorf(codes.Unauthenticated, "missing \"Bearer \" prefix in Authorization header")
	}

	valid, err := i.state.ValidateAPIKey(strings.TrimPrefix(token, AuthPrefix))
	if err != nil {
		i.logger.Errorf("validating API key: %v", err)
		return nil, status.Errorf(codes.Internal, "validating token")
	}

	if !valid {
		i.logger.Infof("invalid token from client %s", client.Addr.String())
		return nil, status.Errorf(codes.Unauthenticated, "invalid token")
	}

	return handler(ctx, req)
}

func (i *GRPCAuthInterceptor) StreamServerInterceptor(
	srv interface{},
	ss grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) error {
	ctx := ss.Context()
	client, _ := peer.FromContext(ctx)
	i.logger.Debugf("GRPC stream auth interceptor: client=%s, method=%s", client.Addr.String(), info.FullMethod)

	meta, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Errorf(codes.InvalidArgument, "missing metadata")
	}

	authHeader, ok := meta["authorization"]
	if !ok || len(authHeader) == 0 {
		return status.Errorf(codes.Unauthenticated, "authorization token not supplied")
	}

	token := authHeader[0]
	if !strings.HasPrefix(token, AuthPrefix) {
		return status.Errorf(codes.Unauthenticated, "missing \"Bearer \" prefix in Authorization header")
	}

	valid, err := i.state.ValidateAPIKey(strings.TrimPrefix(token, AuthPrefix))
	if err != nil {
		i.logger.Errorf("validating API key: %v", err)
		return status.Errorf(codes.Internal, "validating token")
	}

	if !valid {
		i.logger.Infof("invalid token from client %s", client.Addr.String())
		return status.Errorf(codes.Unauthenticated, "invalid token")
	}

	return handler(srv, ss)
}