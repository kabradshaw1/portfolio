package http

import (
	"context"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/jwtctx"
)

// ContextWithJWT returns a new context that carries the user's bearer token.
// User-scoped tools extract it with JWTFromContext.
func ContextWithJWT(ctx context.Context, jwt string) context.Context {
	return jwtctx.WithJWT(ctx, jwt)
}

// JWTFromContext returns the bearer token attached by ContextWithJWT, or "".
func JWTFromContext(ctx context.Context) string {
	return jwtctx.FromContext(ctx)
}
