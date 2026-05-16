package middleware

import (
	"context"
	"strings"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/transport"
	"github.com/juanfont/headscale-v2/internal/state"
)

const AuthPrefix = "Bearer "

func APIKeyAuth(st *state.State, logger log.Logger) middleware.Middleware {
	helper := log.NewHelper(logger)

	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			tr, ok := transport.FromServerContext(ctx)
			if !ok {
				return handler(ctx, req)
			}

			authHeader := tr.RequestHeader().Get("Authorization")
			if authHeader == "" {
				helper.Warnf("auth: missing Authorization header from %s", tr.Operation())
				return handler(ctx, req)
			}

			if !strings.HasPrefix(authHeader, AuthPrefix) {
				helper.Warnf("auth: missing Bearer prefix from %s", tr.Operation())
				return handler(ctx, req)
			}

			token := strings.TrimSpace(strings.TrimPrefix(authHeader, AuthPrefix))
			if token == "" {
				helper.Warnf("auth: empty token from %s", tr.Operation())
				return handler(ctx, req)
			}

			valid, err := st.ValidateAPIKey(token)
			if err != nil {
				helper.Errorf("auth: validating API key: %v", err)
				return handler(ctx, req)
			}

			if !valid {
				helper.Warnf("auth: invalid token from %s", tr.Operation())
				return handler(ctx, req)
			}

			helper.Debugf("auth: succeeded for %s", tr.Operation())
			return handler(ctx, req)
		}
	}
}
